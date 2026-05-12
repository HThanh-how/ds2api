package usagelog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
)

const (
	DefaultLimit = 1000
	MaxLimit     = 5000
)

type Entry struct {
	ID               string  `json:"id"`
	CreatedAt        int64   `json:"created_at"`
	CallerID         string  `json:"caller_id,omitempty"`
	AccountID        string  `json:"account_id,omitempty"`
	Surface          string  `json:"surface,omitempty"`
	Model            string  `json:"model,omitempty"`
	Stream           bool    `json:"stream"`
	StatusCode       int     `json:"status_code"`
	ElapsedMs        int64   `json:"elapsed_ms,omitempty"`
	PromptTokens     int     `json:"prompt_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	InputCost        float64 `json:"input_cost"`
	OutputCost       float64 `json:"output_cost"`
	TotalCost        float64 `json:"total_cost"`
	RetryCount       int     `json:"retry_count"`
	FinishReason     string  `json:"finish_reason,omitempty"`
	ErrorCode        string  `json:"error_code,omitempty"`
	UserInputPreview string  `json:"user_input_preview,omitempty"`
}

type LogParams struct {
	CallerID         string
	AccountID        string
	Surface          string
	Model            string
	Stream           bool
	StatusCode       int
	ElapsedMs        int64
	PromptTokens     int
	OutputTokens     int
	ReasoningTokens  int
	RetryCount       int
	FinishReason     string
	ErrorCode        string
	UserInputPreview string
}

type Summary struct {
	Hour         string  `json:"hour"`
	Requests     int     `json:"requests"`
	PromptTokens int     `json:"prompt_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	Errors       int     `json:"errors"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
}

