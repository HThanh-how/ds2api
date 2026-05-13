package health

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ds2api/internal/config"
)

type tursoClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

func newTursoClient(url, token string) *tursoClient {
	baseURL := strings.TrimSpace(url)
	baseURL = strings.TrimPrefix(baseURL, "libsql://")
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	return &tursoClient{
		baseURL:    baseURL,
		authToken:  strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type tursoRequest struct {
	Requests []tursoStmt `json:"requests"`
}

type tursoStmt struct {
	Type string   `json:"type"`
	Stmt tursoSQL `json:"stmt,omitempty"`
}

type tursoSQL struct {
	Sql  string     `json:"sql"`
	Args []tursoArg `json:"args,omitempty"`
}

type tursoArg struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

func toArgs(args []any) []tursoArg {
	out := make([]tursoArg, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			out[i] = tursoArg{Type: "text", Value: v}
		case int:
			out[i] = tursoArg{Type: "integer", Value: fmt.Sprintf("%d", v)}
		case int64:
			out[i] = tursoArg{Type: "integer", Value: fmt.Sprintf("%d", v)}
		case float64:
			out[i] = tursoArg{Type: "float", Value: v}
		case bool:
			if v {
				out[i] = tursoArg{Type: "integer", Value: "1"}
			} else {
				out[i] = tursoArg{Type: "integer", Value: "0"}
			}
		case nil:
			out[i] = tursoArg{Type: "null", Value: nil}
		default:
			out[i] = tursoArg{Type: "text", Value: fmt.Sprintf("%v", a)}
		}
	}
	return out
}

type tursoResponse struct {
	Results []tursoResult `json:"results"`
}

type tursoResult struct {
	Type     string         `json:"type"`
	Response *tursoExecResp `json:"response,omitempty"`
	Error    *tursoError    `json:"error,omitempty"`
}

type tursoExecResp struct {
	Result *tursoQueryResult `json:"result,omitempty"`
}

type tursoQueryResult struct {
	Cols []tursoCol      `json:"cols"`
	Rows [][]tursoRowVal `json:"rows"`
}

type tursoCol struct {
	Name string `json:"name"`
}

type tursoRowVal struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type tursoError struct {
	Message string `json:"message"`
}

func (c *tursoClient) execute(sql string, args ...any) error {
	resp, err := c.pipeline([]tursoStmt{{Type: "execute", Stmt: tursoSQL{Sql: sql, Args: toArgs(args)}}})
	if err != nil {
		return err
	}
	if len(resp.Results) > 0 && resp.Results[0].Error != nil {
		return fmt.Errorf("turso: %s", resp.Results[0].Error.Message)
	}
	return nil
}

func (c *tursoClient) query(sql string, args ...any) ([]string, [][]string, error) {
	resp, err := c.pipeline([]tursoStmt{{Type: "execute", Stmt: tursoSQL{Sql: sql, Args: toArgs(args)}}})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Results) == 0 {
		return nil, nil, nil
	}
	result := resp.Results[0]
	if result.Error != nil {
		return nil, nil, fmt.Errorf("turso: %s", result.Error.Message)
	}
	if result.Response == nil || result.Response.Result == nil {
		return nil, nil, nil
	}
	qr := result.Response.Result
	cols := make([]string, len(qr.Cols))
	for i, col := range qr.Cols {
		cols[i] = col.Name
	}
	rows := make([][]string, len(qr.Rows))
	for i, r := range qr.Rows {
		row := make([]string, len(r))
		for j, v := range r {
			if len(v.Value) > 0 && string(v.Value) != "null" {
				s := string(v.Value)
				if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
					var unquoted string
					_ = json.Unmarshal(v.Value, &unquoted)
					row[j] = unquoted
				} else {
					row[j] = s
				}
			}
		}
		rows[i] = row
	}
	return cols, rows, nil
}

func (c *tursoClient) pipeline(stmts []tursoStmt) (*tursoResponse, error) {
	body, err := json.Marshal(tursoRequest{Requests: stmts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+"/v2/pipeline", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("turso http %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var result tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ensureTable(tc *tursoClient) error {
	if tc == nil {
		return nil
	}
	sql := `CREATE TABLE IF NOT EXISTS account_health (
		account_id TEXT PRIMARY KEY,
		health_score REAL NOT NULL DEFAULT 100.0,
		consecutive_failures INTEGER NOT NULL DEFAULT 0,
		last_failure_reason TEXT DEFAULT '',
		last_failure_at INTEGER DEFAULT 0,
		last_success_at INTEGER DEFAULT 0,
		cooldown_until INTEGER DEFAULT 0,
		mute_until INTEGER DEFAULT 0,
		total_requests INTEGER NOT NULL DEFAULT 0,
		total_failures INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0
	)`
	if err := tc.execute(sql); err != nil {
		return err
	}
	_ = tc.execute(`CREATE INDEX IF NOT EXISTS idx_health_score ON account_health(health_score)`)
	return nil
}

func loadFromTurso(tc *tursoClient) ([]AccountHealth, error) {
	if tc == nil {
		return nil, nil
	}
	cols, rows, err := tc.query("SELECT account_id, health_score, consecutive_failures, last_failure_reason, last_failure_at, last_success_at, cooldown_until, mute_until, total_requests, total_failures, updated_at FROM account_health")
	if err != nil {
		return nil, err
	}
	colIdx := make(map[string]int, len(cols))
	for i, c := range cols {
		colIdx[c] = i
	}
	results := make([]AccountHealth, 0, len(rows))
	for _, row := range rows {
		h := AccountHealth{
			AccountID:           getCol(row, colIdx, "account_id"),
			HealthScore:         parseFloat(getCol(row, colIdx, "health_score"), 100),
			ConsecutiveFailures: parseInt(getCol(row, colIdx, "consecutive_failures")),
			LastFailureReason:   getCol(row, colIdx, "last_failure_reason"),
			LastFailureAt:       parseInt64(getCol(row, colIdx, "last_failure_at")),
			LastSuccessAt:       parseInt64(getCol(row, colIdx, "last_success_at")),
			CooldownUntil:       parseInt64(getCol(row, colIdx, "cooldown_until")),
			MuteUntil:           parseInt64(getCol(row, colIdx, "mute_until")),
			TotalRequests:       parseInt64(getCol(row, colIdx, "total_requests")),
			TotalFailures:       parseInt64(getCol(row, colIdx, "total_failures")),
			UpdatedAt:           parseInt64(getCol(row, colIdx, "updated_at")),
		}
		results = append(results, h)
	}
	return results, nil
}

func persistToTurso(tc *tursoClient, h *AccountHealth) {
	if tc == nil || h == nil {
		return
	}
	sql := `INSERT OR REPLACE INTO account_health
		(account_id, health_score, consecutive_failures, last_failure_reason, last_failure_at, last_success_at, cooldown_until, mute_until, total_requests, total_failures, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	err := tc.execute(sql,
		h.AccountID,
		h.HealthScore,
		h.ConsecutiveFailures,
		h.LastFailureReason,
		h.LastFailureAt,
		h.LastSuccessAt,
		h.CooldownUntil,
		h.MuteUntil,
		h.TotalRequests,
		h.TotalFailures,
		h.UpdatedAt,
	)
	if err != nil {
		config.Logger.Warn("[health] turso persist failed", "account", h.AccountID, "error", err)
	}
}

func getCol(row []string, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
