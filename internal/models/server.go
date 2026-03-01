package models

import "time"

// ── Central Stack ────────────────────────────────────────────────────────────

type StackState string

const (
	StackStateIdle    StackState = "idle"
	StackStateRunning StackState = "running"
	StackStateUp      StackState = "up"
	StackStateFailed  StackState = "failed"
)

// StackConfig holds configuration for the central Prometheus/Grafana/Loki deployment.
type StackConfig struct {
	GrafanaPort    int    `json:"grafana_port"`
	PrometheusPort int    `json:"prometheus_port"`
	LokiPort       int    `json:"loki_port"`
	DataDir        string `json:"data_dir"`
}

func DefaultStackConfig() StackConfig {
	return StackConfig{
		GrafanaPort:    3000,
		PrometheusPort: 9090,
		LokiPort:       3100,
		DataDir:        "/opt/open-mon-stack",
	}
}

// StackStatus is the live status of the local monitoring stack.
type StackStatus struct {
	State     StackState      `json:"state"`
	Log       []string        `json:"log"`
	Services  []ServiceStatus `json:"services,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ── Services ─────────────────────────────────────────────────────────────────

type ServiceName string

const (
	ServicePrometheus   ServiceName = "prometheus"
	ServiceGrafana      ServiceName = "grafana"
	ServiceLoki         ServiceName = "loki"
	ServiceNodeExporter ServiceName = "node-exporter"
)

type ServiceStatus struct {
	Name    ServiceName `json:"name"`
	Running bool        `json:"running"`
	Health  string      `json:"health"` // healthy, unhealthy, starting, unknown
	Port    int         `json:"port,omitempty"`
}

// ── Targets (monitored remote servers) ───────────────────────────────────────

type AgentType string

const (
	AgentNodeExporter AgentType = "node-exporter"
	AgentPromtail     AgentType = "promtail"
	AgentCAdvisor     AgentType = "cadvisor"
)

// AgentInfo describes an agent for display in the UI.
type AgentInfo struct {
	Type        AgentType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Port        int       `json:"port"`
	Recommended bool      `json:"recommended"`
}

var AgentCatalog = []AgentInfo{
	{
		Type:        AgentNodeExporter,
		Name:        "Node Exporter",
		Description: "System-Metriken: CPU, RAM, Disk, Netzwerk",
		Port:        9100,
		Recommended: true,
	},
	{
		Type:        AgentPromtail,
		Name:        "Promtail",
		Description: "Log-Collector — sendet /var/log und Docker-Logs an Loki",
		Port:        9080,
		Recommended: true,
	},
	{
		Type:        AgentCAdvisor,
		Name:        "cAdvisor",
		Description: "Docker-Container-Metriken (CPU, RAM pro Container)",
		Port:        8080,
		Recommended: false,
	},
}

// Target is a remote server being monitored.
type Target struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Host      string            `json:"host"`
	Agents    []AgentType       `json:"agents"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
