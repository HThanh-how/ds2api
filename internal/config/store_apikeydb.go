package config

import (
	"context"
	"slices"
	"strings"
	"time"

	"ds2api/internal/apikeydb"
)

func (s *Store) SetAPIKeyTurso(c *apikeydb.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKeyTurso = c
}

// SyncAPIKeysToTursoNow pushes current config API keys to Turso (full replace).
func (s *Store) SyncAPIKeysToTursoNow(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	cl := s.apiKeyTurso
	if cl == nil || !cl.Enabled() {
		s.mu.RUnlock()
		return nil
	}
	rows := apiKeyRowsFromConfigLocked(s.cfg)
	s.mu.RUnlock()
	return cl.Sync(ctx, rows)
}

func (s *Store) pushAPIKeysToTursoAsync() {
	if s == nil || s.apiKeyTurso == nil || !s.apiKeyTurso.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.SyncAPIKeysToTursoNow(ctx); err != nil {
		Logger.Warn("[api_key_db] background sync failed", "error", err.Error())
	}
}

func apiKeyRowsFromConfigLocked(c Config) []apikeydb.Row {
	out := make([]apikeydb.Row, 0, len(c.APIKeys))
	for _, k := range c.APIKeys {
		if kk := strings.TrimSpace(k.Key); kk != "" {
			out = append(out, apikeydb.Row{Key: kk, Name: k.Name, Remark: k.Remark})
		}
	}
	slices.SortFunc(out, func(a, b apikeydb.Row) int {
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

func normalizedAPIKeyRows(c Config) []apikeydb.Row {
	cc := c.Clone()
	cc.NormalizeCredentials()
	return apiKeyRowsFromConfigLocked(cc)
}

func apiKeyRowsEqual(a, b []apikeydb.Row) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name || a[i].Remark != b[i].Remark {
			return false
		}
	}
	return true
}
