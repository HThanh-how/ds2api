
# DS2API Usage Analytics — Vercel Deployment Strategy

> **Date**: 2026-05-12
> **Context**: User is deploying on Vercel. The [standalone SQLite plan](./usage-analytics-plan.md) assumes a persistent filesystem and long-running goroutines — neither exists on Vercel. This document provides the adapted, Vercel-optimized solution.
> **Scope**: Suggestions only, no code changes.

---

## 1. Why the Original SQLite Plan Doesn't Work on Vercel

| Assumption in Standalone Plan | Reality on Vercel |
|-------------------------------|-------------------|
| Persistent writable filesystem at `./data/usage.db` | Ephemeral `/tmp` only — data is lost on cold start or instance rotation |
| Background goroutine for hourly TTL cleanup | No persistent background processes — serverless functions terminate after response |
| Long-lived `*sql.DB` connection pool | Each function invocation is short-lived; must open/close per request |
| Single-node writer (WAL mode safe) | Multiple concurrent Lambda instances could write concurrently |

For Vercel deployment, we need an **external, cloud-hosted database** with a generous free tier.

---

## 2. Best Solution: Turso (libSQL) — Managed SQLite at the Edge

### Why Turso is the #1 Choice

| Criteria | Turso Free Tier | Why It Fits |
|----------|----------------|-------------|
| **Storage** | 1 GB | ~10 million request logs (each ~100 bytes) — years of data for personal use |
| **Reads** | 1 billion row reads/month | Aggregation queries, dashboard, log browsing all covered |
| **Writes** | 25 million row writes/month | Each request = 1 INSERT; 25M requests/month ≈ 800K requests/day |
| **Protocol** | libSQL (SQLite wire protocol) | Reuses the exact same schema from the [standalone plan](./usage-analytics-plan.md#3-database-schema-design) — zero schema changes |
| **Go client** | `github.com/tursodatabase/go-libsql` | Mature, serverless-friendly (stateless HTTP connections) |
| **Edge deployment** | Global replicas | Fast reads from Vercel's edge network |
| **TTL cleanup** | Vercel Cron Job calling `DELETE WHERE created_at < ...` | Replaces background goroutine from standalone plan |
| **Cost** | $0 forever | No credit card needed for free tier |

### Turso Architecture for Vercel

```
┌─────────────────────────────────────────────────────┐
│                    Vercel                            │
│  ┌──────────┐   ┌──────────────┐   ┌────────────┐  │
│  │ WebUI    │   │ Go Function   │   │ Cron Job   │  │
│  │ (static) │   │ api/index.go  │   │ (scheduled)│  │
│  └────┬─────┘   └──────┬───────┘   └─────┬──────┘  │
│       │                │                  │         │
│       │  GET /admin/   │  INSERT on       │ DELETE  │
│       │  usage/*       │  every request   │ old rows│
│       │                │                  │         │
└───────┼────────────────┼──────────────────┼─────────┘
        │                │                  │
        ▼                ▼                  ▼
   ┌─────────────────────────────────────────────────┐
   │              Turso (libSQL Cloud)                │
   │  ┌───────────────────────────────────────────┐  │
   │  │  usage_log table                           │  │
   │  │  (identical schema to standalone plan)     │  │
   │  └───────────────────────────────────────────┘  │
   │  Free: 1 GB storage | 1B reads | 25M writes/mo  │
   └─────────────────────────────────────────────────┘
```

### Go Code Pattern (Serverless-Aware)

The `internal/usagelog` package is identical to the standalone plan **except**:

1. **Connection lifecycle**: Instead of a persistent `*sql.DB` pool, each function invocation opens a new Turso HTTP connection, performs its operation, and closes it.
2. **No cleanup goroutine**: Cleanup is handled by a separate Vercel Cron Job endpoint.
3. **Async writes become sync**: Since the Turso write is an HTTP call to an edge database, latency is very low (typically <10ms to the nearest edge). Synchronous writes are acceptable in serverless — especially with Go's fast startup.

```go
// pseudocode: internal/usagelog/store.go (Vercel adaptation)
func (l *Logger) Record(ctx context.Context, params LogParams) error {
    db, err := sql.Open("libsql", l.databaseURL+"/?authToken="+l.authToken)
    if err != nil {
        return err
    }
    defer db.Close()
    
    _, err = db.ExecContext(ctx, `
        INSERT INTO usage_log (...) VALUES (...)`,
        params.ID, params.CreatedAt, params.CallerID, /* ... */
    )
    return err
}
```

---

## 3. Alternative: Upstash Redis

| Criteria | Upstash Redis Free Tier | Trade-off vs Turso |
|----------|------------------------|---------------------|
| **Storage** | 256 MB | Enough for moderate usage (1-2M requests) |
| **Commands** | 10,000/day | Tight for heavy usage (each request = 3-5 Redis commands) |
| **TTL** | Built-in `EXPIRE` | Simpler than manual cleanup — just set TTL on each key |
| **Queries** | No SQL; must pre-aggregate with INCR counters | Harder to do ad-hoc queries like "show me all requests between 2-3 PM with model v4-pro" |
| **Setup** | Simply set REDIS_URL env var | Less schema work, but harder to query |

### When to Choose Upstash Over Turso

- You have **very low request volume** (<2,000 requests/day — fits within 10K Redis commands)
- You **don't need complex aggregation queries** — just want a simple log table
- You prefer **zero schema management** and built-in TTL

### Upstash Redis Pattern

```go
// Each request → two Redis operations
key := fmt.Sprintf("usage:%s", requestID)
hset := client.HSet(ctx, key,
    "caller_id", params.CallerID,
    "account_id", params.AccountID,
    "model", params.Model,
    "prompt_tokens", params.PromptTokens,
    "output_tokens", params.OutputTokens,
    "total_cost", params.TotalCost,
    // ...
)
client.Expire(ctx, key, 30*24*time.Hour) // auto-delete after 30 days

// For time-based listing, maintain a sorted set
client.ZAdd(ctx, "usage:by_time", redis.Z{
    Score:  float64(params.CreatedAt),
    Member: params.ID,
})
client.Expire(ctx, "usage:by_time", 30*24*time.Hour)
```

---

## 4. Cleanup Strategy: Vercel Cron Jobs

Replaces the background goroutine from the standalone plan.

### Configuration in `vercel.json`

```json
{
  "crons": [
    {
      "path": "/admin/usage/cleanup",
      "schedule": "0 * * * *"
    }
  ]
}
```

### Cleanup Endpoint

```
POST /admin/usage/cleanup
Authorization: Bearer <internal-cron-secret>

→ Connects to Turso/Upstash
→ Deletes records older than retention_days
→ Returns { "deleted": 1234, "retention_days": 30 }
```

- Cron Job fires every hour (or daily — user configurable)
- Uses an internal secret for authentication (not exposed to users)
- For **Turso**: runs a simple `DELETE FROM usage_log WHERE created_at < ?`
- For **Upstash**: keys auto-expire via `EXPIRE` — cleanup is automatic; the cron endpoint is optional (just for monitoring/health)

---

## 5. Vercel Route Registration

New admin routes must be registered in `vercel.json` rewrites:

```json
{
  "source": "/admin/usage(.*)",
  "destination": "/api/index"
}
```

And in `internal/httpapi/admin/handler.go`, the new usage handler routes:

```go
adminusage.RegisterRoutes(pr, usageHandler)
```

The WebUI static files are already served at `/admin/*` — no change needed.

---

## 6. Cost Comparison

| Component | Free Tier | Monthly Cost | What It Gives You |
|-----------|-----------|-------------|-------------------|
| **Vercel** | Hobby | $0 | Serverless hosting + 100 GB-hours |
| **Turso** | Free | $0 | 1 GB storage, 1B reads, 25M writes |
| **Upstash Redis** (alt) | Free | $0 | 256 MB, 10K commands/day |
| **Total** | — | **$0/month** | Full usage analytics with auto-cleanup |

For teams with higher volume, Turso's paid tiers start at $9/month for 500 GB storage.

---

## 7. Updated Implementation Roadmap

### Phase 1 — Database Setup
1. Create Turso database via `turso db create ds2api-usage`
2. Run schema migration (identical to standalone plan's CREATE TABLE statements)
3. Add Turso connection URL + auth token to `config.json`:
   ```json
   "usage_log": {
     "enabled": true,
     "provider": "turso",
     "turso_url": "libsql://ds2api-usage-org.turso.io",
     "turso_auth_token": "${TURSO_AUTH_TOKEN}",
     "retention_days": 30
   }
   ```
   Or via Vercel environment variables:
   ```
   TURSO_DATABASE_URL=libsql://ds2api-usage-org.turso.io
   TURSO_AUTH_TOKEN=your-token-here
   ```

### Phase 2 — Backend
4. Add `github.com/tursodatabase/go-libsql` to `go.mod`
5. Create `internal/usagelog/` package (serverless-aware: open/close per request)
6. Create `internal/httpapi/admin/usage/` handler
7. Register routes in `vercel.json` and Go router
8. Hook into `completionruntime` (identical hook points as standalone plan)

### Phase 3 — Cleanup
9. Create `POST /admin/usage/cleanup` endpoint
10. Add Vercel Cron Job to `vercel.json` with hourly schedule

### Phase 4 — WebUI (unchanged from standalone plan)
11. Add `📈 Usage` sidebar entry
12. Create `webui/src/features/usage/` components
13. i18n strings

---

## 8. Summary — Recommended Approach

| # | What | Database | Why |
|---|------|----------|-----|
| 1 | **Primary: Turso (libSQL)** | Cloud-hosted SQLite, edge-distributed | Full SQL, generous free tier, same schema as standalone plan |
| 2 | **Alternative: Upstash Redis** | Managed Redis, built-in TTL | Simpler setup, but limited query power and tighter free tier |
| 3 | **Not recommended: Vercel KV** | Vercel's KV (Redis) | Free tier too small (256 MB) for detailed usage logs |
| 4 | **Not recommended: Vercel Postgres** | Serverless Postgres | Free tier expires after 12 months |

**Go with Turso** if you want:
- Full SQL queries (aggregation, filtering, GROUP BY by hour/model/caller)
- The same schema from the standalone plan — no redesign needed
- 1 GB free storage (vs 256 MB on Upstash/KV)
- Enough free capacity for even heavy usage (25M writes/month)

**Go with Upstash Redis** if you want:
- Simpler code (just SET/GET/ZADD)
- Built-in TTL (no cron job needed for cleanup)
- But you trade off query flexibility and have a tighter free tier limit

The WebUI, cost estimation, and per-key breakdown sections from the [standalone plan](./usage-analytics-plan.md) all apply unchanged.
