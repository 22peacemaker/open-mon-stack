package storage_test

import (
	"os"
	"testing"
	"time"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// ── Stack Config ──────────────────────────────────────────────────────────────

func TestDefaultStackConfig(t *testing.T) {
	s := newStore(t)
	cfg := s.GetStackConfig()
	if cfg.GrafanaPort != 3000 {
		t.Errorf("GrafanaPort: got %d, want 3000", cfg.GrafanaPort)
	}
	if cfg.PrometheusPort != 9090 {
		t.Errorf("PrometheusPort: got %d, want 9090", cfg.PrometheusPort)
	}
	if cfg.LokiPort != 3100 {
		t.Errorf("LokiPort: got %d, want 3100", cfg.LokiPort)
	}
}

func TestSaveStackConfig(t *testing.T) {
	s := newStore(t)
	cfg := models.StackConfig{GrafanaPort: 4000, PrometheusPort: 9999, LokiPort: 4100, DataDir: "/custom"}
	if err := s.SaveStackConfig(cfg); err != nil {
		t.Fatalf("SaveStackConfig: %v", err)
	}
	got := s.GetStackConfig()
	if got.GrafanaPort != 4000 {
		t.Errorf("GrafanaPort: got %d, want 4000", got.GrafanaPort)
	}
}

// ── Stack Status ──────────────────────────────────────────────────────────────

func TestDefaultStackStatus(t *testing.T) {
	s := newStore(t)
	st := s.GetStackStatus()
	if st.State != models.StackStateIdle {
		t.Errorf("default state: got %q, want idle", st.State)
	}
}

func TestSetStackStatus(t *testing.T) {
	s := newStore(t)
	before := time.Now()
	s.SetStackStatus(models.StackStatus{State: models.StackStateRunning, Log: []string{"hello"}})
	st := s.GetStackStatus()
	if st.State != models.StackStateRunning {
		t.Errorf("state: got %q, want running", st.State)
	}
	if st.UpdatedAt.Before(before) {
		t.Error("UpdatedAt should be set")
	}
}

func TestAppendLog(t *testing.T) {
	s := newStore(t)
	s.AppendLog("line1")
	s.AppendLog("line2")
	st := s.GetStackStatus()
	if len(st.Log) != 2 {
		t.Errorf("log length: got %d, want 2", len(st.Log))
	}
	if st.Log[0] != "line1" {
		t.Errorf("log[0]: got %q", st.Log[0])
	}
}

// ── Targets ───────────────────────────────────────────────────────────────────

func TestAddAndGetTarget(t *testing.T) {
	s := newStore(t)
	tgt := &models.Target{
		ID:     "t1",
		Name:   "prod-db",
		Host:   "10.0.0.1",
		Agents: []models.AgentType{models.AgentNodeExporter, models.AgentPromtail},
	}
	if err := s.AddTarget(tgt); err != nil {
		t.Fatalf("AddTarget: %v", err)
	}
	got, ok := s.GetTarget("t1")
	if !ok {
		t.Fatal("GetTarget: not found")
	}
	if got.Host != "10.0.0.1" {
		t.Errorf("Host: got %q", got.Host)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestListTargets(t *testing.T) {
	s := newStore(t)
	for i, id := range []string{"a", "b", "c"} {
		_ = s.AddTarget(&models.Target{ID: id, Name: id, Host: "1.2.3." + string(rune('1'+i))})
	}
	list := s.ListTargets()
	if len(list) != 3 {
		t.Errorf("ListTargets: got %d, want 3", len(list))
	}
}

func TestDeleteTarget(t *testing.T) {
	s := newStore(t)
	_ = s.AddTarget(&models.Target{ID: "del", Name: "del", Host: "1.1.1.1"})
	if err := s.DeleteTarget("del"); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
	if _, ok := s.GetTarget("del"); ok {
		t.Error("target should be deleted")
	}
}

func TestUpdateTarget(t *testing.T) {
	s := newStore(t)
	_ = s.AddTarget(&models.Target{ID: "u1", Name: "old", Host: "1.1.1.1"})
	updated := &models.Target{ID: "u1", Name: "new", Host: "2.2.2.2", Agents: []models.AgentType{models.AgentCAdvisor}}
	if err := s.UpdateTarget(updated); err != nil {
		t.Fatalf("UpdateTarget: %v", err)
	}
	got, _ := s.GetTarget("u1")
	if got.Name != "new" {
		t.Errorf("Name: got %q, want 'new'", got.Name)
	}
}

func TestUpdateTargetNotFound(t *testing.T) {
	s := newStore(t)
	err := s.UpdateTarget(&models.Target{ID: "ghost"})
	if err == nil {
		t.Error("expected error for missing target")
	}
}

// ── Persistence ───────────────────────────────────────────────────────────────

func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	s1, _ := storage.New(dir)
	_ = s1.SaveStackConfig(models.StackConfig{GrafanaPort: 5555, PrometheusPort: 9090, LokiPort: 3100, DataDir: "/opt"})
	_ = s1.AddTarget(&models.Target{ID: "p1", Name: "persist", Host: "3.3.3.3", Agents: []models.AgentType{models.AgentNodeExporter}})

	s2, err := storage.New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	cfg := s2.GetStackConfig()
	if cfg.GrafanaPort != 5555 {
		t.Errorf("GrafanaPort not persisted: got %d", cfg.GrafanaPort)
	}
	tgt, ok := s2.GetTarget("p1")
	if !ok {
		t.Fatal("target not persisted")
	}
	if tgt.Host != "3.3.3.3" {
		t.Errorf("Host not persisted: got %q", tgt.Host)
	}
}

func TestNewExistingDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(dir, 0755)
	if _, err := storage.New(dir); err != nil {
		t.Fatalf("New with existing dir: %v", err)
	}
}
