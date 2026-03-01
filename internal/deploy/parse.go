package deploy

import (
	"encoding/json"

	"github.com/open-mon-stack/open-mon-stack/internal/models"
)

// composePSEntry matches the JSON output of `docker compose ps --format json`
type composePSEntry struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	Status  string `json:"Status"`
	Health  string `json:"Health"`
	Ports   string `json:"Publishers"`
}

var servicePortMap = map[string]int{
	"prometheus":    9090,
	"grafana":       3000,
	"loki":          3100,
	"node-exporter": 9100,
}

// centralServices are the services that run in the central stack.
var centralServices = []models.ServiceName{
	models.ServicePrometheus,
	models.ServiceGrafana,
	models.ServiceLoki,
	models.ServiceNodeExporter,
}

func parseComposePS(data []byte) []models.ServiceStatus {
	var entries []composePSEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		entries = []composePSEntry{}
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
			status.Running = e.Status != "" && e.Health != "unhealthy"
			status.Health = e.Health
			if status.Health == "" {
				if e.Status != "" {
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
