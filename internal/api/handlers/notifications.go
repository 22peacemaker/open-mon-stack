package handlers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/22peacemaker/open-mon-stack/internal/deploy"
	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

// ── Channel Handler ───────────────────────────────────────────────────────────

type ChannelHandler struct {
	store      *storage.Store
	appDataDir string
	omsPort    int
}

func NewChannelHandler(store *storage.Store, appDataDir string, omsPort int) *ChannelHandler {
	return &ChannelHandler{store: store, appDataDir: appDataDir, omsPort: omsPort}
}

func (h *ChannelHandler) List(c echo.Context) error {
	channels := h.store.ListChannels()
	if channels == nil {
		channels = []*models.NotificationChannel{}
	}
	return c.JSON(http.StatusOK, channels)
}

func (h *ChannelHandler) Create(c echo.Context) error {
	var req struct {
		Name         string             `json:"name"`
		Type         models.ChannelType `json:"type"`
		URL          string             `json:"url"`
		Topic        string             `json:"topic,omitempty"`
		SMTPHost     string             `json:"smtp_host,omitempty"`
		SMTPPort     int                `json:"smtp_port,omitempty"`
		SMTPUsername string             `json:"smtp_username,omitempty"`
		SMTPPassword string             `json:"smtp_password,omitempty"`
		SMTPFrom     string             `json:"smtp_from,omitempty"`
		SMTPTo       string             `json:"smtp_to,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.Type == "" {
		req.Type = models.ChannelWebhook
	}
	if req.Type == models.ChannelNtfy && req.Topic == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "topic is required for ntfy channels")
	}
	if req.Type == models.ChannelEmail {
		if req.SMTPHost == "" || req.SMTPFrom == "" || req.SMTPTo == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "smtp_host, smtp_from, and smtp_to are required for email channels")
		}
		if req.SMTPPort == 0 {
			req.SMTPPort = 587
		}
	} else if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}

	ch := &models.NotificationChannel{
		ID:           newID(),
		Name:         req.Name,
		Type:         req.Type,
		URL:          req.URL,
		Topic:        req.Topic,
		SMTPHost:     req.SMTPHost,
		SMTPPort:     req.SMTPPort,
		SMTPUsername: req.SMTPUsername,
		SMTPPassword: req.SMTPPassword,
		SMTPFrom:     req.SMTPFrom,
		SMTPTo:       req.SMTPTo,
	}
	if err := h.store.AddChannel(ch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertmanager()
	return c.JSON(http.StatusCreated, ch)
}

func (h *ChannelHandler) Get(c echo.Context) error {
	id := c.Param("id")
	ch, ok := h.store.GetChannel(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	return c.JSON(http.StatusOK, ch)
}

func (h *ChannelHandler) Update(c echo.Context) error {
	id := c.Param("id")
	existing, ok := h.store.GetChannel(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}

	var req struct {
		Name         string             `json:"name"`
		Type         models.ChannelType `json:"type"`
		URL          string             `json:"url"`
		Topic        string             `json:"topic,omitempty"`
		SMTPHost     string             `json:"smtp_host,omitempty"`
		SMTPPort     int                `json:"smtp_port,omitempty"`
		SMTPUsername string             `json:"smtp_username,omitempty"`
		SMTPPassword string             `json:"smtp_password,omitempty"`
		SMTPFrom     string             `json:"smtp_from,omitempty"`
		SMTPTo       string             `json:"smtp_to,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	existing.Topic = req.Topic
	if req.SMTPHost != "" {
		existing.SMTPHost = req.SMTPHost
	}
	if req.SMTPPort != 0 {
		existing.SMTPPort = req.SMTPPort
	}
	if req.SMTPUsername != "" {
		existing.SMTPUsername = req.SMTPUsername
	}
	if req.SMTPPassword != "" {
		existing.SMTPPassword = req.SMTPPassword
	}
	if req.SMTPFrom != "" {
		existing.SMTPFrom = req.SMTPFrom
	}
	if req.SMTPTo != "" {
		existing.SMTPTo = req.SMTPTo
	}

	if err := h.store.UpdateChannel(existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertmanager()
	return c.JSON(http.StatusOK, existing)
}

func (h *ChannelHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if _, ok := h.store.GetChannel(id); !ok {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}
	if err := h.store.DeleteChannel(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertmanager()
	return c.NoContent(http.StatusNoContent)
}

// Test dispatches a test notification to the channel.
func (h *ChannelHandler) Test(c echo.Context) error {
	id := c.Param("id")
	ch, ok := h.store.GetChannel(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}

	event := models.AlertEvent{
		AlertName: "TestAlert",
		Severity:  "info",
		Instance:  "open-mon-stack",
		Status:    "firing",
		Summary:   "This is a test notification from Open Mon Stack",
		FiredAt:   time.Now(),
	}

	if err := dispatchToChannel(ch, event); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("dispatch failed: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "sent"})
}

func (h *ChannelHandler) reloadAlertmanager() {
	cfg := h.store.GetStackConfig()
	d := deploy.NewLocal(h.appDataDir)
	_ = d.ReloadAlertmanagerConfig(h.omsPort, cfg.AlertmanagerPort)
}

// ── Alert Rule Handler ────────────────────────────────────────────────────────

type AlertHandler struct {
	store      *storage.Store
	appDataDir string
}

func NewAlertHandler(store *storage.Store, appDataDir string) *AlertHandler {
	return &AlertHandler{store: store, appDataDir: appDataDir}
}

func (h *AlertHandler) List(c echo.Context) error {
	rules := h.store.ListAlertRules()
	if rules == nil {
		rules = []*models.AlertRule{}
	}
	return c.JSON(http.StatusOK, rules)
}

func (h *AlertHandler) Create(c echo.Context) error {
	var req struct {
		Name        string              `json:"name"`
		Expr        string              `json:"expr"`
		For         string              `json:"for"`
		Severity    models.AlertSeverity `json:"severity"`
		Summary     string              `json:"summary"`
		Description string              `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name == "" || req.Expr == "" || req.For == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, expr, and for are required")
	}
	if req.Severity == "" {
		req.Severity = models.SeverityWarning
	}

	r := &models.AlertRule{
		ID:          newID(),
		Name:        req.Name,
		Expr:        req.Expr,
		For:         req.For,
		Severity:    req.Severity,
		Summary:     req.Summary,
		Description: req.Description,
		Enabled:     true,
		Preset:      false,
	}
	if err := h.store.AddAlertRule(r); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertRules()
	return c.JSON(http.StatusCreated, r)
}

func (h *AlertHandler) Get(c echo.Context) error {
	id := c.Param("id")
	r, ok := h.store.GetAlertRule(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "alert rule not found")
	}
	return c.JSON(http.StatusOK, r)
}

func (h *AlertHandler) Update(c echo.Context) error {
	id := c.Param("id")
	existing, ok := h.store.GetAlertRule(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "alert rule not found")
	}

	var req struct {
		Name        string               `json:"name"`
		Expr        string               `json:"expr"`
		For         string               `json:"for"`
		Severity    models.AlertSeverity `json:"severity"`
		Summary     string               `json:"summary"`
		Description string               `json:"description"`
		Enabled     *bool                `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Presets: only enabled toggle is allowed
	if !existing.Preset {
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Expr != "" {
			existing.Expr = req.Expr
		}
		if req.For != "" {
			existing.For = req.For
		}
		if req.Severity != "" {
			existing.Severity = req.Severity
		}
		if req.Summary != "" {
			existing.Summary = req.Summary
		}
		if req.Description != "" {
			existing.Description = req.Description
		}
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.store.UpdateAlertRule(existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertRules()
	return c.JSON(http.StatusOK, existing)
}

func (h *AlertHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	r, ok := h.store.GetAlertRule(id)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "alert rule not found")
	}
	if r.Preset {
		return echo.NewHTTPError(http.StatusForbidden, "preset rules cannot be deleted")
	}
	if err := h.store.DeleteAlertRule(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reloadAlertRules()
	return c.NoContent(http.StatusNoContent)
}

func (h *AlertHandler) ListEvents(c echo.Context) error {
	events := h.store.ListAlertEvents()
	if events == nil {
		events = []models.AlertEvent{}
	}
	return c.JSON(http.StatusOK, events)
}

func (h *AlertHandler) reloadAlertRules() {
	cfg := h.store.GetStackConfig()
	rules := h.store.ListAlertRules()
	d := deploy.NewLocal(h.appDataDir)
	_ = d.ReloadAlertRules(rules, cfg.PrometheusPort)
}

// ── Webhook Receiver ──────────────────────────────────────────────────────────

// alertmanagerPayload is the JSON body Alertmanager POSTs to receivers.
type alertmanagerPayload struct {
	Receiver string           `json:"receiver"`
	Status   string           `json:"status"`
	Alerts   []alertmanagerAlert `json:"alerts"`
}

type alertmanagerAlert struct {
	Status      string            `json:"status"` // "firing" | "resolved"
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
}

type WebhookReceiver struct {
	store *storage.Store
}

func NewWebhookReceiver(store *storage.Store) *WebhookReceiver {
	return &WebhookReceiver{store: store}
}

// Receive accepts Alertmanager webhook payloads, stores alert events, and dispatches to channels.
func (r *WebhookReceiver) Receive(c echo.Context) error {
	var payload alertmanagerPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	channels := r.store.ListChannels()

	for _, a := range payload.Alerts {
		event := models.AlertEvent{
			AlertName: a.Labels["alertname"],
			Severity:  a.Labels["severity"],
			Instance:  a.Labels["instance"],
			Status:    a.Status,
			Summary:   a.Annotations["summary"],
			FiredAt:   a.StartsAt,
		}
		if event.FiredAt.IsZero() {
			event.FiredAt = time.Now()
		}
		r.store.AppendAlertEvent(event)

		for _, ch := range channels {
			_ = dispatchToChannel(ch, event) // best-effort; log errors silently
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "received"})
}

// ── Dispatchers ───────────────────────────────────────────────────────────────

func dispatchToChannel(ch *models.NotificationChannel, event models.AlertEvent) error {
	switch ch.Type {
	case models.ChannelSlack:
		return dispatchSlack(ch, event)
	case models.ChannelDiscord:
		return dispatchDiscord(ch, event)
	case models.ChannelNtfy:
		return dispatchNtfy(ch, event)
	case models.ChannelN8n:
		return dispatchN8n(ch, event)
	case models.ChannelEmail:
		return dispatchEmail(ch, event)
	default:
		return dispatchWebhook(ch, event)
	}
}

func dispatchSlack(ch *models.NotificationChannel, event models.AlertEvent) error {
	emoji := ":warning:"
	if event.Severity == "critical" {
		emoji = ":red_circle:"
	}
	if event.Status == "resolved" {
		emoji = ":white_check_mark:"
	}

	text := fmt.Sprintf("%s *[%s]* %s\n%s", emoji, strings.ToUpper(event.Status), event.AlertName, event.Summary)
	body, _ := json.Marshal(map[string]string{"text": text})
	return postJSON(ch.URL, body)
}

func dispatchDiscord(ch *models.NotificationChannel, event models.AlertEvent) error {
	color := 16753920 // orange for warning
	if event.Severity == "critical" {
		color = 16711680 // red
	}
	if event.Status == "resolved" {
		color = 65280 // green
	}

	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       fmt.Sprintf("[%s] %s", strings.ToUpper(event.Status), event.AlertName),
				"description": event.Summary,
				"color":       color,
				"footer":      map[string]string{"text": fmt.Sprintf("Severity: %s | Instance: %s", event.Severity, event.Instance)},
				"timestamp":   event.FiredAt.UTC().Format(time.RFC3339),
			},
		},
	}
	body, _ := json.Marshal(payload)
	return postJSON(ch.URL, body)
}

func dispatchNtfy(ch *models.NotificationChannel, event models.AlertEvent) error {
	url := strings.TrimRight(ch.URL, "/") + "/" + ch.Topic
	priority := "default"
	if event.Severity == "critical" {
		priority = "urgent"
	}
	title := fmt.Sprintf("[%s] %s", strings.ToUpper(event.Status), event.AlertName)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(event.Summary))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", fmt.Sprintf("rotating_light,%s", event.Severity))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}

func dispatchN8n(ch *models.NotificationChannel, event models.AlertEvent) error {
	payload := map[string]any{
		"alertName": event.AlertName,
		"severity":  event.Severity,
		"instance":  event.Instance,
		"status":    event.Status,
		"summary":   event.Summary,
		"firedAt":   event.FiredAt.UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	return postJSON(ch.URL, body)
}

func dispatchWebhook(ch *models.NotificationChannel, event models.AlertEvent) error {
	payload := map[string]any{
		"alertName": event.AlertName,
		"severity":  event.Severity,
		"instance":  event.Instance,
		"status":    event.Status,
		"summary":   event.Summary,
		"firedAt":   event.FiredAt.UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	return postJSON(ch.URL, body)
}

func dispatchEmail(ch *models.NotificationChannel, event models.AlertEvent) error {
	port := ch.SMTPPort
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", ch.SMTPHost, port)

	statusEmoji := "🔥"
	if event.Status == "resolved" {
		statusEmoji = "✅"
	}
	subject := fmt.Sprintf("%s [%s] %s", statusEmoji, strings.ToUpper(event.Status), event.AlertName)
	body := fmt.Sprintf(
		"Alert: %s\nStatus: %s\nSeverity: %s\nInstance: %s\nSummary: %s\nFired at: %s",
		event.AlertName,
		strings.ToUpper(event.Status),
		event.Severity,
		event.Instance,
		event.Summary,
		event.FiredAt.UTC().Format(time.RFC3339),
	)
	msg := []byte(
		"From: " + ch.SMTPFrom + "\r\n" +
			"To: " + ch.SMTPTo + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"\r\n" +
			body + "\r\n",
	)

	var auth smtp.Auth
	if ch.SMTPUsername != "" {
		auth = smtp.PlainAuth("", ch.SMTPUsername, ch.SMTPPassword, ch.SMTPHost)
	}

	// Try STARTTLS first (port 587); fall back to plain for local relays (port 25).
	if port == 465 {
		// Implicit TLS (SMTPS)
		tlsCfg := &tls.Config{ServerName: ch.SMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, ch.SMTPHost)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := client.Mail(ch.SMTPFrom); err != nil {
			return err
		}
		for _, to := range strings.Split(ch.SMTPTo, ",") {
			if err := client.Rcpt(strings.TrimSpace(to)); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err = w.Write(msg); err != nil {
			return err
		}
		return w.Close()
	}

	// STARTTLS / plain
	recipients := strings.Split(ch.SMTPTo, ",")
	for i, r := range recipients {
		recipients[i] = strings.TrimSpace(r)
	}
	return smtp.SendMail(addr, auth, ch.SMTPFrom, recipients, msg)
}

func postJSON(url string, body []byte) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
