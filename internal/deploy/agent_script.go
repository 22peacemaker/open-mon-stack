package deploy

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/open-mon-stack/open-mon-stack/internal/models"
)

const agentScriptTemplate = `#!/usr/bin/env bash
# ============================================================
#  Open Mon Stack — Agent Setup Script
#  Target: {{.TargetName}} ({{.TargetHost}})
#  Agents: {{.AgentList}}
# ============================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[oms-agent]${NC} $*"; }
ok()    { echo -e "${GREEN}[ok]${NC} $*"; }
die()   { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

LOKI_URL="{{.LokiURL}}"
DATA_DIR="/opt/oms-agent"

# ── Docker ───────────────────────────────────────────────────
install_docker() {
  if command -v docker &>/dev/null; then
    ok "Docker already installed"
    return
  fi
  info "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  ok "Docker installed"
}

# ── Directories ──────────────────────────────────────────────
create_dirs() {
  info "Creating directories..."
  mkdir -p "$DATA_DIR"
{{if .HasPromtail}}  mkdir -p "$DATA_DIR/promtail"{{end}}
  ok "Directories ready"
}
{{if .HasPromtail}}
# ── Promtail config ──────────────────────────────────────────
write_promtail_config() {
  info "Writing Promtail config (→ $LOKI_URL)..."
  cat > "$DATA_DIR/promtail/promtail-config.yml" <<'PROMTAIL_EOF'
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: {{.LokiURL}}/loki/api/v1/push

scrape_configs:
  - job_name: system
    static_configs:
      - targets: [localhost]
        labels:
          job: varlogs
          host: {{.TargetHost}}
          __path__: /var/log/*log

  - job_name: docker
    static_configs:
      - targets: [localhost]
        labels:
          job: docker
          host: {{.TargetHost}}
          __path__: /var/lib/docker/containers/*/*-json.log
    pipeline_stages:
      - json:
          expressions:
            output: log
            stream: stream
      - output:
          source: output
PROMTAIL_EOF
  ok "Promtail config written"
}
{{end}}
# ── Docker Compose for agents ────────────────────────────────
write_compose() {
  info "Writing agent docker-compose.yml..."
  cat > "$DATA_DIR/docker-compose.yml" <<'COMPOSE_EOF'
version: "3.8"
services:
{{if .HasNodeExporter}}  node-exporter:
    image: prom/node-exporter:latest
    container_name: oms-agent-node-exporter
    restart: unless-stopped
    ports:
      - "9100:9100"
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    command:
      - "--path.procfs=/host/proc"
      - "--path.rootfs=/rootfs"
      - "--path.sysfs=/host/sys"
      - "--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)"
    network_mode: host
{{end}}{{if .HasPromtail}}  promtail:
    image: grafana/promtail:latest
    container_name: oms-agent-promtail
    restart: unless-stopped
    volumes:
      - $DATA_DIR/promtail:/etc/promtail
      - /var/log:/var/log:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    command: -config.file=/etc/promtail/promtail-config.yml
    network_mode: host
{{end}}{{if .HasCAdvisor}}  cadvisor:
    image: gcr.io/cadvisor/cadvisor:latest
    container_name: oms-agent-cadvisor
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker/:/var/lib/docker:ro
    network_mode: host
{{end}}COMPOSE_EOF
  ok "docker-compose.yml written"
}

# ── Start agents ─────────────────────────────────────────────
start_agents() {
  info "Pulling images..."
  docker compose -f "$DATA_DIR/docker-compose.yml" pull

  info "Starting agents..."
  docker compose -f "$DATA_DIR/docker-compose.yml" up -d --remove-orphans

  ok "Agents running!"
}

# ── Summary ──────────────────────────────────────────────────
print_summary() {
  echo ""
  echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
  echo -e "${GREEN}║    OMS Agents are running!           ║${NC}"
  echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
  echo ""
{{if .HasNodeExporter}}  echo -e "  Node Exporter: ${CYAN}http://$(hostname -I | awk '{print $1}'):9100${NC}"
{{end}}{{if .HasPromtail}}  echo -e "  Promtail:      ${CYAN}http://$(hostname -I | awk '{print $1}'):9080${NC}"
{{end}}{{if .HasCAdvisor}}  echo -e "  cAdvisor:      ${CYAN}http://$(hostname -I | awk '{print $1}'):8080${NC}"
{{end}}  echo ""
  echo -e "  Reporting to Loki: ${YELLOW}$LOKI_URL${NC}"
  echo ""
}

# ── Main ─────────────────────────────────────────────────────
main() {
  echo ""
  echo -e "${CYAN}  OMS Agent Setup — {{.TargetName}}${NC}"
  echo -e "  Agents: ${YELLOW}{{.AgentList}}${NC}"
  echo ""

  install_docker
  create_dirs
{{if .HasPromtail}}  write_promtail_config
{{end}}  write_compose
  start_agents
  print_summary
}

main "$@"
`

type agentScriptData struct {
	TargetName      string
	TargetHost      string
	AgentList       string
	LokiURL         string
	HasNodeExporter bool
	HasPromtail     bool
	HasCAdvisor     bool
}

// GenerateAgentScript returns a self-contained bash script that installs the
// configured agents on a target server.
func GenerateAgentScript(target *models.Target, lokiURL string) (string, error) {
	data := agentScriptData{
		TargetName: target.Name,
		TargetHost: target.Host,
		LokiURL:    lokiURL,
	}

	names := make([]string, 0, len(target.Agents))
	for _, a := range target.Agents {
		switch a {
		case models.AgentNodeExporter:
			data.HasNodeExporter = true
			names = append(names, "Node Exporter")
		case models.AgentPromtail:
			data.HasPromtail = true
			names = append(names, "Promtail")
		case models.AgentCAdvisor:
			data.HasCAdvisor = true
			names = append(names, "cAdvisor")
		}
	}
	data.AgentList = strings.Join(names, ", ")

	if !data.HasNodeExporter && !data.HasPromtail && !data.HasCAdvisor {
		return "", fmt.Errorf("no agents selected")
	}

	tmpl, err := template.New("agent").Parse(agentScriptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
