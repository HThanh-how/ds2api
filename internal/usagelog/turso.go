package usagelog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type TursoClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

type tursoRequest struct {
	Requests []tursoStatement `json:"requests"`
}

type tursoStatement struct {
	Type  string         `json:"type"`
	Stmt  tursoSQL       `json:"stmt,omitempty"`
	Close *tursoCloseReq `json:"close,omitempty"`
}

type tursoSQL struct {
	Sql  string `json:"sql"`
	Args []any  `json:"args,omitempty"`
}

type tursoCloseReq struct{}

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
	Cols         []tursoCol      `json:"cols"`
	Rows         [][]tursoRowVal `json:"rows"`
	RowsAffected int             `json:"rows_affected"`
	LastInsertID *string         `json:"last_insert_rowid"`
}

type tursoCol struct {
	Name string `json:"name"`
}

type tursoRowVal struct {
	Type  string  `json:"type"`
	Value *string `json:"value"`
}

type tursoError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func NewTursoClient(tursoURL, authToken string) *TursoClient {
	baseURL := strings.TrimSpace(tursoURL)
	baseURL = strings.TrimPrefix(baseURL, "libsql://")
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	return &TursoClient{
		baseURL:    baseURL,
		authToken:  strings.TrimSpace(authToken),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *TursoClient) Execute(sql string, args ...any) error {
	resp, err := c.pipeline([]tursoStatement{{
		Type: "execute",
		Stmt: tursoSQL{Sql: sql, Args: args},
	}})
	if err != nil {
		return err
	}
	if len(resp.Results) > 0 && resp.Results[0].Error != nil {
		return fmt.Errorf("turso execute: %s", resp.Results[0].Error.Message)
	}
	return nil
}

func (c *TursoClient) Query(sql string, args ...any) ([]string, [][]string, error) {
	resp, err := c.pipeline([]tursoStatement{{
		Type: "execute",
		Stmt: tursoSQL{Sql: sql, Args: args},
	}})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Results) == 0 {
		return nil, nil, nil
	}
	result := resp.Results[0]
	if result.Error != nil {
		return nil, nil, fmt.Errorf("turso query: %s", result.Error.Message)
	}
	if result.Response == nil || result.Response.Result == nil {
		return nil, nil, nil
	}
	qr := result.Response.Result
	cols := make([]string, len(qr.Cols))
	for i, c := range qr.Cols {
		cols[i] = c.Name
	}
	rows := make([][]string, len(qr.Rows))
	for i, r := range qr.Rows {
		row := make([]string, len(r))
		for j, v := range r {
			if v.Value != nil {
				row[j] = *v.Value
			}
		}
		rows[i] = row
	}
	return cols, rows, nil
}

func (c *TursoClient) pipeline(stmts []tursoStatement) (*tursoResponse, error) {
	body, err := json.Marshal(tursoRequest{Requests: stmts})
	if err != nil {
		return nil, err
	}
	url := c.baseURL + "/v2/pipeline"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("turso http %d", resp.StatusCode)
	}
	var result tursoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *TursoClient) EnsureTable(ctx context.Context) error {
	sql := `CREATE TABLE IF NOT EXISTS usage_log (
		id TEXT PRIMARY KEY,
		created_at INTEGER NOT NULL,
		caller_id TEXT DEFAULT '',
		account_id TEXT DEFAULT '',
		surface TEXT DEFAULT '',
		model TEXT DEFAULT '',
		stream INTEGER NOT NULL DEFAULT 0,
		status_code INTEGER NOT NULL,
		elapsed_ms INTEGER NOT NULL DEFAULT 0,
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		input_cost REAL NOT NULL DEFAULT 0,
		output_cost REAL NOT NULL DEFAULT 0,
		total_cost REAL NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		finish_reason TEXT DEFAULT '',
		error_code TEXT DEFAULT '',
		user_input_preview TEXT DEFAULT ''
	)`
	if err := c.Execute(sql); err != nil {
		return fmt.Errorf("turso ensure table: %w", err)
	}
	idxSQL := `CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_log(created_at)`
	_ = c.Execute(idxSQL)
	idxCallerSQL := `CREATE INDEX IF NOT EXISTS idx_usage_caller ON usage_log(caller_id)`
	_ = c.Execute(idxCallerSQL)
	return nil
}
