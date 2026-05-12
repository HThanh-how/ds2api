
# DS2API Usage Analytics & Enhanced Monitoring Plan

> **Date**: 2026-05-12
> **Request**: Enhanced WebUI detail + full request usage log with token tracking, cost estimation, stored in a free database with auto-cleanup
> **Scope**: Suggestions only, no code changes

---

## 1. Current State Assessment

### What Already Exists

| Component | Location | Capability |
|-----------|----------|------------|
| **Chat History** | [`internal/chathistory/store.go`](d:/HT/ds2api/internal/chathistory/store.go:37) | Per-request JSON file storage, max 50 entries (ring buffer). Tracks: CallerID, AccountID, Model, Usage (tokens), ElapsedMs, StatusCode, Content |
| **QueueCards** | [`webui/src/features/account/QueueCards.jsx`](d:/HT/ds2api/webui/src/features/account/QueueCards.jsx:8) | Shows only 3 numbers: Available accounts, In-use slots, Total pool |
| **ChatHistory UI** | [`webui/src/features/chatHistory/ChatHistoryContainer.jsx`](d:/HT/ds2api/webui/src/features/chatHistory/ChatHistoryContainer.jsx:16) | List + detail pane, auto-refresh, streaming status |
| **Admin API** | `GET /admin/queue/status` | Returns `available`, `in_use`, `total`, `available_accounts`, `in_use_accounts`, `waiting`, `max_queue_size` |

### What's Missing

1. **QueueCards too simple** — doesn't show per-account breakdown, queue depth, wait time, account health
2. **No persistent usage log** — chat history is limited to 50 entries and designed for conversation review, not analytics
3. **No token aggregation** — can't answer "how many tokens did I use this hour/day/week?"
4. **No cost tracking** — no pricing config, no cost estimation
5. **No per-API-key breakdown** — can't see which caller consumes what
6. **No time-series data** — chat history doesn't support hourly/daily grouping
7. **No auto-cleanup for logs** — chat history has manual limit, but no time-based TTL

---

## 2. Recommended Free Database: SQLite

### Why SQLite is the Best Fit

| Criteria | SQLite | Alternative: DuckDB | Alternative: BoltDB |
|----------|--------|---------------------|---------------------|
| **Cost** | Free, embedded | Free, embedded | Free, embedded |
| **Setup** | Zero-config, single file | Zero-config, single file | Zero-config, single file |
| **Go driver** | `github.com/mattn/go-sqlite3` (pure Go: `modernc.org/sqlite`) | `github.com/marcboeker/go-duckdb` | `go.etcd.io/bbolt` |
| **Query power** | Full SQL, aggregations, GROUP BY | Full SQL, excellent for analytics | Key-value only, no queries |
| **Concurrency** | WAL mode supports concurrent reads + single writer | Good concurrency | MVCC, good concurrency |
| **File size** | Can handle GB-scale databases | Better for huge analytical workloads | Good for moderate data |
| **Time-based cleanup** | `DELETE FROM usage WHERE created_at < datetime('now', '-7 days')` | Same | Must implement manually |
| **Maturity** | Extremely mature, battle-tested | Newer, less Go ecosystem | Mature |

**Recommendation: SQLite with WAL mode.** It's the most pragmatic choice:
- Pure Go driver (`modernc.org/sqlite`) means no CGO dependency, cross-compiles everywhere
- Already in the Go ecosystem — many projects use it for exactly this use case
- Simple SQL queries for aggregation by hour, by API key, by account
- TTL cleanup with a simple cron-like goroutine running `DELETE` every hour

---

## 3. Database Schema Design

### Main Usage Log Table

