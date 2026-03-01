package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/stack"
)

type LocalDeployer struct {
	generator  *stack.Generator
	appDataDir string
}

func NewLocal(appDataDir string) *LocalDeployer {
	return &LocalDeployer{
		generator:  stack.New(),
		appDataDir: appDataDir,
	}
}

func (d *LocalDeployer) stackDir() string {
	return filepath.Join(d.appDataDir, "stack")
}

// Deploy writes all config files, pulls images, and starts the central stack.
func (d *LocalDeployer) Deploy(ctx context.Context, cfg models.StackConfig, targets []*models.Target, logFn func(string)) error {
	dir := d.stackDir()

	logFn("Writing configuration files...")
	if err := d.generator.WriteConfigs(dir, cfg, targets); err != nil {
		return fmt.Errorf("generate configs: %w", err)
	}
	logFn(fmt.Sprintf("Config written to %s", dir))

	if err := d.checkDockerAvailable(); err != nil {
		return err
	}

	logFn("Pulling Docker images (this may take a moment)...")
	if err := d.runCompose(ctx, logFn, "pull"); err != nil {
		return fmt.Errorf("docker compose pull: %w", err)
	}

	logFn("Starting monitoring stack...")
	if err := d.runCompose(ctx, logFn, "up", "-d", "--remove-orphans"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	logFn("Stack is up and running!")
	return nil
}

// Stop tears down the central stack.
func (d *LocalDeployer) Stop(ctx context.Context, logFn func(string)) error {
	logFn("Stopping monitoring stack...")
	return d.runCompose(ctx, logFn, "down")
}

// Status returns the current service statuses.
func (d *LocalDeployer) Status(ctx context.Context) ([]models.ServiceStatus, error) {
	composeFile := filepath.Join(d.stackDir(), "docker-compose.yml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return nil, nil
	}

	out, err := exec.CommandContext(ctx, "docker", "compose",
		"-f", composeFile,
		"ps", "--format", "json",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}
	return parseComposePS(out), nil
}

// ReloadPrometheusConfig rewrites prometheus.yml and hot-reloads Prometheus.
func (d *LocalDeployer) ReloadPrometheusConfig(targets []*models.Target, prometheusPort int) error {
	if err := d.generator.WritePrometheusConfig(d.stackDir(), targets); err != nil {
		return fmt.Errorf("write prometheus config: %w", err)
	}
	return ReloadPrometheus(prometheusPort)
}

func (d *LocalDeployer) checkDockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found: install Docker first (https://docs.docker.com/get-docker/)")
	}
	out, err := exec.Command("docker", "info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker daemon not running: %s", out)
	}
	return nil
}

func (d *LocalDeployer) runCompose(ctx context.Context, logFn func(string), args ...string) error {
	composeFile := filepath.Join(d.stackDir(), "docker-compose.yml")
	cmdArgs := append([]string{"compose", "-f", composeFile}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return err
	}

	go streamLines(stdout, logFn)
	go streamLines(stderr, logFn)

	return cmd.Wait()
}

func streamLines(r io.Reader, logFn func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			logFn(line)
		}
	}
}
