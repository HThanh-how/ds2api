
# DS2API Enterprise Monitoring & Instant Error Alerting — Implementation Plan

> **Date**: 2026-05-12
> **Status**: Ready for implementation
> **Mode**: Architect plan → switch to Code mode

---

## Overview

Implement enterprise-grade monitoring with instant error notification, fully compatible with Vercel serverless deployment. The user will provide URLs/tokens after implementation.

### Two core deliverables

| # | Feature | What it does |
|---|---------|-------------|
| 1 | **Enterprise Monitoring** | Prometheus-compatible `/metrics` endpoint, request counters, latency histograms, account pool gauges, token usage tracking |
| 2 | **Instant Error Alerting** | Webhook notifications on critical errors via Discord/Slack/Telegram/custom URL, with rate limiting to prevent spam |

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Vercel                            │
│  ┌───────────────┐  ┌───────────────────────────┐   │
│  │ Go Function   │  │ GET /metrics              │   │
│  │ (handles req) │  │ (Prometheus endpoint)      │   │
│  └───┬───────────┘  └───────────────────────────┘   │
│      │                                               │
│      │ On critical error:                            │
│      │  → POST webhook (Discord/Slack/etc.)          │
│      │                                               │
│      │ On every request:                             │
│      │  → update in-memory Prometheus metrics        │
│      │                                               │
└──────┼───────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────┐
│              External Services (user configured)      │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────┐  │
│  │ Prometheus│  │ Discord  │  │ Slack / Telegram   │  │
│  │ (scrape) │  │ Webhook  │  │ / Custom Webhook   │  │
│  └──────────┘  └──────────┘  └────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

**Note**: On Vercel, Prometheus scraping won't work directly because serverless functions are ephemeral. However:
- The `/metrics` endpoint works for local/Docker deployments
- For Vercel, use **Pushgateway** or **OTEL collector** as an alternative
- The webhook alerting works perfectly on Vercel since it fires from within the function

---

## File Structure (All New Files)

```
internal/
├── monitor/
│   ├── metrics.go           # Prometheus metrics registry + counters/gauges/histograms
│   ├── metrics_test.go      # Tests for metrics
│   ├── alerter.go           # Webhook alerter (Discord/Slack/Telegram/custom)
│   ├── alerter_test.go      # Tests for alerter
│   ├── hooks.go             # Middleware hooks to inject into existing code
│   └── hooks_test.go        # Tests for hooks
├── httpapi/admin/monitor/
│   ├── handler.go           # Admin endpoints: GET /admin/monitor/settings, PUT /admin/monitor/settings
│   ├── handler_test.go      # Tests
│   └── deps.go              # Dependency injection helpers
```

### config.json additions

```json
"monitor": {
  "metrics": {
    "enabled": true,
    "path": "/metrics"
  },
  "alerting": {
    "enabled": true,
    "rate_limit_seconds": 60,
    "channels": {
      "discord": {
        "enabled": false,
        "webhook_url": ""
      },
      "slack": {
        "enabled": false,
        "webhook_url": ""
      },
      "telegram": {
        "enabled": false,
        "bot_token": "",
        "chat_id": ""
      },
      "custom": {
        "enabled": false,
        "url": "",
        "headers": {}
      }
    },
    "triggers": {
      "account_all_down": true,
      "high_error_rate": true,
      "high_error_rate_threshold": 0.30,
      "consecutive_upstream_failures": true,
      "consecutive_upstream_threshold": 10,
      "session_creation_failure": true,
      "pow_failure": true,
      "content_filter_block": true,
      "token_refresh_failure": true
    }
  }
}
```

---

## Implementation Details

### 1. Prometheus Metrics (`internal/monitor/metrics.go`)

Since Go's standard Prometheus client uses global registries which don't work well with serverless, we use a **lightweight, manual implementation** with atomic counters and a simple Prometheus text format serializer.

**Counters (atomic.Int64)**:
```
ds2api_requests_total{surface="chat",model="deepseek-v4-pro",status="200"} 1523
ds2api_requests_total{surface="responses",model="deepseek-v4-flash",status="200"} 892
ds2api_requests_total{surface="chat",model="deepseek-v4-pro",status="429"} 12
ds2api_requests_total{surface="chat",model="deepseek-v4-pro",status="500"} 3
```

**Histograms** (manually bucketed, cumulative):
```
ds2api_request_duration_ms_bucket{surface="chat",le="100"} 450
ds2api_request_duration_ms_bucket{surface="chat",le="500"} 1200
ds2api_request_duration_ms_bucket{surface="chat",le="1000"} 1480
ds2api_request_duration_ms_bucket{surface="chat",le="5000"} 1520
ds2api_request_duration_ms_bucket{surface="chat",le="+Inf"} 1523
ds2api_request_duration_ms_sum{surface="chat"} 1234567
ds2api_request_duration_ms_count{surface="chat"} 1523
```

