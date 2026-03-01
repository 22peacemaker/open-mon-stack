# open-mon-stack

[![CI](https://github.com/open-mon-stack/open-mon-stack/actions/workflows/ci.yml/badge.svg)](https://github.com/open-mon-stack/open-mon-stack/actions/workflows/ci.yml)
[![Release](https://github.com/open-mon-stack/open-mon-stack/releases/latest/badge.svg)](https://github.com/open-mon-stack/open-mon-stack/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A single-binary monitoring stack manager. Deploy Prometheus, Grafana, and Loki on any Linux server in seconds — then monitor remote servers by running one command on each.

## How it works

**Central stack** (runs on your monitoring server):
- Prometheus — metrics storage and querying
- Grafana — dashboards and visualization
- Loki — log aggregation
- Node Exporter — host metrics for the monitoring server itself

**Remote agents** (run on each server you want to monitor):
- Node Exporter — CPU, RAM, disk, network metrics
- Promtail — log shipper
- cAdvisor — Docker container metrics (auto-detected)

open-mon-stack manages the central stack via Docker Compose and generates ready-to-run bash scripts to install agents on remote servers.

## Requirements

- Docker and Docker Compose on the monitoring server
- SSH access to remote servers (for agent installation)

## Installation

**Homebrew (macOS/Linux):**
```bash
brew install open-mon-stack/tap/open-mon-stack
```

**DEB/RPM packages:** Download from the [releases page](https://github.com/open-mon-stack/open-mon-stack/releases).

**Binary:**
```bash
curl -L https://github.com/open-mon-stack/open-mon-stack/releases/latest/download/open-mon-stack_linux_amd64.tar.gz | tar xz
sudo mv open-mon-stack /usr/local/bin/
```

## Usage

```bash
open-mon-stack [flags]

Flags:
  -port int     HTTP port to listen on (default 8080)
  -data string  Directory for data and stack configs (default ~/.open-mon-stack)
  -version      Print version and exit
```

Open `http://localhost:8080` in your browser to access the web interface.

**Quick start:**
1. Start the app: `open-mon-stack`
2. Open the web UI and configure the stack (ports, data directory)
3. Click **Deploy** to start Prometheus, Grafana, and Loki
4. Add remote servers and download their agent install scripts
5. Run the script on each server: `curl -fsSL <script-url> | bash`

## Data

All configuration is stored in `~/.open-mon-stack/data.json`. The stack config files (docker-compose.yml, prometheus.yml, etc.) are written to the data directory on deploy.

## License

MIT — see [LICENSE](LICENSE).