```sql
CREATE TABLE IF NOT EXISTS usage_log (
    id              TEXT PRIMARY KEY,          -- UUID, matches chat_history entry ID
    created_at      INTEGER NOT NULL,          -- Unix timestamp (seconds) for time-based queries & TTL
    caller_id       TEXT NOT NULL,             -- SHA256 prefix of API key ("caller:abcdef12")
    caller_name     TEXT DEFAULT '',           -- Human-readable API key name (from api_keys[].name)
    account_id      TEXT DEFAULT '',           -- DeepSeek account identifier (email/mobile)
    surface         TEXT NOT NULL,             -- "chat", "responses", "claude", "gemini", "embeddings"
    model           TEXT NOT NULL,             -- DeepSeek model name (e.g., "deepseek-v4-pro")
    stream          INTEGER NOT NULL DEFAULT 0,-- 1 = streaming, 0 = non-streaming
    status_code     INTEGER NOT NULL,          -- HTTP status code returned to client
    elapsed_ms      INTEGER NOT NULL,          -- Total request latency in milliseconds
    prompt_tokens   INTEGER NOT NULL DEFAULT 0,-- Input/prompt token count
    output_tokens   INTEGER NOT NULL DEFAULT 0,-- Output/completion token count (including reasoning)
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,-- Reasoning-only token count
    total_tokens    INTEGER NOT NULL DEFAULT 0,-- prompt_tokens + output_tokens
    input_cost      REAL NOT NULL DEFAULT 0.0, -- Estimated USD cost for input tokens
    output_cost     REAL NOT NULL DEFAULT 0.0, -- Estimated USD cost for output tokens
    total_cost      REAL NOT NULL DEFAULT 0.0, -- input_cost + output_cost
    retry_count     INTEGER NOT NULL DEFAULT 0,-- Number of empty-output retries performed
    finish_reason   TEXT DEFAULT '',           -- "stop", "length", "tool_calls", "content_filter", etc.
    error_message   TEXT DEFAULT '',           -- Error message if request failed
    user_input      TEXT DEFAULT '',           -- Truncated user input (first 200 chars for context)
    content_preview TEXT DEFAULT ''            -- Truncated assistant output (first 200 chars)
);

-- Index for time-based queries and cleanup
CREATE INDEX IF NOT EXISTS idx_usage_log_created_at ON usage_log(created_at);

-- Index for per-caller aggregation
CREATE INDEX IF NOT EXISTS idx_usage_log_caller_id ON usage_log(caller_id);

-- Index for per-account aggregation
CREATE INDEX IF NOT EXISTS idx_usage_log_account_id ON usage_log(account_id);

-- Composite index for hourly aggregation queries
CREATE INDEX IF NOT EXISTS idx_usage_log_surface_model_created
    ON usage_log(surface, model, created_at);
```

### Hourly Aggregation Table (Optional, for faster dashboard queries)

```sql
CREATE TABLE IF NOT EXISTS usage_hourly (
    hour            TEXT NOT NULL,             -- "2026-05-12T14:00:00Z" (ISO 8601, truncated to hour)
    caller_id       TEXT NOT NULL,
    account_id      TEXT DEFAULT '',
    surface         TEXT NOT NULL,
    model           TEXT NOT NULL,
    request_count   INTEGER NOT NULL DEFAULT 0,
    total_prompt_tokens   INTEGER NOT NULL DEFAULT 0,
    total_output_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    total_cost      REAL NOT NULL DEFAULT 0.0,
    avg_elapsed_ms  REAL NOT NULL DEFAULT 0.0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (hour, caller_id, surface, model)
);
```

### Pricing Configuration Table (or JSON in config)

```sql
-- Alternative: store in config.json as pricing.model_prices
CREATE TABLE IF NOT EXISTS pricing (
    model           TEXT PRIMARY KEY,          -- "deepseek-v4-flash"
    input_per_1m    REAL NOT NULL,             -- USD per 1M input tokens
    output_per_1m   REAL NOT NULL,             -- USD per 1M output tokens
    currency        TEXT NOT NULL DEFAULT 'USD',
    updated_at      INTEGER NOT NULL
);
```

**Better approach for pricing**: Store in `config.json` under a new `pricing` section, since it needs to be user-editable via WebUI and doesn't change frequently:

```json
"pricing": {
  "enabled": true,
  "default_input_per_1m": 0.14,
  "default_output_per_1m": 0.28,
  "model_prices": {
    "deepseek-v4-flash": { "input_per_1m": 0.14, "output_per_1m": 0.28 },
    "deepseek-v4-pro": { "input_per_1m": 0.55, "output_per_1m": 1.10 },
    "deepseek-v4-vision": { "input_per_1m": 0.55, "output_per_1m": 1.10 }
  }
}
```

---

## 4. Where to Hook Into the Code (Injection Points)

### A. Request Logging Hook

The ideal injection point is in the shared completion runtime, right after usage data is finalized. Based on the current architecture:

| Hook Point | File | What's Available |
|------------|------|------------------|
| **Non-stream completion end** | [`internal/completionruntime/nonstream.go`](d:/HT/ds2api/internal/completionruntime/nonstream.go:1) | `RequestAuth` (CallerID, AccountID), model, usage, elapsed, status, retry count |
| **Stream completion end** | [`internal/completionruntime/stream_retry.go`](d:/HT/ds2api/internal/completionruntime/stream_retry.go:39) | Same data via `StreamRetryHooks.Finalize` callback |
| **Chat history Update** | [`internal/chathistory/store.go`](d:/HT/ds2api/internal/chathistory/store.go:294) | Already stores usage — can extend to also write to SQLite |

