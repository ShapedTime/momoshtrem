package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting momoshtrem", "config", *configPath)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Ensure required directories exist
	if err := cfg.EnsureDirectories(); err != nil {
		slog.Error("Failed to create directories", "error", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := library.NewDB(cfg.Database.Path)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("Database initialized", "path", cfg.Database.Path)

	// Initialize repositories
	movieRepo := library.NewMovieRepository(db)
	showRepo := library.NewShowRepository(db)
	assignmentRepo := library.NewAssignmentRepository(db)

	// Initialize TMDB client
	var tmdbClient *tmdb.Client
	if cfg.TMDB.APIKey != "" {
		tmdbClient = tmdb.NewClient(cfg.TMDB.APIKey)
		slog.Info("TMDB client initialized")
	} else {
		slog.Warn("TMDB API key not configured, some features will be unavailable")
		tmdbClient = tmdb.NewClient("") // Empty client will fail on API calls
	}

	// Initialize VFS
	libraryFS := vfs.NewLibraryFS(movieRepo, showRepo, assignmentRepo)
	slog.Info("VFS initialized")

	// Initialize servers
	apiServer := api.NewServer(movieRepo, showRepo, assignmentRepo, tmdbClient)
	webdavServer := webdav.NewServer(libraryFS)

	// Start HTTP servers
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: apiServer.Handler(),
	}

	webdavHTTPServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.WebDAVPort),
		Handler: webdavServer.Handler(),
	}

	// Start servers in goroutines
	go func() {
		slog.Info("Starting REST API server", "port", cfg.Server.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("REST API server error", "error", err)
		}
	}()

	go func() {
		slog.Info("Starting WebDAV server", "port", cfg.Server.WebDAVPort)
		if err := webdavHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("WebDAV server error", "error", err)
		}
	}()

	slog.Info("momoshtrem is ready",
		"api_url", fmt.Sprintf("http://localhost:%d/api", cfg.Server.HTTPPort),
		"webdav_url", fmt.Sprintf("http://localhost:%d", cfg.Server.WebDAVPort),
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("Received signal, shutting down", "signal", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("REST API server shutdown error", "error", err)
	}
	if err := webdavHTTPServer.Shutdown(ctx); err != nil {
		slog.Error("WebDAV server shutdown error", "error", err)
	}

	slog.Info("momoshtrem stopped")
}
