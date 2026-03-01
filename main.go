package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/22peacemaker/open-mon-stack/internal/api"
	"github.com/22peacemaker/open-mon-stack/internal/storage"
)

// Set by goreleaser via -ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

//go:embed web
var webFS embed.FS

func main() {
	var (
		port        = flag.Int("port", 8080, "HTTP port to listen on")
		dataDir     = flag.String("data", defaultDataDir(), "Directory for storing data and stack configs")
		showVersion = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("open-mon-stack %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	store, err := storage.New(*dataDir)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}

	srv := api.New(store, *dataDir, webFS)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Open Mon Stack %s running at http://localhost%s", version, addr)
	log.Printf("Data directory: %s", *dataDir)

	if err := srv.Start(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".open-mon-stack"
	}
	return filepath.Join(home, ".open-mon-stack")
}
