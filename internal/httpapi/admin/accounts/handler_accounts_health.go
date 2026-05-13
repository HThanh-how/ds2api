package accounts

import (
	"net/http"
	"sort"

	"ds2api/internal/health"
)

func (h *Handler) accountsHealth(w http.ResponseWriter, _ *http.Request) {
	tracker := health.Global()
	if tracker == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	accounts := h.Store.Accounts()
	allHealth := tracker.GetAllHealth()
	healthMap := make(map[string]health.AccountHealth, len(allHealth))
	for _, ah := range allHealth {
		healthMap[ah.AccountID] = ah
	}

	results := make([]map[string]any, 0, len(accounts))
	for _, acc := range accounts {
		id := acc.Identifier()
		if id == "" {
			continue
		}
		ah, exists := healthMap[id]
		if !exists {
			ah = health.AccountHealth{
				AccountID:   id,
				HealthScore: 100,
				Status:      health.StatusHealthy,
			}
		}
		results = append(results, map[string]any{
			"account_id":           ah.AccountID,
			"name":                 acc.Name,
			"health_score":         ah.HealthScore,
			"status":               string(ah.Status),
			"consecutive_failures": ah.ConsecutiveFailures,
			"last_failure_reason":  ah.LastFailureReason,
			"last_failure_at":      ah.LastFailureAt,
			"last_success_at":      ah.LastSuccessAt,
			"cooldown_until":       ah.CooldownUntil,
			"mute_until":           ah.MuteUntil,
			"total_requests":       ah.TotalRequests,
			"total_failures":       ah.TotalFailures,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		si, _ := results[i]["health_score"].(float64)
		sj, _ := results[j]["health_score"].(float64)
		return si > sj
	})

	writeJSON(w, http.StatusOK, results)
}
