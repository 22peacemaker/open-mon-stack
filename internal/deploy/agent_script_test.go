package deploy_test

import (
	"strings"
	"testing"

	"github.com/22peacemaker/open-mon-stack/internal/deploy"
	"github.com/22peacemaker/open-mon-stack/internal/models"
)

func target(agents ...models.AgentType) *models.Target {
	return &models.Target{
		ID:     "t1",
		Name:   "prod-server",
		Host:   "10.0.0.1",
		Agents: agents,
	}
}

func TestAgentScriptShebang(t *testing.T) {
	s, err := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if err != nil {
		t.Fatalf("GenerateAgentScript: %v", err)
	}
	if !strings.HasPrefix(s, "#!/usr/bin/env bash") {
		t.Error("missing bash shebang")
	}
}

func TestAgentScriptSetE(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "set -euo pipefail") {
		t.Error("missing set -euo pipefail")
	}
}

func TestAgentScriptContainsTargetInfo(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "prod-server") {
		t.Error("missing target name")
	}
	if !strings.Contains(s, "10.0.0.1") {
		t.Error("missing target host")
	}
}

func TestAgentScriptNodeExporter(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "node-exporter") {
		t.Error("missing node-exporter")
	}
	if !strings.Contains(s, "9100") {
		t.Error("missing node-exporter port")
	}
}

func TestAgentScriptPromtail(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentPromtail), "http://mon:3100")
	if !strings.Contains(s, "promtail") {
		t.Error("missing promtail")
	}
	if !strings.Contains(s, "http://mon:3100") {
		t.Error("missing Loki URL")
	}
}

func TestAgentScriptCAdvisor(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentCAdvisor), "http://mon:3100")
	if !strings.Contains(s, "cadvisor") {
		t.Error("missing cadvisor")
	}
	if !strings.Contains(s, "8080") {
		t.Error("missing cadvisor port")
	}
}

func TestAgentScriptAllAgents(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(
		target(models.AgentNodeExporter, models.AgentPromtail, models.AgentCAdvisor),
		"http://mon:3100",
	)
	for _, want := range []string{"node-exporter", "promtail", "cadvisor"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in combined script", want)
		}
	}
}

func TestAgentScriptNoAgentsError(t *testing.T) {
	_, err := deploy.GenerateAgentScript(target(), "http://mon:3100")
	if err == nil {
		t.Error("expected error for empty agents list")
	}
}

func TestAgentScriptDockerInstall(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "get.docker.com") {
		t.Error("script should include Docker install")
	}
}

func TestAgentScriptNodeExporterOnlyNoPromtailConfig(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	// No promtail config should be written if promtail not selected
	if strings.Contains(s, "write_promtail_config") {
		t.Error("should not write promtail config when promtail not selected")
	}
}

func TestAgentScriptContainsRootOrSudoCheck(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "id -u") {
		t.Error("script should check for root (id -u)")
	}
	if !strings.Contains(s, "sudo") {
		t.Error("script should mention or use sudo for re-exec")
	}
}

func TestAgentScriptContainsFetchCmdOrCurlWget(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "FETCH_CMD") {
		t.Error("script should set or use FETCH_CMD for downloads")
	}
	if !strings.Contains(s, "curl") || !strings.Contains(s, "wget") {
		t.Error("script should support curl and/or wget for downloads")
	}
}

func TestAgentScriptContainsDockerComposeV2Check(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "docker compose version") {
		t.Error("script should check for Docker Compose V2")
	}
	if !strings.Contains(s, "docker-compose-plugin") {
		t.Error("script should install or reference docker-compose-plugin")
	}
}

func TestAgentScriptMainCallsEnsurePrerequisites(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "ensure_prerequisites") {
		t.Error("main should call ensure_prerequisites")
	}
}

func TestAgentScriptPortCheckOnlyForSelectedAgents(t *testing.T) {
	s, _ := deploy.GenerateAgentScript(target(models.AgentNodeExporter), "http://mon:3100")
	if !strings.Contains(s, "9100") {
		t.Error("script with node-exporter should reference port 9100")
	}
	sAll, _ := deploy.GenerateAgentScript(
		target(models.AgentNodeExporter, models.AgentPromtail, models.AgentCAdvisor),
		"http://mon:3100",
	)
	for _, port := range []string{"9100", "9080", "8080"} {
		if !strings.Contains(sAll, port) {
			t.Errorf("script with all agents should reference port %s", port)
		}
	}
}
