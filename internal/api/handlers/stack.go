package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"

	"github.com/open-mon-stack/open-mon-stack/internal/deploy"
	"github.com/open-mon-stack/open-mon-stack/internal/models"
	"github.com/open-mon-stack/open-mon-stack/internal/storage"
)

type StackHandler struct {
	store      *storage.Store
	appDataDir string
	mu         sync.Mutex
	deploying  bool
	cancelFn   context.CancelFunc
}

func NewStackHandler(store *storage.Store, appDataDir string) *StackHandler {
	return &StackHandler{store: store, appDataDir: appDataDir}
}

func (h *StackHandler) GetConfig(c echo.Context) error {
	cfg := h.store.GetStackConfig()
	return c.JSON(http.StatusOK, cfg)
}

func (h *StackHandler) SaveConfig(c echo.Context) error {
	var cfg models.StackConfig
	if err := c.Bind(&cfg); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if cfg.GrafanaPort == 0 {
		cfg.GrafanaPort = 3000
	}
	if cfg.PrometheusPort == 0 {
		cfg.PrometheusPort = 9090
	}
	if cfg.LokiPort == 0 {
		cfg.LokiPort = 3100
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/opt/open-mon-stack"
	}
	if err := h.store.SaveStackConfig(cfg); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, cfg)
}

func (h *StackHandler) GetStatus(c echo.Context) error {
	st := h.store.GetStackStatus()
	return c.JSON(http.StatusOK, st)
}

func (h *StackHandler) Deploy(c echo.Context) error {
	h.mu.Lock()
	if h.deploying {
		h.mu.Unlock()
		return echo.NewHTTPError(http.StatusConflict, "deploy already in progress")
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.deploying = true
	h.cancelFn = cancel
	h.mu.Unlock()

	cfg := h.store.GetStackConfig()
	targets := h.store.ListTargets()

	h.store.SetStackStatus(models.StackStatus{
		State: models.StackStateRunning,
		Log:   []string{"Starting deployment..."},
	})

	go func() {
		defer func() {
			h.mu.Lock()
			h.deploying = false
			cancel()
			h.mu.Unlock()
		}()

		d := deploy.NewLocal(h.appDataDir)
		logFn := func(line string) { h.store.AppendLog(line) }

		err := d.Deploy(ctx, cfg, targets, logFn)

		st := h.store.GetStackStatus()
		if err != nil {
			st.State = models.StackStateFailed
			st.Log = append(st.Log, fmt.Sprintf("ERROR: %s", err.Error()))
		} else {
			st.State = models.StackStateUp
		}
		h.store.SetStackStatus(st)
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "deploy started"})
}

func (h *StackHandler) Stop(c echo.Context) error {
	logFn := func(line string) { h.store.AppendLog(line) }
	d := deploy.NewLocal(h.appDataDir)
	if err := d.Stop(context.Background(), logFn); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	st := h.store.GetStackStatus()
	st.State = models.StackStateIdle
	h.store.SetStackStatus(st)
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *StackHandler) LiveStatus(c echo.Context) error {
	d := deploy.NewLocal(h.appDataDir)
	services, err := d.Status(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	st := h.store.GetStackStatus()
	st.Services = services
	return c.JSON(http.StatusOK, st)
}

// StreamLogs streams deploy log lines via Server-Sent Events.
func (h *StackHandler) StreamLogs(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	sent := 0
	for {
		st := h.store.GetStackStatus()
		for sent < len(st.Log) {
			fmt.Fprintf(c.Response(), "data: %s\n\n", st.Log[sent])
			c.Response().Flush()
			sent++
		}
		if st.State == models.StackStateUp || st.State == models.StackStateFailed {
			fmt.Fprintf(c.Response(), "event: done\ndata: %s\n\n", st.State)
			c.Response().Flush()
			return nil
		}
		select {
		case <-c.Request().Context().Done():
			return nil
		default:
		}
	}
}
