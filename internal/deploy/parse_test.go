package deploy

import (
	"encoding/json"
	"testing"

	"github.com/22peacemaker/open-mon-stack/internal/models"
)

func TestParseComposePSAllRunning(t *testing.T) {
	entries := []composePSEntry{
		{Service: "prometheus", Status: "running", Health: "healthy"},
		{Service: "grafana", Status: "running", Health: "healthy"},
		{Service: "loki", Status: "running", Health: "healthy"},
		{Service: "node-exporter", Status: "running", Health: ""},
	}
	data, _ := json.Marshal(entries)
	result := parseComposePS(data)

	if len(result) != 4 {
		t.Fatalf("expected 4 services, got %d", len(result))
	}
	for _, svc := range result {
		if !svc.Running {
			t.Errorf("service %s should be running", svc.Name)
		}
	}
}

func TestParseComposePSEmpty(t *testing.T) {
	result := parseComposePS([]byte("[]"))
	if len(result) != 4 {
		t.Fatalf("expected 4 services (unknown state), got %d", len(result))
	}
	for _, svc := range result {
		if svc.Running {
			t.Errorf("service %s should not be running", svc.Name)
		}
		if svc.Health != "unknown" {
			t.Errorf("service %s health: got %q, want 'unknown'", svc.Name, svc.Health)
		}
	}
}

func TestParseComposePSUnhealthy(t *testing.T) {
	entries := []composePSEntry{{Service: "prometheus", Status: "running", Health: "unhealthy"}}
	data, _ := json.Marshal(entries)
	result := parseComposePS(data)

	var prom *models.ServiceStatus
	for i := range result {
		if result[i].Name == models.ServicePrometheus {
			prom = &result[i]
			break
		}
	}
	if prom == nil {
		t.Fatal("prometheus not in results")
	}
	if prom.Running {
		t.Error("unhealthy should not be Running=true")
	}
	if prom.Health != "unhealthy" {
		t.Errorf("health: got %q, want unhealthy", prom.Health)
	}
}

func TestParseComposePSPortMapping(t *testing.T) {
	entries := []composePSEntry{{Service: "grafana", Status: "running", Health: "healthy"}}
	data, _ := json.Marshal(entries)
	result := parseComposePS(data)

	var grafana *models.ServiceStatus
	for i := range result {
		if result[i].Name == models.ServiceGrafana {
			grafana = &result[i]
			break
		}
	}
	if grafana == nil {
		t.Fatal("grafana not in results")
	}
	if grafana.Port != 3000 {
		t.Errorf("port: got %d, want 3000", grafana.Port)
	}
}

func TestParseComposePSNoPromtail(t *testing.T) {
	// Promtail is now a remote agent — not in the central service list
	result := parseComposePS([]byte("[]"))
	for _, svc := range result {
		if svc.Name == "promtail" {
			t.Error("promtail should not appear in central stack services")
		}
	}
}
