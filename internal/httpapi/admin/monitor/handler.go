package adminmonitor

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
	"ds2api/internal/monitor"
)

type Handler struct {
	Store adminshared.ConfigStore
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/monitor/settings", h.GetSettings)
	r.Put("/monitor/settings", h.PutSettings)
	r.Get("/monitor/health-check", h.HealthCheck)
}

func (h *Handler) GetSettings(w http.ResponseWriter, _ *http.Request) {
	if h.Store == nil {
		adminshared.WriteJSON(w, http.StatusOK, config.DefaultMonitorConfig())
		return
	}
	adminshared.WriteJSON(w, http.StatusOK, h.Store.MonitorConfig())
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var cfg config.MonitorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		adminshared.WriteJSON(w, http.StatusBadRequest, map[string]string{"detail": "invalid monitor config: " + err.Error()})
		return
	}
	if err := h.Store.Update(func(c *config.Config) error {
		c.Monitor = cfg
		return nil
	}); err != nil {
		adminshared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"detail": "failed to save monitor config: " + err.Error()})
		return
	}
	adminshared.WriteJSON(w, http.StatusOK, cfg)
}

func (h *Handler) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	accounts := h.Store.Accounts()
	healthy := 0
	for _, acc := range accounts {
		if acc.Token != "" {
			healthy++
		}
	}
	status := "healthy"
	if healthy == 0 && len(accounts) > 0 {
		status = "unhealthy"
		monitor.OnAllAccountsDown()
	}
	adminshared.WriteJSON(w, http.StatusOK, map[string]any{
		"healthy":        status == "healthy",
		"accounts_ok":    healthy,
		"accounts_down":  len(accounts) - healthy,
		"uptime_seconds": monitor.UptimeSeconds(),
	})
}
