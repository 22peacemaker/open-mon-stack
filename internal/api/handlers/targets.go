package handlers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/open-mon-stack/open-mon-stack/internal/deploy"
	"github.com/open-mon-stack/open-mon-stack/internal/models"
	"github.com/open-mon-stack/open-mon-stack/internal/storage"
)

type TargetHandler struct {
	store      *storage.Store
	appDataDir string
}

func NewTargetHandler(store *storage.Store, appDataDir string) *TargetHandler {
	return &TargetHandler{store: store, appDataDir: appDataDir}
}

func (h *TargetHandler) List(c echo.Context) error {
	targets := h.store.ListTargets()
	if targets == nil {
		targets = []*models.Target{}
	}
	return c.JSON(http.StatusOK, targets)
}

func (h *TargetHandler) Agents(c echo.Context) error {
	return c.JSON(http.StatusOK, models.AgentCatalog)
}

func (h *TargetHandler) Create(c echo.Context) error {
	var req struct {
		Name   string            `json:"name"`
		Host   string            `json:"host"`
		Agents []models.AgentType `json:"agents"`
		Labels map[string]string  `json:"labels,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name == "" || req.Host == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and host are required")
	}
	if len(req.Agents) == 0 {
		// Default: node-exporter + promtail
		req.Agents = []models.AgentType{models.AgentNodeExporter, models.AgentPromtail}
	}

	t := &models.Target{
		ID:     newID(),
		Name:   req.Name,
		Host:   req.Host,
		Agents: req.Agents,
		Labels: req.Labels,
	}
	if err := h.store.AddTarget(t); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Hot reload Prometheus with the new target
	h.reloadPrometheus()

	return c.JSON(http.StatusCreated, t)
}

func (h *TargetHandler) Get(c echo.Context) error {
	id := c.Param("id")
	t, ok := h.store.GetTarget(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "target not found")
	}
	return c.JSON(http.StatusOK, t)
}

func (h *TargetHandler) Update(c echo.Context) error {
	id := c.Param("id")
	existing, ok := h.store.GetTarget(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "target not found")
	}

	var req struct {
		Name   string             `json:"name"`
		Agents []models.AgentType `json:"agents"`
		Labels map[string]string  `json:"labels,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if len(req.Agents) > 0 {
		existing.Agents = req.Agents
	}
	if req.Labels != nil {
		existing.Labels = req.Labels
	}

	if err := h.store.UpdateTarget(existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadPrometheus()
	return c.JSON(http.StatusOK, existing)
}

func (h *TargetHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if _, ok := h.store.GetTarget(id); !ok {
		return echo.NewHTTPError(http.StatusNotFound, "target not found")
	}
	if err := h.store.DeleteTarget(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadPrometheus()
	return c.NoContent(http.StatusNoContent)
}

// AgentScript returns the curl one-liner + full bash script for a target.
func (h *TargetHandler) AgentScript(c echo.Context) error {
	id := c.Param("id")
	t, ok := h.store.GetTarget(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "target not found")
	}

	cfg := h.store.GetStackConfig()
	host := c.Request().Host
	lokiURL := cfg.DataDir // fallback
	_ = lokiURL

	// Build Loki URL from request host (strip port, add Loki port)
	lokiBaseURL := "http://" + extractHost(host)
	lokiURL = lokiBaseURL + ":" + itoa(cfg.LokiPort)

	script, err := deploy.GenerateAgentScript(t, lokiURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	scriptURL := "http://" + host + "/api/targets/" + id + "/script/raw"
	oneLiner := "curl -fsSL '" + scriptURL + "' | bash"

	if c.QueryParam("raw") == "1" {
		c.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.Response().Header().Set("Content-Disposition", `attachment; filename="oms-agent-setup.sh"`)
		return c.String(http.StatusOK, script)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"one_liner":  oneLiner,
		"script_url": scriptURL,
		"script":     script,
		"target":     t,
	})
}

// reloadPrometheus rewrites prometheus.yml and hot-reloads — best-effort, no error returned.
func (h *TargetHandler) reloadPrometheus() {
	cfg := h.store.GetStackConfig()
	targets := h.store.ListTargets()
	d := deploy.NewLocal(h.appDataDir)
	_ = d.ReloadPrometheusConfig(targets, cfg.PrometheusPort)
}

func extractHost(hostPort string) string {
	for i := len(hostPort) - 1; i >= 0; i-- {
		if hostPort[i] == ':' {
			return hostPort[:i]
		}
	}
	return hostPort
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