**Recommended approach**: Create a new `internal/usagelog` package with a `Logger` that:

1. Has a `Record(params LogParams)` method
2. Is called from `completionruntime` at the end of every request (stream and non-stream)
3. Also called from the embeddings handler
4. Internally writes to SQLite asynchronously (buffered channel to avoid blocking the response)

### B. Queue Status Enhancement

The current [`GET /admin/queue/status`](d:/HT/ds2api/internal/account/pool_core.go:102) returns:
```json
{
  "available": 2,
  "in_use": 1,
  "total": 3,
  "available_accounts": ["email1@example.com", "email2@example.com"],
  "in_use_accounts": ["email3@example.com"],
  "max_inflight_per_account": 2,
  "global_max_inflight": 6,
  "recommended_concurrency": 6,
  "waiting": 0,
  "max_queue_size": 6
}
```

**Enhanced version** should add:
```json
{
  "available": 2,
  "in_use": 1,
  "total": 3,
  "waiting": 0,
  "accounts": [
    {
      "id": "email1@example.com",
      "name": "Primary Account",
      "status": "idle",           // "idle" | "busy" | "full" | "unhealthy"
      "in_use_slots": 0,
      "max_slots": 2,
      "health_score": 1.0,        // 0.0-1.0 based on recent success rate
      "consecutive_failures": 0,
      "last_used_at": "2026-05-12T14:30:00Z",
      "last_error": null
    },
    ...
  ],
  "recent_requests_per_minute": 12.5,  // Rolling 60-second window
  "uptime_seconds": 86400
}
```

### C. Usage Stats Admin Endpoint

New endpoints:
- `GET /admin/usage/stats?from=2026-05-12&to=2026-05-13` — aggregated stats
- `GET /admin/usage/log?page=1&limit=50&caller=xxx&model=yyy` — raw log entries
- `GET /admin/usage/summary` — today's summary (total requests, tokens, cost)

---

## 5. WebUI Enhancement Plan

### A. Enhanced QueueCards → "Dashboard" Tab

Replace or augment the current 3-card QueueCards with a richer dashboard:

```
┌──────────────────────────────────────────────────────────────┐
│  📊 Dashboard                                                 │
├────────────┬────────────┬────────────┬────────────────────────┤
│ Available  │ In Use     │ Queue      │ Requests/min (24h)     │
│   2 of 3   │  1 slot    │  0 waiting │  ▁▃▅▂█▄▃▁▂▃▅...      │
│  accounts  │  1 of 6    │  0 of 6    │  avg: 8.2 peak: 23     │
├────────────┴────────────┴────────────┴────────────────────────┤
│  Account Breakdown                                            │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ Account          │ Slots  │ Status  │ Health  │ Last Used│ │
│  │ user@email.com   │ 0/2    │ 🟢 idle │ 100%    │ 2m ago   │ │
│  │ user2@email.com  │ 1/2    │ 🟡 busy │ 95%     │ now      │ │
│  │ +8613800138000   │ 0/2    │ 🔴 fail │ 45%     │ 30s ago  │ │
│  └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### B. New "Usage Analytics" Tab (main ask)

A new sidebar entry `📈 Usage` with sub-tabs:

#### Tab 1: "Live Log" — Real-time request table

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  📋 Live Request Log                                    [Auto-refresh ⏸️▶️]   │
├──────┬──────────┬──────────┬──────────────┬───────┬──────┬──────┬────────────┤
│ Time │ API Key  │ Account  │ Model        │ Tokens│ Cost │ ms   │ Status     │
├──────┼──────────┼──────────┼──────────────┼───────┼──────┼──────┼────────────┤
│14:32 │ sk-****ab│ u@e.com  │ v4-pro       │1.2k/3k│$0.003│ 850ms│ ✅ 200     │
│14:31 │ sk-****cd│ 138****00│ v4-flash     │500/1k │$0.001│ 320ms│ ✅ 200     │
│14:30 │ sk-****ab│ u@e.com  │ v4-pro       │800/0k │$0.001│ 520ms│ ⚠️ empty   │
│14:30 │ sk-****ef│ u2@e.com │ v4-pro-search│2k/5k │$0.008│1200ms│ ✅ 200     │
└──────┴──────────┴──────────┴──────────────┴───────┴──────┴──────┴────────────┘
Page: [1] 2 3 ... 45   Showing 50 of 2,341
```

