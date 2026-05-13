package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"ds2api/internal/config"
)

// ErrAPIKeyRevoked is returned when a credential was a configured API key but
// has been revoked; the request must not fall through to DeepSeek passthrough.
var ErrAPIKeyRevoked = errors.New("api key revoked")

var (
	apiKeyPositiveTTL  = 24 * time.Hour // max lifetime of a positive cache entry
	apiKeyRevalidate   = time.Hour      // minimum interval between store lookups for a hot key
	apiKeyRevokedBlock = 24 * time.Hour // block deleted keys quickly after removal
)

// APIKeyCache implements a small in-process verification layer on top of
// config.Store API keys: positive entries reduce HasAPIKey pressure; revoked
// entries keep recently deleted keys from being treated as passthrough tokens.
type APIKeyCache struct {
	mu       sync.Mutex
	positive map[string]*apiKeyPosEntry
	revoked  map[string]time.Time // key hash -> block until (wall clock)
}

type apiKeyPosEntry struct {
	lastVerified time.Time
	expiresAt    time.Time
}

func NewAPIKeyCache() *APIKeyCache {
	return &APIKeyCache{
		positive: make(map[string]*apiKeyPosEntry),
		revoked:  make(map[string]time.Time),
	}
}

func apiKeyHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (c *APIKeyCache) sweepExpiredLocked(now time.Time) {
	for h, until := range c.revoked {
		if !now.Before(until) {
			delete(c.revoked, h)
		}
	}
	for h, e := range c.positive {
		if e == nil || !now.Before(e.expiresAt) {
			delete(c.positive, h)
		}
	}
}

// InvalidateOne drops the positive entry for a single key (e.g. after add).
func (c *APIKeyCache) InvalidateOne(raw string) {
	if c == nil || raw == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.positive, apiKeyHash(raw))
}

// InvalidateAllPositive drops all positive entries (e.g. bulk config change).
func (c *APIKeyCache) InvalidateAllPositive() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.positive = make(map[string]*apiKeyPosEntry)
}

// RegisterRevokedKey blocks this exact key string for apiKeyRevokedBlock.
func (c *APIKeyCache) RegisterRevokedKey(raw string) {
	if c == nil || raw == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.sweepExpiredLocked(now)
	h := apiKeyHash(raw)
	delete(c.positive, h)
	c.revoked[h] = now.Add(apiKeyRevokedBlock)
}

// ClearRevokedKey removes a revocation entry (e.g. same key re-added legitimately).
func (c *APIKeyCache) ClearRevokedKey(raw string) {
	if c == nil || raw == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.revoked, apiKeyHash(raw))
}

// ManagedByConfigStore reports whether raw is a configured ds2api API key.
// If false and err is nil, the caller should treat raw as a passthrough DeepSeek token.
func (c *APIKeyCache) ManagedByConfigStore(store *config.Store, raw string) (bool, error) {
	if store == nil {
		return false, nil
	}
	if c == nil {
		return store.HasAPIKey(raw), nil
	}
	h := apiKeyHash(raw)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.sweepExpiredLocked(now)
	if until, ok := c.revoked[h]; ok && now.Before(until) {
		return false, ErrAPIKeyRevoked
	}
	if e, ok := c.positive[h]; ok && now.Before(e.expiresAt) {
		if now.Sub(e.lastVerified) < apiKeyRevalidate {
			return true, nil
		}
		if !store.HasAPIKey(raw) {
			delete(c.positive, h)
			c.revoked[h] = now.Add(apiKeyRevokedBlock)
			return false, ErrAPIKeyRevoked
		}
		e.lastVerified = now
		e.expiresAt = now.Add(apiKeyPositiveTTL)
		return true, nil
	}
	delete(c.positive, h)
	if store.HasAPIKey(raw) {
		c.positive[h] = &apiKeyPosEntry{lastVerified: now, expiresAt: now.Add(apiKeyPositiveTTL)}
		return true, nil
	}
	return false, nil
}
