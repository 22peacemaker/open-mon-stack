package stack

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/22peacemaker/open-mon-stack/internal/models"
)

//go:embed all:templates
var templateFS embed.FS

type Generator struct{}

func New() *Generator {
	return &Generator{}
}

// prometheusTemplateData is passed to the prometheus.yml template.
type prometheusTemplateData struct {
	Targets []prometheusTarget
}

type prometheusTarget struct {
	ID             string
	Name           string
	Host           string
	Labels         map[string]string
	HasNodeExporter bool
	HasCAdvisor    bool
}

// WriteConfigs writes all central stack config files to outDir.
func (g *Generator) WriteConfigs(outDir string, cfg models.StackConfig, targets []*models.Target) error {
	dirs := []string{
		filepath.Join(outDir, "prometheus"),
		filepath.Join(outDir, "grafana", "provisioning", "datasources"),
		filepath.Join(outDir, "grafana", "provisioning", "dashboards"),
		filepath.Join(outDir, "loki"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	if err := g.renderTemplate("templates/docker-compose.yml.tmpl", filepath.Join(outDir, "docker-compose.yml"), cfg); err != nil {
		return fmt.Errorf("docker-compose: %w", err)
	}
	if err := g.renderTemplate("templates/prometheus/prometheus.yml.tmpl", filepath.Join(outDir, "prometheus", "prometheus.yml"), buildPrometheusData(targets)); err != nil {
		return fmt.Errorf("prometheus config: %w", err)
	}

	staticFiles := map[string]string{
		"templates/loki/loki-config.yml":                    filepath.Join(outDir, "loki", "loki-config.yml"),
		"templates/grafana/datasources/datasources.yml":     filepath.Join(outDir, "grafana", "provisioning", "datasources", "datasources.yml"),
		"templates/grafana/dashboards/dashboards.yml":       filepath.Join(outDir, "grafana", "provisioning", "dashboards", "dashboards.yml"),
		"templates/grafana/dashboards/system-overview.json": filepath.Join(outDir, "grafana", "provisioning", "dashboards", "system-overview.json"),
	}
	for src, dst := range staticFiles {
		data, err := templateFS.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}

// WritePrometheusConfig (re)writes only prometheus.yml — used for hot reload.
func (g *Generator) WritePrometheusConfig(outDir string, targets []*models.Target) error {
	return g.renderTemplate(
		"templates/prometheus/prometheus.yml.tmpl",
		filepath.Join(outDir, "prometheus", "prometheus.yml"),
		buildPrometheusData(targets),
	)
}

// RenderCompose renders the docker-compose template to a string.
func (g *Generator) RenderCompose(cfg models.StackConfig) (string, error) {
	return g.renderToString("templates/docker-compose.yml.tmpl", cfg)
}

// RenderPrometheus renders the prometheus config template to a string.
func (g *Generator) RenderPrometheus(targets []*models.Target) (string, error) {
	return g.renderToString("templates/prometheus/prometheus.yml.tmpl", buildPrometheusData(targets))
}

// ReadStatic returns the raw content of a static embedded file.
func (g *Generator) ReadStatic(path string) (string, error) {
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildPrometheusData(targets []*models.Target) prometheusTemplateData {
	pts := make([]prometheusTarget, 0, len(targets))
	for _, t := range targets {
		pt := prometheusTarget{
			ID:     t.ID,
			Name:   t.Name,
			Host:   t.Host,
			Labels: t.Labels,
		}
		for _, a := range t.Agents {
			switch a {
			case models.AgentNodeExporter:
				pt.HasNodeExporter = true
			case models.AgentCAdvisor:
				pt.HasCAdvisor = true
			}
		}
		pts = append(pts, pt)
	}
	return prometheusTemplateData{Targets: pts}
}

func (g *Generator) renderTemplate(tmplPath, outPath string, data any) error {
	content, err := g.renderToString(tmplPath, data)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(content), 0644)
}

func (g *Generator) renderToString(tmplPath string, data any) (string, error) {
	raw, err := templateFS.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(filepath.Base(tmplPath)).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
