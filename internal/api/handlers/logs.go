package handlers

import (
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

type LogsHandler struct {
	store *storage.Store
}

func NewLogsHandler(store *storage.Store) *LogsHandler {
	return &LogsHandler{store: store}
}

// Query proxies a LogQL query_range request to the local Loki instance.
// GET /api/logs/query — forwards all query params as-is.
func (h *LogsHandler) Query(c echo.Context) error {
	st := h.store.GetStackStatus()
	if st.State != models.StackStateUp {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "stack is not running")
	}

	cfg := h.store.GetStackConfig()
	lokiURL := fmt.Sprintf("http://localhost:%d/loki/api/v1/query_range?%s",
		cfg.LokiPort, c.QueryString())

	resp, err := http.Get(lokiURL) //nolint:noctx
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "loki unreachable: "+err.Error())
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(resp.Body)
	return c.Blob(resp.StatusCode, "application/json", body)
}