Clicking a row expands details: full user input, response preview, error details.

#### Tab 2: "Hourly Summary" — Aggregated by hour

```
┌──────────────────────────────────────────────────────────────────┐
│  📊 Hourly Usage                                    2026-05-12   │
├──────────┬─────────┬───────────┬──────────┬──────────┬───────────┤
│ Hour     │ Requests│ Tokens In │Tokens Out│  Cost    │ Errors    │
├──────────┼─────────┼───────────┼──────────┼──────────┼───────────┤
│ 14:00    │   156   │  245,000  │  89,000  │ $0.124   │    2      │
│ 13:00    │   203   │  312,000  │ 145,000  │ $0.198   │    5      │
│ 12:00    │   178   │  289,000  │ 102,000  │ $0.156   │    1      │
│ ...      │   ...   │    ...    │   ...    │   ...    │   ...     │
├──────────┼─────────┼───────────┼──────────┼──────────┼───────────┤
│ TOTAL    │ 2,341   │3,450,000  │890,000   │ $1.89    │   12      │
└──────────┴─────────┴───────────┴──────────┴──────────┴───────────┘
```

#### Tab 3: "By API Key" — Per-caller breakdown

```
┌──────────────────────────────────────────────────────────────────┐
│  🔑 Usage by API Key                                Today        │
├──────────────┬─────────┬───────────┬──────────┬──────────────────┤
│ API Key      │ Requests│Tokens     │ Cost     │ Top Model        │
├──────────────┼─────────┼───────────┼──────────┼──────────────────┤
│ Main Key     │  1,200  │ 2.1M      │ $1.12    │ v4-pro (60%)     │
│ sk-****ab    │         │           │          │ v4-flash (40%)   │
├──────────────┼─────────┼───────────┼──────────┼──────────────────┤
│ Backup Key   │    890  │ 1.5M      │ $0.65    │ v4-flash (80%)   │
│ sk-****cd    │         │           │          │ v4-pro (20%)     │
├──────────────┼─────────┼───────────┼──────────┼──────────────────┤
│ Test Key     │    251  │ 0.45M     │ $0.12    │ v4-flash (100%)  │
│ sk-****ef    │         │           │          │                  │
└──────────────┴─────────┴───────────┴──────────┴──────────────────┘
```

---

## 6. Auto-Cleanup Strategy

### SQLite TTL Cleanup (Weekly/Monthly)

A background goroutine runs every hour (or on startup):

```go
// Pseudocode — runs in a goroutine
func (l *Logger) startCleanupLoop(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            l.cleanup()
        case <-ctx.Done():
            return
        }
    }
}

func (l *Logger) cleanup() {
    cutoff := time.Now().AddDate(0, 0, -l.retentionDays).Unix()
    _, err := l.db.Exec(
        "DELETE FROM usage_log WHERE created_at < ?", cutoff,
    )
    if err != nil {
        config.Logger.Warn("[usage_log] cleanup failed", "error", err)
    } else {
        config.Logger.Info("[usage_log] cleanup completed", "cutoff", cutoff)
    }
    // Optionally VACUUM to reclaim disk space (less frequent, e.g., weekly)
}
```

### Configuration in `config.json`

