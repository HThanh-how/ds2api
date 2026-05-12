package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
	SeverityRecovery Severity = "recovery"
)

type AlertEvent struct {
	Type     string   `json:"type"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Account  string   `json:"account,omitempty"`
	Model    string   `json:"model,omitempty"`
	Error    string   `json:"error,omitempty"`
	Rate     float64  `json:"rate,omitempty"`
	Count    int      `json:"count,omitempty"`
}

type Alerter struct {
	store    *config.Store
	client   *http.Client
	mu       sync.Mutex
	cooldown map[string]time.Time
	// Track consecutive failures per account for recovery detection.
	accountFailures map[string]int
	// Track if all-accounts-down alert was already sent (prevent repeats until recovery).
	allDownSent bool
}

func NewAlerter(store *config.Store) *Alerter {
	return &Alerter{
		store:           store,
		client:          &http.Client{Timeout: 10 * time.Second},
		cooldown:        map[string]time.Time{},
		accountFailures: map[string]int{},
	}
}

// notify sends the alert to all enabled channels if not on cooldown.
func (a *Alerter) notify(evt AlertEvent) {
	if a == nil || a.store == nil {
		return
	}
	cfg := a.store.MonitorAlerting()
	if !cfg.Enabled {
		return
	}
	key := fmt.Sprintf("%s:%s", evt.Severity, evt.Type)
	if !a.checkCooldown(key, cfg.RateLimitSeconds) {
		return
	}
	payload := a.formatPayload(evt)
	a.dispatch(payload, cfg)
}

func (a *Alerter) formatPayload(evt AlertEvent) map[string]any {
	color := "3066993"
	switch evt.Severity {
	case SeverityCritical:
		color = "15548997" // red
	case SeverityWarning:
		color = "16705372" // orange
	case SeverityInfo:
		color = "5763719" // green
	case SeverityRecovery:
		color = "65280" // bright green
	}
	fields := []map[string]any{}
	if evt.Account != "" {
		fields = append(fields, map[string]any{"name": "Account", "value": evt.Account, "inline": true})
	}
	if evt.Model != "" {
		fields = append(fields, map[string]any{"name": "Model", "value": evt.Model, "inline": true})
	}
	if evt.Error != "" {
		fields = append(fields, map[string]any{"name": "Error", "value": fmt.Sprintf("```%s```", evt.Error)})
	}
	if evt.Rate > 0 {
		fields = append(fields, map[string]any{"name": "Error Rate", "value": fmt.Sprintf("%.1f%%", evt.Rate*100), "inline": true})
	}
	if evt.Count > 0 {
		fields = append(fields, map[string]any{"name": "Count", "value": fmt.Sprintf("%d", evt.Count), "inline": true})
	}
	return map[string]any{
		"content":    nil,
		"username":   "DS2API Monitor",
		"avatar_url": "https://raw.githubusercontent.com/CJackHwang/ds2api/main/webui/public/ds2api-favicon.svg",
		"embeds": []map[string]any{{
			"title":       fmt.Sprintf("[%s] %s", strings.ToUpper(string(evt.Severity)), evt.Type),
			"description": evt.Message,
			"color":       mustParseInt(color),
			"fields":      fields,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}},
	}
}

func (a *Alerter) dispatch(payload map[string]any, cfg config.AlertingConfig) {
	if cfg.Channels.Discord.Enabled && strings.TrimSpace(cfg.Channels.Discord.WebhookURL) != "" {
		a.postWebhook(cfg.Channels.Discord.WebhookURL, payload, "Discord")
	}
	if cfg.Channels.Slack.Enabled && strings.TrimSpace(cfg.Channels.Slack.WebhookURL) != "" {
		a.postSlack(cfg.Channels.Slack.WebhookURL, payload)
	}
	if cfg.Channels.Telegram.Enabled && strings.TrimSpace(cfg.Channels.Telegram.BotToken) != "" && strings.TrimSpace(cfg.Channels.Telegram.ChatID) != "" {
		a.postTelegram(cfg.Channels.Telegram.BotToken, cfg.Channels.Telegram.ChatID, payload)
	}
	if cfg.Channels.Custom.Enabled && strings.TrimSpace(cfg.Channels.Custom.URL) != "" {
		a.postCustom(cfg.Channels.Custom.URL, cfg.Channels.Custom.Headers, payload)
	}
}

func (a *Alerter) postWebhook(url string, payload map[string]any, channel string) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		config.Logger.Warn("[monitor] dispatch "+channel+" request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		config.Logger.Warn("[monitor] dispatch "+channel+" failed", "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		config.Logger.Warn("[monitor] dispatch "+channel+" returned status", "status", resp.StatusCode)
	}
}

func (a *Alerter) postSlack(url string, payload map[string]any) {
	slackPayload := map[string]any{"text": extractSlackText(payload)}
	body, _ := json.Marshal(slackPayload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		config.Logger.Warn("[monitor] dispatch Slack request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		config.Logger.Warn("[monitor] dispatch Slack failed", "error", err)
		return
	}
	_ = resp.Body.Close()
}

func extractSlackText(payload map[string]any) string {
	embeds, _ := payload["embeds"].([]map[string]any)
	if len(embeds) == 0 {
		return "DS2API Alert"
	}
	e := embeds[0]
	title, _ := e["title"].(string)
	desc, _ := e["description"].(string)
	return fmt.Sprintf("*%s*\n%s", title, desc)
}

func (a *Alerter) postTelegram(botToken, chatID string, payload map[string]any) {
	text := extractTelegramText(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	body, _ := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		config.Logger.Warn("[monitor] dispatch Telegram failed", "error", err)
		return
	}
	_ = resp.Body.Close()
}

func extractTelegramText(payload map[string]any) string {
	embeds, _ := payload["embeds"].([]map[string]any)
	if len(embeds) == 0 {
		return "DS2API Alert"
	}
	e := embeds[0]
	title, _ := e["title"].(string)
	desc, _ := e["description"].(string)
	fields, _ := e["fields"].([]map[string]any)
	text := fmt.Sprintf("<b>%s</b>\n%s", title, desc)
	for _, f := range fields {
		name, _ := f["name"].(string)
		value, _ := f["value"].(string)
		text += fmt.Sprintf("\n<b>%s:</b> %s", name, value)
	}
	return text
}

func (a *Alerter) postCustom(url string, headers map[string]string, payload map[string]any) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		config.Logger.Warn("[monitor] dispatch Custom failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		config.Logger.Warn("[monitor] dispatch Custom failed", "error", err)
		return
	}
	_ = resp.Body.Close()
}

func (a *Alerter) checkCooldown(key string, rateLimitSeconds int) bool {
	if rateLimitSeconds <= 0 {
		rateLimitSeconds = 60
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	last, ok := a.cooldown[key]
	if ok && time.Since(last) < time.Duration(rateLimitSeconds)*time.Second {
		return false
	}
	a.cooldown[key] = time.Now()
	return true
}

// -- Public alerting methods --

func (a *Alerter) AlertAllAccountsDown() {
	if a.allDownSent {
		return
	}
	a.allDownSent = true
	a.notify(AlertEvent{
		Type:     "All Accounts Down",
		Severity: SeverityCritical,
		Message:  "🚨 ALL DeepSeek accounts are DOWN. No requests can be processed. Check account credentials and network connectivity immediately.",
	})
	RecordAlertEvent(string(SeverityCritical), "all_accounts_down")
}

func (a *Alerter) AlertAccountRecovered(account string) {
	a.allDownSent = false
	a.notify(AlertEvent{
		Type:     "Account Recovered",
		Severity: SeverityRecovery,
		Message:  fmt.Sprintf("✅ Account %s has recovered and is healthy again.", account),
		Account:  account,
	})
	RecordAlertEvent(string(SeverityRecovery), "account_recovered")
}

func (a *Alerter) AlertHighErrorRate(rate float64, windowSec int, threshold float64) {
	a.notify(AlertEvent{
		Type:     "High Error Rate",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("⚠️ Error rate is %.1f%% over the last %ds (threshold: %.1f%%).", rate*100, windowSec, threshold*100),
		Rate:     rate,
	})
	RecordAlertEvent(string(SeverityWarning), "high_error_rate")
}

func (a *Alerter) AlertConsecutiveFailures(account string, count int, threshold int) {
	if a == nil || a.store == nil {
		return
	}
	// Track failures internally.
	a.mu.Lock()
	a.accountFailures[account] = count
	a.mu.Unlock()
	if count >= threshold {
		a.notify(AlertEvent{
			Type:     "Consecutive Failures",
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("⚠️ %d consecutive upstream failures from account %s (threshold: %d).", count, account, threshold),
			Account:  account,
			Count:    count,
		})
		RecordAlertEvent(string(SeverityWarning), "consecutive_upstream_failures")
	}
}

func (a *Alerter) AlertSessionCreationFailure(account, errMsg string) {
	a.notify(AlertEvent{
		Type:     "Session Creation Failure",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Session creation failed for account %s.", account),
		Account:  account,
		Error:    errMsg,
	})
	RecordAlertEvent(string(SeverityInfo), "session_creation_failure")
}

func (a *Alerter) AlertPowFailure(account string) {
	a.notify(AlertEvent{
		Type:     "PoW Failure",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("PoW request failed for account %s.", account),
		Account:  account,
	})
	RecordAlertEvent(string(SeverityInfo), "pow_failure")
}

func (a *Alerter) AlertContentFilterBlock(account string) {
	a.notify(AlertEvent{
		Type:     "Content Filter Block",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Content filter triggered for account %s.", account),
		Account:  account,
	})
	RecordAlertEvent(string(SeverityInfo), "content_filter_block")
}

func (a *Alerter) AlertTokenRefreshFailure(account, errMsg string) {
	a.notify(AlertEvent{
		Type:     "Token Refresh Failure",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("Token refresh failed for account %s.", account),
		Account:  account,
		Error:    errMsg,
	})
	RecordAlertEvent(string(SeverityWarning), "token_refresh_failure")
}

func mustParseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// Ensure http package is used.
var _ = context.Background
