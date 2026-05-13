package health

import (
	"sync"
	"time"
)

type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusMuted       Status = "muted"
	StatusRateLimited Status = "rate_limited"
	StatusLoginFailed Status = "login_failed"
	StatusCooldown    Status = "cooldown"
)

type AccountHealth struct {
	AccountID           string  `json:"account_id"`
	HealthScore         float64 `json:"health_score"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	LastFailureReason   string  `json:"last_failure_reason"`
	LastFailureAt       int64   `json:"last_failure_at"`
	LastSuccessAt       int64   `json:"last_success_at"`
	CooldownUntil       int64   `json:"cooldown_until"`
	MuteUntil           int64   `json:"mute_until"`
	TotalRequests       int64   `json:"total_requests"`
	TotalFailures       int64   `json:"total_failures"`
	UpdatedAt           int64   `json:"updated_at"`
	Status              Status  `json:"status"`
}

type Tracker struct {
	mu      sync.RWMutex
	entries map[string]*AccountHealth
	persist func(h *AccountHealth)
}

func NewTracker(persist func(h *AccountHealth)) *Tracker {
	return &Tracker{
		entries: make(map[string]*AccountHealth),
		persist: persist,
	}
}

func (t *Tracker) getOrCreate(accountID string) *AccountHealth {
	h, ok := t.entries[accountID]
	if !ok {
		h = &AccountHealth{
			AccountID:   accountID,
			HealthScore: 100,
			UpdatedAt:   time.Now().Unix(),
		}
		t.entries[accountID] = h
	}
	return h
}

func (t *Tracker) RecordSuccess(accountID string) {
	if accountID == "" {
		return
	}
	t.mu.Lock()
	h := t.getOrCreate(accountID)
	h.ConsecutiveFailures = 0
	h.TotalRequests++
	h.LastSuccessAt = time.Now().Unix()
	h.HealthScore = clampScore(h.HealthScore + 10)
	h.UpdatedAt = time.Now().Unix()
	if h.CooldownUntil > 0 && time.Now().Unix() >= h.CooldownUntil {
		h.CooldownUntil = 0
	}
	if h.MuteUntil > 0 && time.Now().Unix() >= h.MuteUntil {
		h.MuteUntil = 0
	}
	h.Status = computeStatus(h)
	snapshot := *h
	t.mu.Unlock()
	t.doPersist(&snapshot)
}

func (t *Tracker) RecordMute(accountID string, muteUntil float64) {
	if accountID == "" {
		return
	}
	t.mu.Lock()
	h := t.getOrCreate(accountID)
	h.ConsecutiveFailures++
	h.TotalRequests++
	h.TotalFailures++
	h.LastFailureReason = "user is muted"
	h.LastFailureAt = time.Now().Unix()
	h.HealthScore = 0

	now := time.Now().Unix()
	muteTS := int64(muteUntil)
	if muteTS <= now {
		muteTS = now + 3600
	}
	h.MuteUntil = muteTS
	h.CooldownUntil = muteTS

	h.UpdatedAt = now
	h.Status = StatusMuted
	snapshot := *h
	t.mu.Unlock()
	t.doPersist(&snapshot)
}

func (t *Tracker) RecordRateLimit(accountID string) {
	if accountID == "" {
		return
	}
	t.mu.Lock()
	h := t.getOrCreate(accountID)
	h.ConsecutiveFailures++
	h.TotalRequests++
	h.TotalFailures++
	h.LastFailureReason = "rate_limited"
	h.LastFailureAt = time.Now().Unix()
	h.HealthScore = clampScore(h.HealthScore - 25)
	h.CooldownUntil = time.Now().Unix() + 60
	h.UpdatedAt = time.Now().Unix()
	h.Status = StatusRateLimited
	snapshot := *h
	t.mu.Unlock()
	t.doPersist(&snapshot)
}

func (t *Tracker) RecordLoginFailure(accountID string) {
	if accountID == "" {
		return
	}
	t.mu.Lock()
	h := t.getOrCreate(accountID)
	h.ConsecutiveFailures++
	h.TotalRequests++
	h.TotalFailures++
	h.LastFailureReason = "login_failed"
	h.LastFailureAt = time.Now().Unix()
	h.HealthScore = 0
	h.CooldownUntil = time.Now().Unix() + 1800
	h.UpdatedAt = time.Now().Unix()
	h.Status = StatusLoginFailed
	snapshot := *h
	t.mu.Unlock()
	t.doPersist(&snapshot)
}

func (t *Tracker) RecordUploadFailure(accountID string) {
	if accountID == "" {
		return
	}
	t.mu.Lock()
	h := t.getOrCreate(accountID)
	h.ConsecutiveFailures++
	h.TotalRequests++
	h.TotalFailures++
	h.LastFailureReason = "upload_failed"
	h.LastFailureAt = time.Now().Unix()
	h.HealthScore = clampScore(h.HealthScore - 15)
	h.CooldownUntil = time.Now().Unix() + 30
	h.UpdatedAt = time.Now().Unix()
	h.Status = computeStatus(h)
	snapshot := *h
	t.mu.Unlock()
	t.doPersist(&snapshot)
}

func (t *Tracker) IsAvailable(accountID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	h, ok := t.entries[accountID]
	if !ok {
		return true
	}
	if h.CooldownUntil <= 0 {
		return true
	}
	return time.Now().Unix() >= h.CooldownUntil
}

func (t *Tracker) GetHealthScore(accountID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	h, ok := t.entries[accountID]
	if !ok {
		return 100
	}
	if h.CooldownUntil > 0 && time.Now().Unix() >= h.CooldownUntil {
		return clampScore(h.HealthScore + 10)
	}
	return h.HealthScore
}

func (t *Tracker) CooldownUntil(accountID string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	h, ok := t.entries[accountID]
	if !ok {
		return 0
	}
	return h.CooldownUntil
}

func (t *Tracker) GetAllHealth() []AccountHealth {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now().Unix()
	result := make([]AccountHealth, 0, len(t.entries))
	for _, h := range t.entries {
		entry := *h
		entry.Status = computeStatusAt(h, now)
		result = append(result, entry)
	}
	return result
}

func (t *Tracker) GetHealth(accountID string) (AccountHealth, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	h, ok := t.entries[accountID]
	if !ok {
		return AccountHealth{}, false
	}
	entry := *h
	entry.Status = computeStatusAt(h, time.Now().Unix())
	return entry, true
}

func (t *Tracker) Load(entries []AccountHealth) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range entries {
		e := entries[i]
		e.Status = computeStatusAt(&e, time.Now().Unix())
		t.entries[e.AccountID] = &e
	}
}

func (t *Tracker) doPersist(h *AccountHealth) {
	if t.persist == nil {
		return
	}
	t.persist(h)
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func computeStatus(h *AccountHealth) Status {
	return computeStatusAt(h, time.Now().Unix())
}

func computeStatusAt(h *AccountHealth, now int64) Status {
	if h.MuteUntil > 0 && now < h.MuteUntil {
		return StatusMuted
	}
	if h.CooldownUntil > 0 && now < h.CooldownUntil {
		if h.LastFailureReason == "rate_limited" {
			return StatusRateLimited
		}
		if h.LastFailureReason == "login_failed" {
			return StatusLoginFailed
		}
		return StatusCooldown
	}
	return StatusHealthy
}
