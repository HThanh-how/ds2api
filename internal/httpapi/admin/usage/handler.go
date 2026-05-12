package adminusage

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/usagelog"
)

type Handler struct{}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/usage/log", h.Log)
	r.Get("/usage/summary", h.Summary)
	r.Get("/usage/caller-summary", h.CallerSummary)
	r.Delete("/usage/log", h.Clear)
	r.Get("/usage/count", h.Count)
}

func (h *Handler) Log(w http.ResponseWriter, r *http.Request) {
	store := usagelog.GlobalStore()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if to <= 0 {
		to = time.Now().UTC().UnixMilli()
	}
	if from <= 0 {
		from = to - 86400000 // default: last 24 hours
	}

	entries, total := store.Query(usagelog.QueryParams{
		From:     from,
		To:       to,
		CallerID: r.URL.Query().Get("caller"),
		Model:    r.URL.Query().Get("model"),
		Surface:  r.URL.Query().Get("surface"),
		Page:     page,
		Limit:    limit,
	})

	if entries == nil {
		entries = []usagelog.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": entries,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	store := usagelog.GlobalStore()
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if to <= 0 {
		to = time.Now().UTC().UnixMilli()
	}
	if from <= 0 {
		from = time.Now().UTC().Add(-48 * time.Hour).UnixMilli()
	}

	summaries, err := store.Summary(from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}
	if summaries == nil {
		summaries = []usagelog.Summary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": summaries,
		"total": len(summaries),
	})
}

func (h *Handler) CallerSummary(w http.ResponseWriter, r *http.Request) {
	store := usagelog.GlobalStore()
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if to <= 0 {
		to = time.Now().UTC().UnixMilli()
	}
	if from <= 0 {
		from = time.Now().UTC().Add(-48 * time.Hour).UnixMilli()
	}

	summaries, err := store.CallerSummary(from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		return
	}
	if summaries == nil {
		summaries = []usagelog.CallerSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": summaries,
		"total": len(summaries),
	})
}

func (h *Handler) Clear(w http.ResponseWriter, _ *http.Request) {
	store := usagelog.GlobalStore()
	store.Clear()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *Handler) Count(w http.ResponseWriter, _ *http.Request) {
	store := usagelog.GlobalStore()
	count := store.EntriesCount()
	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
