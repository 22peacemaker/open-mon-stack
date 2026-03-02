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

Public (no auth required):
```
POST     /api/auth/login           Authenticate; sets oms_session cookie
POST     /api/auth/logout          Clears session
GET      /api/setup/status         {needs_setup: bool} — true until first admin is created
POST     /api/setup                Create the first admin account
POST     /api/webhooks/receiver    Alertmanager webhook (called by local container)
```

Auth required (viewer+):
```
GET      /api/auth/me              Current user info
GET/PUT  /api/stack/config         Stack configuration (ports, data dir)  [PUT: admin]
POST     /api/stack/deploy         Start deployment (async, logs via SSE) [admin]
POST     /api/stack/stop           Stop stack                              [admin]
GET      /api/stack/status         Current state + buffered logs
GET      /api/stack/health         Lightweight health check: {healthy, services}
GET      /api/stack/logs           SSE log stream

GET/POST /api/targets              List / create monitored servers          [POST: admin]
GET/PUT  /api/targets/:id          Get / update target                      [PUT: admin]
DELETE   /api/targets/:id          Remove target                            [admin]
GET      /api/targets/:id/script   Download agent install bash script
GET      /api/agents               Available agent catalog

GET/POST /api/channels             Notification channels                    [POST: admin]
GET/PUT  /api/channels/:id         Get / update channel                     [PUT: admin]
DELETE   /api/channels/:id                                                  [admin]
POST     /api/channels/:id/test    Send a test notification                 [admin]

GET/POST /api/alerts/rules         Alert rules (PromQL-based)               [POST: admin]
GET/PUT  /api/alerts/rules/:id                                              [PUT: admin]
DELETE   /api/alerts/rules/:id     Cannot delete preset rules              [admin]
GET      /api/alerts/events        In-memory alert event log (last 100)

GET      /api/logs/query            Loki log query proxy (query, limit, start, end); 503 if stack not up [viewer+]

GET/POST /api/users                User management                          [admin]
GET/PUT  /api/users/:id                                                     [admin]
DELETE   /api/users/:id                                                     [admin]

GET/POST /api/tokens               API tokens (programmatic access)        [admin]
DELETE  /api/tokens/:id            Revoke token                             [admin]
```

Auth for protected routes: session cookie `oms_session` **or** header `X-OMS-Token: <token>`. Token format: `oms_<id>_<secret>`; raw value returned only once on `POST /api/tokens`.

## Key Implementation Details

- **Embedded assets**: Templates embedded with `//go:embed` in `internal/stack/generator.go`; frontend embedded in `internal/api/server.go`
- **StackStatus is in-memory only** — not persisted to `data.json`; resets on restart
- **Authentication**: Session-cookie auth (`oms_session`, HttpOnly, 24h TTL) or API token via header `X-OMS-Token`. Tokens are stored in `data.json` (id, name, role, optional expiry, token hash); raw token value is returned only once on creation. Tokens are managed on the Users admin page. Sessions are in-memory only — lost on restart. Roles: `admin` (full write access) and `viewer` (read-only). First-run: visit `/api/setup/status`; if `needs_setup` is true, `POST /api/setup` to create the first admin. Middleware: `internal/api/middleware/auth.go`; aliased as `authmw` in `server.go` to avoid conflict with Echo's middleware package.
- **Alertmanager**: Added to docker-compose stack (port `alertmanager_port` in `StackConfig`). OMS acts as the Alertmanager webhook receiver. Notification channels (Slack, Discord, ntfy, n8n, generic webhook) stored in `data.json`. Alert rules rendered to `internal/stack/templates/prometheus/alerts.yml.tmpl`; hot-reloaded via Prometheus `/-/reload`. Alert events are in-memory only (last 100).
- **Preset alert rules**: 4 built-in rules (HostDown, HighCPU, HighDisk, HighMemory) seeded on first store init. Presets can be toggled (enabled/disabled) but not deleted or fully edited.
- **Loki log viewer**: The **Logs** page in the UI lets viewers query Loki without opening Grafana. `GET /api/logs/query` proxies to the local Loki `query_range` API. The UI offers stream selector (e.g. `{job="varlogs"}`, `{job="docker"}`, or per-target with `host`), plain-text search (`|= "term"`), time range (15m/1h/6h/24h), and optional auto-refresh. Available only when the stack is up.
- **Version info** injected at build time via goreleaser ldflags (`main.version`, `main.commit`, `main.date`)
- **Goroutine safety**: deploy goroutine is guarded by a mutex in `handlers/stack.go`; cancel via context
- Multi-platform builds: Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64) — packaged as DEB, RPM, and Homebrew tap
