package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/22peacemaker/open-mon-stack/internal/models"
)

const sessionTTL = 24 * time.Hour

type Store struct {
	mu          sync.RWMutex
	dataDir     string
	stackConfig models.StackConfig
	stackStatus models.StackStatus
	targets     map[string]*models.Target
	channels    map[string]*models.NotificationChannel
	alertRules  map[string]*models.AlertRule
	alertEvents []models.AlertEvent
	users       map[string]*models.User
	usernameIdx map[string]string  // username → user ID; O(1) lookup
	sessions    map[string]*models.Session
	sessionMu   sync.RWMutex
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
		channels:    make(map[string]*models.NotificationChannel),
		alertRules:  make(map[string]*models.AlertRule),
		alertEvents: []models.AlertEvent{},
		users:       make(map[string]*models.User),
		usernameIdx: make(map[string]string),
		sessions:    make(map[string]*models.Session),
	}
	_ = s.load()
	go s.cleanupSessions()
	return s, nil
}

// cleanupSessions removes expired sessions every 30 minutes.
func (s *Store) cleanupSessions() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.sessionMu.Lock()
		for id, sess := range s.sessions {
			if now.After(sess.ExpiresAt) {
				delete(s.sessions, id)
			}
		}
		s.sessionMu.Unlock()
	}
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

// ── Notification Channels ─────────────────────────────────────────────────────

func (s *Store) AddChannel(c *models.NotificationChannel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.CreatedAt = time.Now()
	s.channels[c.ID] = c
	return s.save()
}

func (s *Store) GetChannel(id string) (*models.NotificationChannel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[id]
	return c, ok
}

func (s *Store) ListChannels() []*models.NotificationChannel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.NotificationChannel, 0, len(s.channels))
	for _, c := range s.channels {
		result = append(result, c)
	}
	return result
}

func (s *Store) UpdateChannel(c *models.NotificationChannel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[c.ID]; !ok {
		return fmt.Errorf("channel %s not found", c.ID)
	}
	s.channels[c.ID] = c
	return s.save()
}

func (s *Store) DeleteChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, id)
	return s.save()
}

// ── Alert Rules ───────────────────────────────────────────────────────────────

func (s *Store) AddAlertRule(r *models.AlertRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.CreatedAt = time.Now()
	s.alertRules[r.ID] = r
	return s.save()
}

func (s *Store) GetAlertRule(id string) (*models.AlertRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.alertRules[id]
	return r, ok
}

func (s *Store) ListAlertRules() []*models.AlertRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.AlertRule, 0, len(s.alertRules))
	for _, r := range s.alertRules {
		result = append(result, r)
	}
	return result
}

func (s *Store) UpdateAlertRule(r *models.AlertRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.alertRules[r.ID]; !ok {
		return fmt.Errorf("alert rule %s not found", r.ID)
	}
	s.alertRules[r.ID] = r
	return s.save()
}

func (s *Store) DeleteAlertRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.alertRules, id)
	return s.save()
}

// ── Alert Events ──────────────────────────────────────────────────────────────

const maxAlertEvents = 100

func (s *Store) AppendAlertEvent(e models.AlertEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertEvents = append([]models.AlertEvent{e}, s.alertEvents...) // newest first
	if len(s.alertEvents) > maxAlertEvents {
		s.alertEvents = s.alertEvents[:maxAlertEvents]
	}
	_ = s.save()
}

func (s *Store) ListAlertEvents() []models.AlertEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.AlertEvent, len(s.alertEvents))
	copy(result, s.alertEvents)
	return result
}

// ── Users ─────────────────────────────────────────────────────────────────────

func (s *Store) UserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

func (s *Store) AddUser(u *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	s.usernameIdx[u.Username] = u.ID
	return s.save()
}

func (s *Store) GetUser(id string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

func (s *Store) GetUserByUsername(username string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usernameIdx[username]
	if !ok {
		return nil, false
	}
	u, ok := s.users[id]
	return u, ok
}

func (s *Store) ListUsers() []*models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.User, 0, len(s.users))
	for _, u := range s.users {
		result = append(result, u)
	}
	return result
}

func (s *Store) UpdateUser(u *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.users[u.ID]
	if !ok {
		return fmt.Errorf("user %s not found", u.ID)
	}
	// Update username index if username changed
	if old.Username != u.Username {
		delete(s.usernameIdx, old.Username)
		s.usernameIdx[u.Username] = u.ID
	}
	s.users[u.ID] = u
	return s.save()
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[id]; ok {
		delete(s.usernameIdx, u.Username)
	}
	delete(s.users, id)
	return s.save()
}

