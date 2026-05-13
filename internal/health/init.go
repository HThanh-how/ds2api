package health

import "ds2api/internal/config"

var globalTracker *Tracker

func Init(tursoURL, tursoToken string) *Tracker {
	var tc *tursoClient
	if tursoURL != "" && tursoToken != "" {
		tc = newTursoClient(tursoURL, tursoToken)
		if err := ensureTable(tc); err != nil {
			config.Logger.Warn("[health] turso init failed", "error", err)
			tc = nil
		}
	}

	persist := func(h *AccountHealth) {
		if tc != nil {
			persistToTurso(tc, h)
		}
	}

	tracker := NewTracker(persist)

	if tc != nil {
		entries, err := loadFromTurso(tc)
		if err != nil {
			config.Logger.Warn("[health] turso load failed", "error", err)
		} else if len(entries) > 0 {
			tracker.Load(entries)
			config.Logger.Info("[health] loaded from turso", "accounts", len(entries))
		}
	}

	globalTracker = tracker
	return tracker
}

func Global() *Tracker {
	return globalTracker
}
