// Package apikeydb persists configured API keys to Turso (libSQL) for durable
// membership checks. When Turso URL/token are unset, the client is nil and the
// config store uses file-backed keys only.
package apikeydb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Row is one API key row for sync (plaintext key exists only in memory here).
type Row struct {
	Key    string
	Name   string
	Remark string
}

// Client talks to Turso HTTP pipeline API (v2).
type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// New returns a client when URL and token are non-empty; otherwise nil.
func New(tursoURL, authToken string) *Client {
	baseURL := strings.TrimSpace(tursoURL)
	authToken = strings.TrimSpace(authToken)
	if baseURL == "" || authToken == "" {
		return nil
	}
	baseURL = strings.TrimPrefix(baseURL, "libsql://")
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	return &Client{
		baseURL:    baseURL,
		authToken:  authToken,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled is true for a non-nil client constructed with credentials.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.authToken != ""
}

func hashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func previewKey(plain string) string {
	if len(plain) <= 24 {
		return plain
	}
	return plain[:24]
}

// EnsureTable creates the ds2api_api_keys table if missing.
func (c *Client) EnsureTable(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	sql := `CREATE TABLE IF NOT EXISTS ds2api_api_keys (
		key_hash TEXT PRIMARY KEY,
		key_preview TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		updated_at INTEGER NOT NULL DEFAULT 0
	)`
	if err := c.exec(ctx, sql); err != nil {
		return err
	}
	return c.exec(ctx, `CREATE INDEX IF NOT EXISTS idx_api_keys_updated ON ds2api_api_keys(updated_at)`)
}

// HasKey reports whether the plaintext key exists in Turso.
func (c *Client) HasKey(ctx context.Context, plain string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return false, nil
	}
	h := hashKey(plain)
	_, rows, err := c.query(ctx, `SELECT 1 FROM ds2api_api_keys WHERE key_hash = ? LIMIT 1`, h)
	if err != nil {
		return false, err
	}
	return len(rows) > 0 && len(rows[0]) > 0 && rows[0][0] != "", nil
}

// Sync replaces all rows in Turso with the given set (config remains source for writes).
func (c *Client) Sync(ctx context.Context, rows []Row) error {
	if !c.Enabled() {
		return nil
	}
	if err := c.exec(ctx, `DELETE FROM ds2api_api_keys`); err != nil {
		return err
	}
	ts := time.Now().Unix()
	const batchSize = 40
	var stmts []tursoStmt
	flush := func() error {
		if len(stmts) == 0 {
			return nil
		}
		err := c.pipeline(ctx, stmts)
		stmts = stmts[:0]
		return err
	}
	for _, r := range rows {
		kk := strings.TrimSpace(r.Key)
		if kk == "" {
			continue
		}
		stmts = append(stmts, tursoStmt{
			Type: "execute",
			Stmt: tursoSQL{
				Sql:  `INSERT INTO ds2api_api_keys (key_hash, key_preview, name, remark, updated_at) VALUES (?,?,?,?,?)`,
				Args: toArgs([]any{hashKey(kk), previewKey(kk), r.Name, r.Remark, ts}),
			},
		})
		if len(stmts) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (c *Client) exec(ctx context.Context, sql string, args ...any) error {
	return c.pipeline(ctx, []tursoStmt{{Type: "execute", Stmt: tursoSQL{Sql: sql, Args: toArgs(args)}}})
}

func (c *Client) query(ctx context.Context, sql string, args ...any) ([]string, [][]string, error) {
	resp, err := c.pipelineResponse(ctx, []tursoStmt{{Type: "execute", Stmt: tursoSQL{Sql: sql, Args: toArgs(args)}}})
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

func (c *Client) pipeline(ctx context.Context, stmts []tursoStmt) error {
	_, err := c.pipelineResponse(ctx, stmts)
	return err
}

func (c *Client) pipelineResponse(ctx context.Context, stmts []tursoStmt) (*tursoResponse, error) {
	body, err := json.Marshal(tursoRequest{Requests: stmts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/pipeline", bytes.NewReader(body))
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
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("turso http %d: %s", resp.StatusCode, string(b))
	}
	var result tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	for _, r := range result.Results {
		if r.Error != nil {
			return &result, fmt.Errorf("turso: %s", r.Error.Message)
		}
	}
	return &result, nil
}