**Gauges** (atomic.Int64):
```
ds2api_account_pool_in_use 3
ds2api_account_pool_available 2
ds2api_account_pool_waiting 1
ds2api_account_pool_total 5
ds2api_tokens_total{type="prompt"} 12345678
ds2api_tokens_total{type="completion"} 5678901
ds2api_tokens_total{type="reasoning"} 456789
ds2api_empty_output_retries_total 15
ds2api_account_switch_retries_total 3
ds2api_active_sessions 42
```

**Serialization**: `GET /metrics` returns Prometheus text format (Content-Type: text/plain; version=0.0.4).

Each label combination uses a map[string]*atomic.Int64 keyed by label string like `surface=chat,model=v4-pro,status=200`.

### 2. Webhook Alerter (`internal/monitor/alerter.go`)

**Design**: Non-blocking, rate-limited alert dispatcher with exponential backoff per channel.

```go
type Alerter struct {
    config    AlertingConfig
    cooldowns map[string]time.Time  // channel -> last alert time
    mu        sync.Mutex
    http      *http.Client
}

func (a *Alerter) Alert(event AlertEvent) {
    // Check if alerting is enabled for this event type
    // Check if channel is on cooldown (rate limit)
    // Fire alert asynchronously via goroutine
    // Update cooldown timestamp
}
```

**Alert Types**:
| Event | Severity | Message Template |
|-------|----------|-----------------|
| `all_accounts_down` | 🔴 CRITICAL | "🚨 ALL DeepSeek accounts are DOWN. No requests can be processed." |
| `high_error_rate` | 🟠 WARNING | "⚠️ Error rate is {rate}% over last {window}s (threshold: {threshold}%)." |
| `consecutive_failures` | 🟠 WARNING | "⚠️ {count} consecutive upstream failures from account {account}." |
| `session_failure` | 🟡 INFO | "⚠️ Session creation failed for account {account}: {error}" |
| `pow_failure` | 🟡 INFO | "⚠️ PoW failure for account {account}" |
| `content_filter` | 🟡 INFO | "⚠️ Content filter triggered for account {account}" |
| `token_refresh_failure` | 🟠 WARNING | "⚠️ Token refresh failed for account {account}: {error}" |
| `account_recovered` | 🟢 INFO | "✅ Account {account} has recovered. Health restored." |

**Channel Formatters**:
- **Discord**: Rich embed with color-coded severity, timestamp, fields for account/model/tokens
- **Slack**: Block Kit message with color bar, structured fields
- **Telegram**: HTML-formatted message with emoji indicators
- **Custom**: JSON POST body with event type, severity, message, metadata

### 3. Hooks (`internal/monitor/hooks.go`)

This is the integration layer. Instead of modifying existing code everywhere, we provide a few hook functions that get called from key points:

```go
// Called at the start of every request
func RecordRequestStart(surface, model string)
// Returns a function to defer for completion
// -> Returns stopwatch func

// Called at request completion
func RecordRequestEnd(surface, model string, statusCode int, elapsedMs int64,
    promptTokens, completionTokens, reasoningTokens int,
    retryCount int, accountID string)

// Called when account pool state changes
func RecordAccountPool(inUse, available, waiting, total int)

// Called on error events
func RecordErrorEvent(event AlertEvent)

// Called when all accounts are detected as unhealthy
func RecordAllAccountsDown()
```

**Integration Points** (existing code — minimal changes):

| Location | Change |
|----------|--------|
| `internal/completionruntime/nonstream.go` | Call `monitor.RecordRequestStart/End` around completion |
| `internal/completionruntime/stream_retry.go` | Call `monitor.RecordRequestStart/End` in Finalize hook |
| `internal/httpapi/openai/embeddings/embeddings_handler.go` | Call `monitor.RecordRequestStart/End` |
| `internal/account/pool_core.go` `Acquire()` / `Release()` | Call `monitor.RecordAccountPool()` |
| `internal/deepseek/client/client_auth.go` | Call `monitor.RecordErrorEvent()` on create session / get PoW failures |
| `internal/auth/request.go` | Call `monitor.RecordErrorEvent()` on token refresh failures |
| `internal/server/router.go` | Add `GET /metrics` handler |

### 4. Vercel Cron Health Check

Add a self-health-check endpoint that the Vercel Cron Job calls every minute:

