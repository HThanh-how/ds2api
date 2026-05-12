package config

type MonitorConfig struct {
	Metrics  MetricsConfig  `json:"metrics,omitempty"`
	Alerting AlertingConfig `json:"alerting,omitempty"`
}

type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

type AlertingConfig struct {
	Enabled          bool             `json:"enabled"`
	RateLimitSeconds int              `json:"rate_limit_seconds,omitempty"`
	Channels         AlertingChannels `json:"channels,omitempty"`
	Triggers         AlertingTriggers `json:"triggers,omitempty"`
}

type AlertingChannels struct {
	Discord  DiscordChannel  `json:"discord,omitempty"`
	Slack    SlackChannel    `json:"slack,omitempty"`
	Telegram TelegramChannel `json:"telegram,omitempty"`
	Custom   CustomChannel   `json:"custom,omitempty"`
}

type DiscordChannel struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type SlackChannel struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

type TelegramChannel struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id,omitempty"`
}

type CustomChannel struct {
	Enabled bool              `json:"enabled"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type AlertingTriggers struct {
	AccountAllDown               bool    `json:"account_all_down"`
	HighErrorRate                bool    `json:"high_error_rate"`
	HighErrorRateThreshold       float64 `json:"high_error_rate_threshold,omitempty"`
	ConsecutiveUpstreamFailures  bool    `json:"consecutive_upstream_failures"`
	ConsecutiveUpstreamThreshold int     `json:"consecutive_upstream_threshold,omitempty"`
	SessionCreationFailure       bool    `json:"session_creation_failure"`
	PowFailure                   bool    `json:"pow_failure"`
	ContentFilterBlock           bool    `json:"content_filter_block"`
	TokenRefreshFailure          bool    `json:"token_refresh_failure"`
}

func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Alerting: AlertingConfig{
			Enabled:          true,
			RateLimitSeconds: 60,
			Channels: AlertingChannels{
				Discord:  DiscordChannel{Enabled: false},
				Slack:    SlackChannel{Enabled: false},
				Telegram: TelegramChannel{Enabled: false},
				Custom:   CustomChannel{Enabled: false},
			},
			Triggers: AlertingTriggers{
				AccountAllDown:               true,
				HighErrorRate:                true,
				HighErrorRateThreshold:       0.30,
				ConsecutiveUpstreamFailures:  true,
				ConsecutiveUpstreamThreshold: 10,
				SessionCreationFailure:       true,
				PowFailure:                   true,
				ContentFilterBlock:           true,
				TokenRefreshFailure:          true,
			},
		},
	}
}
