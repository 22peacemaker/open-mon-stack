package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/open-mon-stack/open-mon-stack/internal/api/handlers"
	"github.com/open-mon-stack/open-mon-stack/internal/models"
	"github.com/open-mon-stack/open-mon-stack/internal/storage"
)

func newEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	return e
}

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return s
}

// ── Target Handler Tests ──────────────────────────────────────────────────────

func TestListTargetsEmpty(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	rec := httptest.NewRecorder()
	if err := h.List(e.NewContext(req, rec)); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	var result []models.Target
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestCreateTarget(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	body := `{"name":"DB Server","host":"10.0.0.1","agents":["node-exporter","promtail"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := h.Create(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", rec.Code)
	}
	var tgt models.Target
	_ = json.Unmarshal(rec.Body.Bytes(), &tgt)
	if tgt.Name != "DB Server" {
		t.Errorf("Name: got %q", tgt.Name)
	}
	if tgt.ID == "" {
		t.Error("ID should be generated")
	}
	if len(tgt.Agents) != 2 {
		t.Errorf("Agents: got %d, want 2", len(tgt.Agents))
	}
}

func TestCreateTargetDefaultAgents(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	body := `{"name":"no-agents","host":"1.1.1.1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	_ = h.Create(e.NewContext(req, rec))

	var tgt models.Target
	_ = json.Unmarshal(rec.Body.Bytes(), &tgt)
	if len(tgt.Agents) == 0 {
		t.Error("should default to recommended agents when none specified")
	}
}

func TestCreateTargetMissingFields(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	cases := []string{
		`{"host":"1.1.1.1"}`,        // missing name
		`{"name":"x"}`,              // missing host
		`{}`,                        // both missing
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		err := h.Create(e.NewContext(req, rec))
		if err == nil {
			t.Errorf("expected error for body %q", body)
			continue
		}
		he := err.(*echo.HTTPError)
		if he.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d for body %q", he.Code, body)
		}
	}
}

func TestGetTargetNotFound(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/targets/nope", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nope")

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected 404")
	}
	if err.(*echo.HTTPError).Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", err.(*echo.HTTPError).Code)
	}
}

func TestDeleteTarget(t *testing.T) {
	e := newEcho()
	store := newStore(t)
	_ = store.AddTarget(&models.Target{ID: "del1", Name: "del", Host: "1.1.1.1"})
	h := handlers.NewTargetHandler(store, t.TempDir())

	req := httptest.NewRequest(http.MethodDelete, "/api/targets/del1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("del1")

	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", rec.Code)
	}
	if _, ok := store.GetTarget("del1"); ok {
		t.Error("target should be deleted")
	}
}

func TestAgentCatalog(t *testing.T) {
	e := newEcho()
	h := handlers.NewTargetHandler(newStore(t), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	if err := h.Agents(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Agents: %v", err)
	}
	var catalog []models.AgentInfo
	_ = json.Unmarshal(rec.Body.Bytes(), &catalog)
	if len(catalog) < 3 {
		t.Errorf("expected at least 3 agents, got %d", len(catalog))
	}
	// Check node-exporter is recommended
	var found bool
	for _, a := range catalog {
		if a.Type == models.AgentNodeExporter && a.Recommended {
			found = true
		}
	}
	if !found {
		t.Error("node-exporter should be recommended")
	}
}

// ── Stack Handler Tests ───────────────────────────────────────────────────────

func TestGetStackConfig(t *testing.T) {
	e := newEcho()
	h := handlers.NewStackHandler(newStore(t), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/stack/config", nil)
	rec := httptest.NewRecorder()
	if err := h.GetConfig(e.NewContext(req, rec)); err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	var cfg models.StackConfig
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.GrafanaPort != 3000 {
		t.Errorf("GrafanaPort: got %d, want 3000", cfg.GrafanaPort)
	}
}

func TestSaveStackConfig(t *testing.T) {
	e := newEcho()
	store := newStore(t)
	h := handlers.NewStackHandler(store, t.TempDir())

	body := `{"grafana_port":4000,"prometheus_port":9999,"loki_port":4100,"data_dir":"/custom"}`
	req := httptest.NewRequest(http.MethodPut, "/api/stack/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := h.SaveConfig(e.NewContext(req, rec)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg := store.GetStackConfig()
	if cfg.GrafanaPort != 4000 {
		t.Errorf("GrafanaPort: got %d, want 4000", cfg.GrafanaPort)
	}
}

func TestGetStackStatus(t *testing.T) {
	e := newEcho()
	h := handlers.NewStackHandler(newStore(t), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/stack/status", nil)
	rec := httptest.NewRecorder()
	if err := h.GetStatus(e.NewContext(req, rec)); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	var st models.StackStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.State != models.StackStateIdle {
		t.Errorf("default state: got %q, want idle", st.State)
	}
}
