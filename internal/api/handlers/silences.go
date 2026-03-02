package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

type SilencesHandler struct {
	store *storage.Store
}

func NewSilencesHandler(store *storage.Store) *SilencesHandler {
	return &SilencesHandler{store: store}
}

func (h *SilencesHandler) ensureStackUp(c echo.Context) error {
	st := h.store.GetStackStatus()
	if st.State != models.StackStateUp {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "stack is not running")
	}
	return nil
}

func (h *SilencesHandler) baseURL() string {
	cfg := h.store.GetStackConfig()
	return fmt.Sprintf("http://localhost:%d/api/v2", cfg.AlertmanagerPort)
}

// List proxies GET to Alertmanager /api/v2/silences (viewer+).
func (h *SilencesHandler) List(c echo.Context) error {
	if err := h.ensureStackUp(c); err != nil {
		return err
	}
	url := h.baseURL() + "/silences"
	if q := c.QueryString(); q != "" {
		url = url + "?" + q
	}
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "alertmanager unreachable: "+err.Error())
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	return c.Blob(resp.StatusCode, "application/json", body)
}

// Create proxies POST to Alertmanager /api/v2/silences (admin).
func (h *SilencesHandler) Create(c echo.Context) error {
	if err := h.ensureStackUp(c); err != nil {
		return err
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	url := h.baseURL() + "/silences"
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "alertmanager unreachable: "+err.Error())
	}
	defer resp.Body.Close() //nolint:errcheck
	respBody, _ := io.ReadAll(resp.Body)
	return c.Blob(resp.StatusCode, "application/json", respBody)
}

// Delete proxies DELETE to Alertmanager /api/v2/silence/:id (admin).
func (h *SilencesHandler) Delete(c echo.Context) error {
	if err := h.ensureStackUp(c); err != nil {
		return err
	}
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "silence id required")
	}
	url := h.baseURL() + "/silence/" + id
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "alertmanager unreachable: "+err.Error())
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode == http.StatusOK {
		return c.NoContent(http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	return c.Blob(resp.StatusCode, "application/json", body)
}