type CallerSummary struct {
	CallerID     string  `json:"caller_id"`
	Requests     int     `json:"requests"`
	PromptTokens int     `json:"prompt_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	Errors       int     `json:"errors"`
	TopModel     string  `json:"top_model,omitempty"`
}

type QueryParams struct {
	From     int64
	To       int64
	CallerID string
	Model    string
	Surface  string
	Page     int
	Limit    int
}

type Store struct {
	mu       sync.RWMutex
	entries  []Entry
	path     string
	maxLimit int
}

var defaultPricing = map[string]struct{ input, output float64 }{
	"deepseek-v4-flash":        {0.14, 0.28},
	"deepseek-v4-flash-search": {0.14, 0.28},
	"deepseek-v4-pro":          {0.55, 1.10},
	"deepseek-v4-pro-search":   {0.55, 1.10},
	"deepseek-v4-vision":       {0.55, 1.10},
	"default":                  {0.14, 0.28},
}

var globalStore *Store

func InitStore(path string, maxLimit int) *Store {
	if maxLimit <= 0 || maxLimit > MaxLimit {
		maxLimit = DefaultLimit
	}
	s := &Store{path: strings.TrimSpace(path), maxLimit: maxLimit}
	s.load()
	globalStore = s
	return s
}

func GlobalStore() *Store {
	return globalStore
}

func (s *Store) Log(params LogParams) {
	if s == nil {
		return
	}
	now := time.Now().UnixMilli()
	inputCost, outputCost := estimateCost(params.Model, params.PromptTokens, params.OutputTokens)
	totalTokens := params.PromptTokens + params.OutputTokens + params.ReasoningTokens
	entry := Entry{
		ID:               fmt.Sprintf("log_%d", now),
		CreatedAt:        now,
		CallerID:         strings.TrimSpace(params.CallerID),
		AccountID:        strings.TrimSpace(params.AccountID),
		Surface:          strings.TrimSpace(params.Surface),
		Model:            strings.TrimSpace(params.Model),
		Stream:           params.Stream,
		StatusCode:       params.StatusCode,
		ElapsedMs:        params.ElapsedMs,
		PromptTokens:     params.PromptTokens,
		OutputTokens:     params.OutputTokens,
		ReasoningTokens:  params.ReasoningTokens,
		TotalTokens:      totalTokens,
		InputCost:        inputCost,
		OutputCost:       outputCost,
		TotalCost:        inputCost + outputCost,
		RetryCount:       params.RetryCount,
		FinishReason:     strings.TrimSpace(params.FinishReason),
		ErrorCode:        strings.TrimSpace(params.ErrorCode),
		UserInputPreview: truncateString(params.UserInputPreview, 200),
	}
	s.mu.Lock()
	s.entries = append(s.entries, entry)
	if len(s.entries) > s.maxLimit {
		s.entries = s.entries[len(s.entries)-s.maxLimit:]
	}
	s.mu.Unlock()
	go s.saveAsync()
}

func (s *Store) Query(params QueryParams) ([]Entry, int) {
	if s == nil {
		return nil, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	filtered := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if params.From > 0 && e.CreatedAt < params.From {
			continue
		}
		if params.To > 0 && e.CreatedAt > params.To {
			continue
		}
		if params.CallerID != "" && e.CallerID != params.CallerID {
			continue
		}
		if params.Model != "" && !strings.Contains(strings.ToLower(e.Model), strings.ToLower(params.Model)) {
			continue
		}
		if params.Surface != "" && e.Surface != params.Surface {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})
	total := len(filtered)
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	start := (params.Page - 1) * params.Limit
	if start >= total {
		return nil, total
	}
	end := start + params.Limit
	if end > total {
		end = total
	}
	return filtered[start:end], total
}

func (s *Store) Summary(from, to int64) ([]Summary, error) {
	if s == nil {
		return nil, errors.New("usage log store not initialized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	hourMap := map[string]*Summary{}
	for _, e := range s.entries {
		if from > 0 && e.CreatedAt < from {
			continue
		}
		if to > 0 && e.CreatedAt > to {
			continue
		}
		t := time.UnixMilli(e.CreatedAt).UTC()
		hourKey := t.Format("2006-01-02T15:04")
		sm, ok := hourMap[hourKey]
		if !ok {
			sm = &Summary{Hour: hourKey}
			hourMap[hourKey] = sm
		}
		sm.Requests++
		sm.PromptTokens += e.PromptTokens
		sm.OutputTokens += e.OutputTokens
		sm.TotalTokens += e.TotalTokens
		sm.TotalCost += e.TotalCost
		if e.StatusCode >= 400 {
			sm.Errors++
		}
		sm.AvgLatencyMs = (sm.AvgLatencyMs*int64(sm.Requests-1) + e.ElapsedMs) / int64(sm.Requests)
	}
	summaries := make([]Summary, 0, len(hourMap))
	for _, sm := range hourMap {
		summaries = append(summaries, *sm)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Hour < summaries[j].Hour })
	return summaries, nil
}

func (s *Store) CallerSummary(from, to int64) ([]CallerSummary, error) {
	if s == nil {
		return nil, errors.New("usage log store not initialized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	callerMap := map[string]*CallerSummary{}
	callerModelCount := map[string]map[string]int{}
	for _, e := range s.entries {
		if from > 0 && e.CreatedAt < from {
			continue
		}
		if to > 0 && e.CreatedAt > to {
			continue
		}
		cs, ok := callerMap[e.CallerID]
		if !ok {
			cs = &CallerSummary{CallerID: e.CallerID}
			callerMap[e.CallerID] = cs
		}
		if callerModelCount[e.CallerID] == nil {
			callerModelCount[e.CallerID] = map[string]int{}
		}
		cs.Requests++
		cs.PromptTokens += e.PromptTokens
		cs.OutputTokens += e.OutputTokens
		cs.TotalTokens += e.TotalTokens
		cs.TotalCost += e.TotalCost
		if e.StatusCode >= 400 {
			cs.Errors++
		}
		callerModelCount[e.CallerID][e.Model]++
	}
	for id, cs := range callerMap {
		topModel := ""
		topCount := 0
		for m, c := range callerModelCount[id] {
			if c > topCount {
				topModel = m
				topCount = c
			}
		}
		cs.TopModel = topModel
	}
	out := make([]CallerSummary, 0, len(callerMap))
	for _, cs := range callerMap {
		out = append(out, *cs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	return out, nil
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.entries = nil
	s.mu.Unlock()
	go s.saveAsync()
}

func (s *Store) EntriesCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *Store) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var entries []Entry
	if json.Unmarshal(data, &entries) != nil {
		return
	}
	s.mu.Lock()
	s.entries = entries
	if len(s.entries) > s.maxLimit {
		s.entries = s.entries[len(s.entries)-s.maxLimit:]
	}
	s.mu.Unlock()
}

func (s *Store) saveAsync() {
	if s.path == "" {
		return
	}
	s.mu.RLock()
	entries := make([]Entry, len(s.entries))
	copy(entries, s.entries)
	s.mu.RUnlock()
	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		config.Logger.Warn("[usage_log] failed to marshal", "error", err)
		return
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		config.Logger.Warn("[usage_log] failed to write", "error", err)
		return
	}
	os.Rename(tmpPath, s.path)
}

func estimateCost(model string, promptTokens, outputTokens int) (float64, float64) {
	pricing, ok := defaultPricing[strings.ToLower(strings.TrimSpace(model))]
	if !ok {
		pricing = defaultPricing["default"]
	}
	inputCost := float64(promptTokens) / 1_000_000 * pricing.input
	outputCost := float64(outputTokens) / 1_000_000 * pricing.output
	return inputCost, outputCost
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
