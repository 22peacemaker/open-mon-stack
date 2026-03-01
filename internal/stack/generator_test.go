package stack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mon-stack/open-mon-stack/internal/models"
	"github.com/open-mon-stack/open-mon-stack/internal/stack"
)

func defaultCfg() models.StackConfig {
	return models.StackConfig{
		GrafanaPort:    3000,
		PrometheusPort: 9090,
		LokiPort:       3100,
		DataDir:        "/opt/oms",
	}
}

func noTargets() []*models.Target { return []*models.Target{} }

func TestRenderCompose(t *testing.T) {
	g := stack.New()
	out, err := g.RenderCompose(defaultCfg())
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	for _, want := range []string{"oms-prometheus", "oms-grafana", "oms-loki", "oms-node-exporter", "3000:3000", "9090:9090", "3100:3100", "/opt/oms"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderCompose: missing %q", want)
		}
	}
}

func TestRenderComposeNoCentralPromtail(t *testing.T) {
	g := stack.New()
	out, err := g.RenderCompose(defaultCfg())
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	// Promtail is now an agent — should NOT be in the central stack compose
	if strings.Contains(out, "oms-promtail") {
		t.Error("central stack should not contain promtail (it's a remote agent)")
	}
}

func TestRenderComposeCustomPorts(t *testing.T) {
	g := stack.New()
	cfg := defaultCfg()
	cfg.GrafanaPort = 4000
	cfg.PrometheusPort = 9999
	cfg.LokiPort = 4100

	out, err := g.RenderCompose(cfg)
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	if !strings.Contains(out, "4000:3000") {
		t.Error("missing custom grafana port mapping")
	}
	if !strings.Contains(out, "9999:9090") {
		t.Error("missing custom prometheus port mapping")
	}
	if !strings.Contains(out, "4100:3100") {
		t.Error("missing custom loki port mapping")
	}
}

func TestRenderPrometheusNoTargets(t *testing.T) {
	g := stack.New()
	out, err := g.RenderPrometheus(noTargets())
	if err != nil {
		t.Fatalf("RenderPrometheus: %v", err)
	}
	if !strings.Contains(out, "oms-node") {
		t.Error("missing default oms-node job")
	}
	if !strings.Contains(out, "node-exporter:9100") {
		t.Error("missing node-exporter target")
	}
}

func TestRenderPrometheusWithNodeExporterTarget(t *testing.T) {
	g := stack.New()
	targets := []*models.Target{
		{ID: "t1", Name: "prod-db", Host: "10.0.0.1", Agents: []models.AgentType{models.AgentNodeExporter}},
	}
	out, err := g.RenderPrometheus(targets)
	if err != nil {
		t.Fatalf("RenderPrometheus: %v", err)
	}
	if !strings.Contains(out, "10.0.0.1:9100") {
		t.Error("missing target host for node-exporter")
	}
	if !strings.Contains(out, "prod-db") {
		t.Error("missing target name label")
	}
}

func TestRenderPrometheusWithCAdvisor(t *testing.T) {
	g := stack.New()
	targets := []*models.Target{
		{ID: "t2", Name: "docker-host", Host: "10.0.0.2", Agents: []models.AgentType{models.AgentCAdvisor}},
	}
	out, err := g.RenderPrometheus(targets)
	if err != nil {
		t.Fatalf("RenderPrometheus: %v", err)
	}
	if !strings.Contains(out, "10.0.0.2:8080") {
		t.Error("missing cadvisor port 8080")
	}
}

func TestRenderPrometheusPromtailNotScraped(t *testing.T) {
	g := stack.New()
	targets := []*models.Target{
		{ID: "t3", Name: "log-host", Host: "10.0.0.3", Agents: []models.AgentType{models.AgentPromtail}},
	}
	out, err := g.RenderPrometheus(targets)
	if err != nil {
		t.Fatalf("RenderPrometheus: %v", err)
	}
	// Promtail pushes to Loki — Prometheus doesn't need to scrape it
	if strings.Contains(out, "10.0.0.3:9080") {
		t.Error("prometheus should not scrape promtail port 9080")
	}
}

func TestReadStatic(t *testing.T) {
	g := stack.New()
	for _, f := range []string{
		"templates/loki/loki-config.yml",
		"templates/promtail/promtail-config.yml",
		"templates/grafana/datasources/datasources.yml",
		"templates/grafana/dashboards/dashboards.yml",
		"templates/grafana/dashboards/system-overview.json",
	} {
		content, err := g.ReadStatic(f)
		if err != nil {
			t.Errorf("ReadStatic(%q): %v", f, err)
		}
		if len(content) == 0 {
			t.Errorf("ReadStatic(%q): empty", f)
		}
	}
}

func TestReadStaticNotFound(t *testing.T) {
	g := stack.New()
	if _, err := g.ReadStatic("templates/nope.yml"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestWriteConfigs(t *testing.T) {
	g := stack.New()
	dir := t.TempDir()

	if err := g.WriteConfigs(dir, defaultCfg(), noTargets()); err != nil {
		t.Fatalf("WriteConfigs: %v", err)
	}

	for _, rel := range []string{
		"docker-compose.yml",
		filepath.Join("prometheus", "prometheus.yml"),
		filepath.Join("loki", "loki-config.yml"),
		filepath.Join("grafana", "provisioning", "datasources", "datasources.yml"),
		filepath.Join("grafana", "provisioning", "dashboards", "dashboards.yml"),
		filepath.Join("grafana", "provisioning", "dashboards", "system-overview.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); os.IsNotExist(err) {
			t.Errorf("WriteConfigs: missing file %s", rel)
		}
	}
}

func TestWriteConfigsIdempotent(t *testing.T) {
	g := stack.New()
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := g.WriteConfigs(dir, defaultCfg(), noTargets()); err != nil {
			t.Fatalf("WriteConfigs run %d: %v", i+1, err)
		}
	}
}

func TestWritePrometheusConfig(t *testing.T) {
	g := stack.New()
	dir := t.TempDir()
	// Need the prometheus subdir to exist first
	if err := g.WriteConfigs(dir, defaultCfg(), noTargets()); err != nil {
		t.Fatalf("WriteConfigs: %v", err)
	}

	targets := []*models.Target{
		{ID: "x1", Name: "web", Host: "5.5.5.5", Agents: []models.AgentType{models.AgentNodeExporter}},
	}
	if err := g.WritePrometheusConfig(dir, targets); err != nil {
		t.Fatalf("WritePrometheusConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "prometheus", "prometheus.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "5.5.5.5") {
		t.Error("prometheus.yml should contain target host after reload write")
	}
}
