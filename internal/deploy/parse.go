package deploy

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/22peacemaker/open-mon-stack/internal/models"
)

// composePSEntry matches the JSON output of `docker compose ps --format json`
type composePSEntry struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`  // machine-readable: "running", "exited" (Docker Compose v2)
	Status  string `json:"Status"` // human-readable: "Up 2 hours" (older Compose)
	Health  string `json:"Health"`
	// Publishers is a JSON array in Docker Compose v2 — we don't use the data,
	// so we parse it into json.RawMessage to avoid type mismatch errors.
	Publishers json.RawMessage `json:"Publishers"`
}

var servicePortMap = map[string]int{
	"prometheus":    9090,
	"grafana":       3000,
	"loki":          3100,
	"node-exporter": 9100,
	"alertmanager":  9093,
}

// centralServices are the services that run in the central stack.
var centralServices = []models.ServiceName{
	models.ServicePrometheus,
	models.ServiceGrafana,
	models.ServiceLoki,
	models.ServiceNodeExporter,
	models.ServiceAlertmanager,
}

// isRunningStatus returns true when the docker compose status string indicates
// the container is actually running. Docker Compose v2 uses "running"; older
// versions use strings like "Up 5 minutes".
func isRunningStatus(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "running") || strings.HasPrefix(lower, "up")
}

func parseComposePS(data []byte) []models.ServiceStatus {
	var entries []composePSEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Docker Compose v2.20+ outputs NDJSON (one object per line) instead of
		// a JSON array. Fall back to line-by-line parsing.
		entries = []composePSEntry{}
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var e composePSEntry
			if json.Unmarshal(line, &e) == nil {
				entries = append(entries, e)
			}
		}
	}

	entryMap := make(map[string]composePSEntry)
	for _, e := range entries {
		entryMap[e.Service] = e
	}

	result := make([]models.ServiceStatus, 0, len(centralServices))
	for _, svc := range centralServices {
		name := string(svc)
		status := models.ServiceStatus{Name: svc}
		if e, ok := entryMap[name]; ok {
			// State is the machine-readable field in Docker Compose v2 ("running"/"exited").
			// Status is the human-readable fallback in older Compose ("Up 2 hours").
			running := isRunningStatus(e.State) || isRunningStatus(e.Status)
			status.Running = running && e.Health != "unhealthy"
			status.Health = e.Health
			if status.Health == "" {
				if running {
					status.Health = "healthy"
				} else {
					status.Health = "unknown"
				}
			}
		} else {
			status.Health = "unknown"
		}
		if p, ok := servicePortMap[name]; ok {
			status.Port = p
		}
		result = append(result, status)
	}
	return result
}