```json
"usage_log": {
  "enabled": true,
  "db_path": "./data/usage.db",
  "retention_days": 30,
  "max_db_size_mb": 500,
  "async_write": true
}
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Enable/disable usage logging |
| `db_path` | `./data/usage.db` | Path to SQLite database file |
| `retention_days` | `30` | Auto-delete entries older than N days (0 = never delete) |
| `max_db_size_mb` | `500` | If DB exceeds this, delete oldest entries first |
| `async_write` | `true` | Buffer writes through a channel (non-blocking) |

---

## 7. Cost Estimation Logic

### Formula

```
input_cost  = (prompt_tokens / 1_000_000) * price_per_1m_input
output_cost = (output_tokens / 1_000_000) * price_per_1m_output
total_cost  = input_cost + output_cost
```

### Default Pricing (DeepSeek API, approximate as of mid-2026)

| Model | Input ($/1M tokens) | Output ($/1M tokens) |
|-------|---------------------|----------------------|
| `deepseek-v4-flash` | $0.14 | $0.28 |
| `deepseek-v4-flash-search` | $0.14 | $0.28 |
| `deepseek-v4-pro` | $0.55 | $1.10 |
| `deepseek-v4-pro-search` | $0.55 | $1.10 |
| `deepseek-v4-vision` | $0.55 | $1.10 |
| `default` (fallback) | $0.14 | $0.28 |

These prices should be **configurable** in `config.json` under the `pricing` section so users can update them when DeepSeek changes pricing.

### Reasoning Token Cost

Currently, reasoning tokens (thinking content) are counted separately in `usage.completion_tokens_details.reasoning_tokens`. They should be priced the same as output tokens (they consume output capacity).

---

## 8. Implementation Roadmap (Suggested Order)

### Phase 1 — Backend Foundation
1. Add `github.com/mattn/go-sqlite3` or `modernc.org/sqlite` to `go.mod`
2. Create `internal/usagelog/` package with:
   - `store.go` — SQLite init, migrations, CRUD
   - `logger.go` — async write worker with buffered channel
   - `cleanup.go` — background TTL cleanup goroutine
   - `query.go` — aggregation queries (hourly, by caller, by account)
3. Create `internal/httpapi/admin/usage/` handler with:
   - `GET /admin/usage/stats` — time-range stats
   - `GET /admin/usage/log` — paginated log
   - `GET /admin/usage/summary` — today's summary
4. Hook into `completionruntime` (both stream and non-stream) to call `usagelog.Record()`
5. Enhance `GET /admin/queue/status` with per-account detail + health scoring

### Phase 2 — WebUI
6. Add `📈 Usage` sidebar entry in `DashboardShell.jsx`
7. Create `webui/src/features/usage/` directory with:
   - `UsageContainer.jsx` — main container with tabs
   - `UsageLogTable.jsx` — live request log table
   - `UsageHourlyChart.jsx` — hourly aggregation (simple bar chart or table)
   - `UsageByKey.jsx` — per-API-key breakdown
   - `usageApi.js` — fetch helpers for the new endpoints
8. Enhance `QueueCards.jsx` → rename to `DashboardCards.jsx` with per-account detail table
9. Add i18n keys for all new UI strings (en + zh)

### Phase 3 — Polish
10. Add simple chart visualization (consider lightweight library like `recharts` or just HTML/CSS bar charts)
11. Add CSV export button for usage data
12. Add WebUI settings section for pricing configuration

---

## 9. Alternative Approaches Considered

### A. Extend Chat History Instead of New SQLite DB

**Rejected because:**
- Chat history is limited to 50 entries (ring buffer design)
- File-based JSON is not queryable for aggregation
- Mixing analytics with conversation history creates confusion
- Chat history is for conversation review; usage log is for analytics

### B. Use External Time-Series DB (InfluxDB, TimescaleDB)

**Rejected because:**
- User wants "free database" — external DBs add operational complexity
- SQLite handles the expected volume easily (even 1M requests/month is only ~100MB)
- Embedded solution keeps DS2API self-contained

### C. Just Use Log Files + Parse with grep/awk

**Rejected because:**
- Not user-friendly (user wants a UI table)
- No aggregation capability without external tools
- Structured querying (by API key, by hour, by model) is painful with flat files

### D. DuckDB Instead of SQLite

**Considered but lower priority:**
- DuckDB is better for heavy analytical queries (columnar storage)
- But SQLite is sufficient for this use case
- Go DuckDB driver is less mature
- Could be a future migration path if data grows very large

---

## 10. Summary

| # | What | How | Effort |
|---|------|-----|--------|
| 1 | **Enhanced QueueCards** | Add per-account detail to existing `/admin/queue/status`, update UI component | Low |
| 2 | **Usage Log DB** | SQLite with `internal/usagelog/` package, async writes | Medium |
| 3 | **Usage Admin API** | New `/admin/usage/*` endpoints with time-range queries | Medium |
| 4 | **Usage WebUI** | New "Usage" tab with log table, hourly summary, per-key breakdown | Medium |
| 5 | **Cost Estimation** | Configurable pricing in `config.json`, compute on write | Low |
| 6 | **Auto-Cleanup** | Background goroutine, TTL-based DELETE + periodic VACUUM | Low |
| 7 | **Account Health** | Track consecutive failures, expose in queue status, auto-skip unhealthy | Low |

The entire feature set can be implemented incrementally. Start with the SQLite logger + basic admin API (backend only, ~2-3 files), then add the WebUI table. The cost estimation is trivial math and can be added last.

The result: a complete, self-hosted usage analytics system with zero external dependencies, auto-cleanup, and a clean WebUI table that answers "who used what, when, how many tokens, and how much did it cost?"
