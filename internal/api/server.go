package api

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/22peacemaker/open-mon-stack/internal/api/handlers"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

type Server struct {
	echo       *echo.Echo
	store      *storage.Store
	appDataDir string
}

func New(store *storage.Store, appDataDir string, webFS embed.FS) *Server {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	s := &Server{echo: e, store: store, appDataDir: appDataDir}
	s.setupRoutes(webFS)
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

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

	api := e.Group("/api")

	// Central stack
	sh := handlers.NewStackHandler(s.store, s.appDataDir)
	api.GET("/stack/config", sh.GetConfig)
	api.PUT("/stack/config", sh.SaveConfig)
	api.GET("/stack/status", sh.LiveStatus)
	api.POST("/stack/deploy", sh.Deploy)
	api.POST("/stack/stop", sh.Stop)
	api.GET("/stack/logs", sh.StreamLogs)

	// Targets (monitored servers)
	th := handlers.NewTargetHandler(s.store, s.appDataDir)
	api.GET("/targets", th.List)
	api.POST("/targets", th.Create)
	api.GET("/targets/:id", th.Get)
	api.PUT("/targets/:id", th.Update)
	api.DELETE("/targets/:id", th.Delete)
	api.GET("/targets/:id/script", th.AgentScript)
	api.GET("/targets/:id/script/raw", func(c echo.Context) error {
		c.QueryParams().Set("raw", "1")
		return th.AgentScript(c)
	})

	// Agent catalog
	api.GET("/agents", th.Agents)
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
