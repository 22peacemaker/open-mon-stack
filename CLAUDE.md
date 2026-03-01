# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o open-mon-stack .

# Run
./open-mon-stack --port 8080 --data ~/.open-mon-stack

# Test all
go test ./...

# Test single package
go test ./internal/deploy/...

# Release (push a tag — GitHub Actions runs goreleaser)
git tag v0.1.0 && git push origin v0.1.0
```

## Architecture

Open Mon Stack is a single-binary monitoring stack manager. It deploys a containerized observability stack (Prometheus, Grafana, Loki) locally via Docker Compose and generates bash scripts to install agents (Node Exporter, Promtail, cAdvisor) on remote servers.

**Runtime flow:**
1. The binary serves a REST API (Echo) and embeds the frontend (`web/index.html`)
2. Configuration and targets are persisted to a single `data.json` file in the data directory (`~/.open-mon-stack` by default)
3. On deploy, `internal/stack/generator.go` renders embedded YAML templates (`internal/stack/templates/`) into the data directory, then `internal/deploy/local.go` runs Docker Compose against them
4. When targets are added/updated, `internal/api/handlers/targets.go` regenerates the Prometheus scrape config and hot-reloads it via Prometheus's `/-/reload` HTTP endpoint
5. Agent install scripts are generated on-demand by `internal/deploy/agent_script.go` and served as downloadable bash files

**Key architectural boundaries:**
- `internal/models/` — shared types (no dependencies on other internal packages)
- `internal/storage/` — thread-safe JSON store (sync.RWMutex), no business logic
- `internal/stack/` — template rendering only; reads config, writes files
- `internal/deploy/` — Docker Compose orchestration + bash script generation
- `internal/api/` — HTTP layer; handlers own goroutine lifecycle for long-running deploys
- `web/index.html` — entire frontend as a single Alpine.js + Tailwind CSS SPA

**Deployment state machine** (`StackStatus.State`): `idle` → `running` → `up` | `failed`
Live logs stream to the frontend via Server-Sent Events on `GET /api/stack/logs`.

## API Routes

```
GET/PUT  /api/stack/config        Stack configuration (ports, data dir)
POST     /api/stack/deploy        Start deployment (async, logs via SSE)
POST     /api/stack/stop          Stop stack
GET      /api/stack/status        Current state + buffered logs
GET      /api/stack/logs          SSE log stream

GET/POST /api/targets             List / create monitored servers
GET/PUT  /api/targets/:id         Get / update target
DELETE   /api/targets/:id         Remove target
GET      /api/targets/:id/script  Download agent install bash script
GET      /api/agents              Available agent catalog
```

## Key Implementation Details

- **Embedded assets**: Templates embedded with `//go:embed` in `internal/stack/generator.go`; frontend embedded in `internal/api/server.go`
- **StackStatus is in-memory only** — not persisted to `data.json`; resets on restart
- **No authentication** on API endpoints; Grafana defaults to `admin`/`admin`
- **Version info** injected at build time via goreleaser ldflags (`main.version`, `main.commit`, `main.date`)
- **Goroutine safety**: deploy goroutine is guarded by a mutex in `handlers/stack.go`; cancel via context
- Multi-platform builds: Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64) — packaged as DEB, RPM, and Homebrew tap