// AdminCount returns the number of users with the admin role.
func (s *Store) AdminCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, u := range s.users {
		if u.Role == models.RoleAdmin {
			count++
		}
	}
	return count
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (s *Store) CreateSession(userID, username string, role models.Role) *models.Session {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	sess := &models.Session{
		ID:        hex.EncodeToString(b),
		UserID:    userID,
		Username:  username,
		Role:      role,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	s.sessionMu.Lock()
	s.sessions[sess.ID] = sess
	s.sessionMu.Unlock()
	// Persist the new session
	s.mu.Lock()
	_ = s.save()
	s.mu.Unlock()
	return sess
}

func (s *Store) GetSession(id string) (*models.Session, bool) {
	s.sessionMu.RLock()
	sess, ok := s.sessions[id]
	s.sessionMu.RUnlock()
	if !ok || time.Now().After(sess.ExpiresAt) {
		if ok {
			s.sessionMu.Lock()
			delete(s.sessions, id)
			s.sessionMu.Unlock()
		}
		return nil, false
	}
	return sess, true
}

func (s *Store) DeleteSession(id string) {
	s.sessionMu.Lock()
	delete(s.sessions, id)
	s.sessionMu.Unlock()
	s.mu.Lock()
	_ = s.save()
	s.mu.Unlock()
}

// ── Persistence ───────────────────────────────────────────────────────────────

type persistData struct {
	StackConfig models.StackConfig                     `json:"stack_config"`
	Targets     map[string]*models.Target              `json:"targets"`
	Channels    map[string]*models.NotificationChannel `json:"channels"`
	AlertRules  map[string]*models.AlertRule           `json:"alert_rules"`
	AlertEvents []models.AlertEvent                    `json:"alert_events,omitempty"`
	Users       map[string]*models.User                `json:"users"`
	Sessions    map[string]*models.Session             `json:"sessions,omitempty"`
}

// save persists all state to data.json atomically via a temp file + rename.
// Callers must hold s.mu (write lock) before calling this.
func (s *Store) save() error {
	// Snapshot non-expired sessions under sessionMu
	s.sessionMu.RLock()
	now := time.Now()
	sessions := make(map[string]*models.Session, len(s.sessions))
	for id, sess := range s.sessions {
		if !now.After(sess.ExpiresAt) {
			sessions[id] = sess
		}
	}
	s.sessionMu.RUnlock()

	data := persistData{
		StackConfig: s.stackConfig,
		Targets:     s.targets,
		Channels:    s.channels,
		AlertRules:  s.alertRules,
		AlertEvents: s.alertEvents,
		Users:       s.users,
		Sessions:    sessions,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dataDir, "data.json.tmp")
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dataDir, "data.json"))
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
	// Migrate: set alertmanager default port if missing from old data.json
	if s.stackConfig.AlertmanagerPort == 0 {
		s.stackConfig.AlertmanagerPort = 9093
	}
	// Migrate: generate Grafana admin password if missing
	if s.stackConfig.GrafanaAdminPassword == "" {
		b := make([]byte, 12)
		_, _ = rand.Read(b)
		s.stackConfig.GrafanaAdminPassword = hex.EncodeToString(b)
		_ = s.save()
	}
	if data.Targets != nil {
		s.targets = data.Targets
	}
	if data.Channels != nil {
		s.channels = data.Channels
	}
	if data.AlertRules != nil {
		s.alertRules = data.AlertRules
	}
	if data.AlertEvents != nil {
		s.alertEvents = data.AlertEvents
	}
	if data.Users != nil {
		s.users = data.Users
		// Rebuild username index
		for _, u := range s.users {
			s.usernameIdx[u.Username] = u.ID
		}
	}
	// Restore non-expired sessions
	if data.Sessions != nil {
		now := time.Now()
		for id, sess := range data.Sessions {
			if !now.After(sess.ExpiresAt) {
				s.sessions[id] = sess
			}
		}
	}
	// Seed presets on first run (no rules in persisted data)
	if len(s.alertRules) == 0 {
		s.seedPresets()
	}
	return nil
}

// seedPresets inserts default alert rules. Called without lock (only from load).
func (s *Store) seedPresets() {
	now := time.Now()
	for i, preset := range models.DefaultAlertPresets() {
		id := fmt.Sprintf("preset-%02d", i+1)
		preset.ID = id
		preset.CreatedAt = now
		s.alertRules[id] = preset
	}
}
