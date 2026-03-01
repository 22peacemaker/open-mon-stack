package deploy

import (
	"fmt"
	"net/http"
	"time"
)

// ReloadPrometheus triggers a hot config reload via Prometheus's HTTP API.
// Prometheus must be started with --web.enable-lifecycle.
func ReloadPrometheus(prometheusPort int) error {
	url := fmt.Sprintf("http://localhost:%d/-/reload", prometheusPort)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "", nil)
	if err != nil {
		return fmt.Errorf("reload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("prometheus reload returned %d", resp.StatusCode)
	}
	return nil
}
