package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/open-mon-stack/open-mon-stack/internal/models"
)

type Store struct {
	mu          sync.RWMutex
	dataDir     string
	stackConfig models.StackConfig
	stackStatus models.StackStatus
	targets     map[string]*models.Target
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Store{
		dataDir:     dataDir,
		stackConfig: models.DefaultStackConfig(),
		stackStatus: models.StackStatus{State: models.StackStateIdle, Log: []string{}},
		targets:     make(map[string]*models.Target),
	}
	_ = s.load()
	return s, nil
}

// ── Stack Config ──────────────────────────────────────────────────────────────

func (s *Store) GetStackConfig() models.StackConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stackConfig
}

func (s *Store) SaveStackConfig(cfg models.StackConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stackConfig = cfg
	return s.save()
}

// ── Stack Status (in-memory, not persisted) ───────────────────────────────────

func (s *Store) GetStackStatus() models.StackStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stackStatus
}

func (s *Store) SetStackStatus(st models.StackStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st.UpdatedAt = time.Now()
	s.stackStatus = st
}

func (s *Store) AppendLog(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stackStatus.Log = append(s.stackStatus.Log, line)
	s.stackStatus.UpdatedAt = time.Now()
}

// ── Targets ───────────────────────────────────────────────────────────────────

func (s *Store) AddTarget(t *models.Target) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.CreatedAt = time.Now()
	s.targets[t.ID] = t
	return s.save()
}

func (s *Store) GetTarget(id string) (*models.Target, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.targets[id]
	return t, ok
}

func (s *Store) ListTargets() []*models.Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.Target, 0, len(s.targets))
	for _, t := range s.targets {
		result = append(result, t)
	}
	return result
}

func (s *Store) DeleteTarget(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.targets, id)
	return s.save()
}

func (s *Store) UpdateTarget(t *models.Target) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.targets[t.ID]; !ok {
		return fmt.Errorf("target %s not found", t.ID)
	}
	s.targets[t.ID] = t
	return s.save()
}

// ── Persistence ───────────────────────────────────────────────────────────────

type persistData struct {
	StackConfig models.StackConfig        `json:"stack_config"`
	Targets     map[string]*models.Target `json:"targets"`
}

func (s *Store) save() error {
	data := persistData{StackConfig: s.stackConfig, Targets: s.targets}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "data.json"), b, 0644)
}

func (s *Store) load() error {
	b, err := os.ReadFile(filepath.Join(s.dataDir, "data.json"))
	if err != nil {
		return err
	}
	var data persistData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	if data.StackConfig.GrafanaPort != 0 {
		s.stackConfig = data.StackConfig
	}
	if data.Targets != nil {
		s.targets = data.Targets
	}
	return nil
}
