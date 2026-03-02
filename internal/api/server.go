package api

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/22peacemaker/open-mon-stack/internal/api/handlers"
	authmw "github.com/22peacemaker/open-mon-stack/internal/api/middleware"
	"github.com/22peacemaker/open-mon-stack/internal/deploy"
	"github.com/22peacemaker/open-mon-stack/internal/models"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

type Server struct {
	echo       *echo.Echo
	store      *storage.Store
	appDataDir string
	omsPort    int
}

func New(store *storage.Store, appDataDir string, webFS embed.FS, omsPort int) *Server {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	s := &Server{echo: e, store: store, appDataDir: appDataDir, omsPort: omsPort}
	s.setupRoutes(webFS)
	go s.recoverStackStatus()
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// recoverStackStatus checks whether the Docker Compose stack is already running
// (e.g. after a server restart) and updates the in-memory status accordingly.
func (s *Server) recoverStackStatus() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	d := deploy.NewLocal(s.appDataDir)
	services, err := d.Status(ctx)
	if err != nil || len(services) == 0 {
		return // leave status as idle
	}

	anyRunning := false
	for _, svc := range services {
		if svc.Running {
			anyRunning = true
			break
		}
	}
	if !anyRunning {
		return
	}

	s.store.SetStackStatus(models.StackStatus{
		State:    models.StackStateUp,
		Log:      []string{"Stack was already running; status recovered on startup."},
		Services: services,
	})
}

// loginRateLimiter restricts login attempts to 10 per minute per IP (burst 5).
var loginRateLimiter = middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
	Store: middleware.NewRateLimiterMemoryStoreWithConfig(
		middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(10.0 / 60.0),
			Burst:     5,
			ExpiresIn: 5 * time.Minute,
		},
	),
	IdentifierExtractor: func(c echo.Context) (string, error) {
		return c.RealIP(), nil
	},
	DenyHandler: func(c echo.Context, identifier string, err error) error {
		return echo.NewHTTPError(http.StatusTooManyRequests, "too many login attempts, please try again later")
	},
})

func (s *Server) setupRoutes(webFS embed.FS) {
	e := s.echo

	// Static frontend
	stripped, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.FileServer(http.FS(stripped)))))

	// SPA fallback
	e.GET("/", s.serveIndex(webFS))
	e.GET("/targets/*", s.serveIndex(webFS))
	e.GET("/alerts/*", s.serveIndex(webFS))
	e.GET("/logs/*", s.serveIndex(webFS))

	// ── Public endpoints ──────────────────────────────────────────────────────

	// Auth
	authH := handlers.NewAuthHandler(s.store)
	e.POST("/api/auth/login", authH.Login, loginRateLimiter)
	e.POST("/api/auth/logout", authH.Logout)
	e.GET("/api/setup/status", authH.SetupStatus)
	e.POST("/api/setup", authH.Setup)

	// Alertmanager webhook receiver stays public (called by the local container)
	wr := handlers.NewWebhookReceiver(s.store)
	e.POST("/api/webhooks/receiver", wr.Receive)

	// ── Auth-required reads ───────────────────────────────────────────────────

	requireAuth := authmw.RequireAuth(s.store)
	requireAdmin := authmw.RequireAdmin(s.store)

	api := e.Group("/api", requireAuth)

	api.GET("/auth/me", authH.Me)
	api.PUT("/auth/me/password", authH.ChangePassword)

	// Central stack (reads)
	sh := handlers.NewStackHandler(s.store, s.appDataDir, s.omsPort)
	api.GET("/stack/config", sh.GetConfig)
	api.GET("/stack/status", sh.LiveStatus)
	api.GET("/stack/health", sh.Health)
	api.GET("/stack/logs", sh.StreamLogs)

	// Targets (reads)
	th := handlers.NewTargetHandler(s.store, s.appDataDir)
	api.GET("/targets", th.List)
	api.GET("/targets/:id", th.Get)
	api.GET("/targets/:id/script", th.AgentScript)
	api.GET("/targets/:id/script/raw", func(c echo.Context) error {
		c.QueryParams().Set("raw", "1")
		return th.AgentScript(c)
	})
	api.GET("/agents", th.Agents)

	// Notification channels (reads)
	ch := handlers.NewChannelHandler(s.store, s.appDataDir, s.omsPort)
	api.GET("/channels", ch.List)
	api.GET("/channels/:id", ch.Get)

	// Alert rules & events (reads)
	alh := handlers.NewAlertHandler(s.store, s.appDataDir)
	api.GET("/alerts/rules", alh.List)
	api.GET("/alerts/rules/:id", alh.Get)
	api.GET("/alerts/events", alh.ListEvents)

	// Loki log viewer (reads; requires stack up, handler returns 503 otherwise)
	lh := handlers.NewLogsHandler(s.store)
	api.GET("/logs/query", lh.Query)

	// ── Admin-only writes ─────────────────────────────────────────────────────

	adm := e.Group("/api", requireAdmin)

	// Central stack (writes)
	adm.PUT("/stack/config", sh.SaveConfig)
	adm.POST("/stack/deploy", sh.Deploy)
	adm.POST("/stack/stop", sh.Stop)

	// Targets (writes)
	adm.POST("/targets", th.Create)
	adm.PUT("/targets/:id", th.Update)
	adm.DELETE("/targets/:id", th.Delete)

	// Notification channels (writes)
	adm.POST("/channels", ch.Create)
	adm.PUT("/channels/:id", ch.Update)
	adm.DELETE("/channels/:id", ch.Delete)
	adm.POST("/channels/:id/test", ch.Test)

	// Alert rules (writes)
	adm.POST("/alerts/rules", alh.Create)
	adm.PUT("/alerts/rules/:id", alh.Update)
	adm.DELETE("/alerts/rules/:id", alh.Delete)

	// User management (admin only)
	uh := handlers.NewUserHandler(s.store)
	adm.GET("/users", uh.List)
	adm.POST("/users", uh.Create)
	adm.GET("/users/:id", uh.Get)
	adm.PUT("/users/:id", uh.Update)
	adm.DELETE("/users/:id", uh.Delete)

	// API tokens (admin only)
	tokH := handlers.NewTokenHandler(s.store)
	adm.GET("/tokens", tokH.List)
	adm.POST("/tokens", tokH.Create)
	adm.DELETE("/tokens/:id", tokH.Delete)
}

func (s *Server) serveIndex(webFS embed.FS) echo.HandlerFunc {
	return func(c echo.Context) error {
		data, err := webFS.ReadFile("web/index.html")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "index not found")
		}
		return c.HTMLBlob(http.StatusOK, data)
	}
}
