package models

import "time"

// ── Notification Channels ────────────────────────────────────────────────────

type ChannelType string

const (
	ChannelSlack   ChannelType = "slack"
	ChannelDiscord ChannelType = "discord"
	ChannelNtfy    ChannelType = "ntfy"
	ChannelN8n     ChannelType = "n8n"
	ChannelWebhook ChannelType = "webhook"
	ChannelEmail   ChannelType = "email"
)

// NotificationChannel is a destination for alert notifications.
type NotificationChannel struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      ChannelType `json:"type"`
	URL       string      `json:"url,omitempty"`   // webhook URL; for ntfy: server base URL (e.g. https://ntfy.sh)
	Topic     string      `json:"topic,omitempty"` // ntfy topic name
	CreatedAt time.Time   `json:"created_at"`

	// SMTP fields (only used when Type == "email")
	SMTPHost     string `json:"smtp_host,omitempty"`
	SMTPPort     int    `json:"smtp_port,omitempty"` // defaults to 587
	SMTPUsername string `json:"smtp_username,omitempty"`
	SMTPPassword string `json:"smtp_password,omitempty"`
	SMTPFrom     string `json:"smtp_from,omitempty"`
	SMTPTo       string `json:"smtp_to,omitempty"` // comma-separated list of recipients
}

// ── Alert Rules ───────────────────────────────────────────────────────────────

type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
)

// AlertRule defines a Prometheus alerting rule.
type AlertRule struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Expr        string        `json:"expr"`        // PromQL expression
	For         string        `json:"for"`         // duration string, e.g. "5m"
	Severity    AlertSeverity `json:"severity"`
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Preset      bool          `json:"preset"` // true = built-in, cannot be deleted
	CreatedAt   time.Time     `json:"created_at"`
}

// AlertEvent is an in-memory record of a received alert (not persisted).
type AlertEvent struct {
	AlertName string    `json:"alert_name"`
	Severity  string    `json:"severity"`
	Instance  string    `json:"instance"`
	Status    string    `json:"status"` // "firing" | "resolved"
	Summary   string    `json:"summary"`
	FiredAt   time.Time `json:"fired_at"`
}

// DefaultAlertPresets returns the built-in alert rule presets.
// Called once on first store initialization (when no rules exist yet).
func DefaultAlertPresets() []*AlertRule {
	return []*AlertRule{
		{
			Name:        "HostDown",
			Expr:        `up == 0`,
			For:         "2m",
			Severity:    SeverityCritical,
			Summary:     `Host {{ $labels.instance }} is down`,
			Description: `Host {{ $labels.instance }} (job={{ $labels.job }}) has been unreachable for more than 2 minutes.`,
			Enabled:     true,
			Preset:      true,
		},
		{
			Name:        "HighCPU",
			Expr:        `(1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m]))) * 100 > 90`,
			For:         "5m",
			Severity:    SeverityWarning,
			Summary:     `High CPU usage on {{ $labels.instance }}`,
			Description: `CPU usage on {{ $labels.instance }} has been above 90% for more than 5 minutes (current: {{ $value | printf "%.1f" }}%).`,
			Enabled:     true,
			Preset:      true,
		},
		{
			Name:        "HighDisk",
			Expr:        `(node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 < 15`,
			For:         "2m",
			Severity:    SeverityWarning,
			Summary:     `Low disk space on {{ $labels.instance }}`,
			Description: `Root filesystem on {{ $labels.instance }} has less than 15% free space ({{ $value | printf "%.1f" }}% remaining).`,
			Enabled:     true,
			Preset:      true,
		},
		{
			Name:        "HighMemory",
			Expr:        `(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100 > 90`,
			For:         "5m",
			Severity:    SeverityWarning,
			Summary:     `High memory usage on {{ $labels.instance }}`,
			Description: `Memory usage on {{ $labels.instance }} has been above 90% for more than 5 minutes (current: {{ $value | printf "%.1f" }}%).`,
			Enabled:     true,
			Preset:      true,
		},
	}
}