```
GET /admin/monitor/health-check
Authorization: Bearer <internal-cron-secret>

→ Tests: can we create a DeepSeek session? Are any accounts healthy?
→ If unhealthy: fires alert webhook
→ Returns: { "healthy": true/false, "accounts_ok": 3, "accounts_down": 1 }
```

`vercel.json` addition:
```json
{
  "crons": [
    {
      "path": "/admin/monitor/health-check",
      "schedule": "* * * * *"
    }
  ]
}
```

### 5. WebUI Admin Settings

New section in the Settings page:

```
⚙️ Monitoring Settings
┌────────────────────────────────────────────────────────────┐
│ ☑ Enable Prometheus /metrics endpoint                      │
│   Path: [/metrics                ]                         │
│                                                            │
│ ☑ Enable Alerting                                          │
│   Rate limit: [60    ] seconds between alerts              │
│                                                            │
│ ─── Alert Channels ─────────────────────────────────────── │
│                                                            │
│ ☐ Discord Webhook                              [Configure] │
│ ☐ Slack Webhook                               [Configure] │
│ ☐ Telegram Bot                                [Configure] │
│ ☐ Custom Webhook                              [Configure] │
│                                                            │
│ ─── Alert Triggers ─────────────────────────────────────── │
│                                                            │
│ ☑ All accounts down (CRITICAL)                             │
│ ☑ High error rate (threshold: [30]%)                       │
│ ☑ Consecutive upstream failures (threshold: [10])          │
│ ☑ Session creation failure                                 │
│ ☑ PoW failure                                              │
│ ☑ Content filter block                                     │
│ ☑ Token refresh failure                                    │
│                                                            │
│ [Test Alert]                              [Save Settings]  │
└────────────────────────────────────────────────────────────┘
```

---

## Vercel-Specific Considerations

| Concern | Solution |
|---------|----------|
| **Prometheus can't scrape ephemeral functions** | `/metrics` endpoint works for local/Docker. For Vercel, users deploy Pushgateway separately and configure `PROMETHEUS_PUSHGATEWAY_URL`. Metrics are pushed periodically via Vercel Cron Job. |
| **Metric persistence across cold starts** | Counters reset on cold start — acceptable for serverless. For long-term aggregation, use the usage log SQLite + Turso from prior plan. |
| **Alert spam on frequent errors** | Rate limit per channel (configurable `rate_limit_seconds`). Cooldown prevents duplicate alerts within the window. |
| **Cron Job cold starts** | The `/admin/monitor/health-check` endpoint is lightweight — just checks if any accounts can create a session. Fast enough for 1-minute cron interval. |

---

## Implementation Order

### Step 1: Metrics Engine
- Create `internal/monitor/metrics.go` — atomic counters, histograms, gauges, Prometheus text serializer
- Create `internal/monitor/metrics_test.go` — verify serialization format, concurrent access
- Register `/metrics` route in `internal/server/router.go`
- Register `/metrics` path in `vercel.json` rewrites (for Vercel deployers who use external Pushgateway instead)

### Step 2: Alerter Engine
- Create `internal/monitor/alerter.go` — Discord/Slack/Telegram/custom webhook dispatch
- Create `internal/monitor/alerter_test.go` — rate limiting, message formatting
- Add config parsing in `internal/config/monitor.go` (new file)

### Step 3: Hooks Integration
- Create `internal/monitor/hooks.go` — wrapper functions
- Inject calls into `completionruntime`, `account/pool_core.go`, `deepseek/client/client_auth.go`, `auth/request.go`
- Create `internal/monitor/hooks_test.go`

### Step 4: Health Check + Cron
- Create `internal/httpapi/admin/monitor/handler.go`
- Add health-check endpoint
- Add admin settings endpoint
- Register in `internal/httpapi/admin/handler.go`
- Add cron job to `vercel.json`

### Step 5: WebUI Settings
- Create `webui/src/features/settings/MonitorSection.jsx`
- Add to `webui/src/features/settings/SettingsContainer.jsx`
- Add i18n strings (en + zh)

### Step 6: Config
- Add `monitor` section to `config.example.json`
- Add validation in `internal/config/validation.go`
- Add accessors in `internal/config/store_accessors.go`

---

## Success Criteria

1. `GET /metrics` returns valid Prometheus text format with all counters/gauges/histograms
2. When all accounts are down, a Discord/Slack/Telegram message arrives within 60 seconds
3. WebUI shows monitoring settings page with all configuration options
4. Health check endpoint correctly reports account status
5. Rate limiting prevents alert spam (max 1 alert per channel per minute)
6. Zero performance impact on normal request path (metrics are atomic operations only)
7. Works identically on local Docker and Vercel deployments
