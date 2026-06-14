// Package main is the entry point for the App Suite API server.
// It parses configuration flags, initializes the SQLite database,
// creates the WebSocket hub, and starts the HTTP server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "/opt/app-suite/data/database.sqlite", "SQLite database path")
	webRoot := flag.String("webroot", "/var/www/www.yoursite.com", "Web root directory for static assets (e.g. image uploads)")
	secret := flag.String("secret", "", "Session cookie secret (env APPSUITE_SESSION_SECRET or random if empty)")
	password := flag.String("password", "", "Admin password (env APPSUITE_ADMIN_PASSWORD or built-in default)")
	flag.Parse()

	if *password == "" {
		*password = os.Getenv("APPSUITE_ADMIN_PASSWORD")
	}
	if *password == "" {
		*password = "changeme1234"
		slog.Warn("No admin password provided; using insecure built-in default. Set -password or APPSUITE_ADMIN_PASSWORD.")
	}

	// Session secret: flag > env > random
	finalSecret := *secret
	if finalSecret == "" {
		finalSecret = os.Getenv("APPSUITE_SESSION_SECRET")
	}
	if finalSecret == "" {
		// Generate a secure random 32-byte secret
		b := make([]byte, 32)
		_, err := rand.Read(b)
		if err != nil {
			slog.Error("failed to generate random session secret", "error", err)
			os.Exit(1)
		}
		finalSecret = base64.RawURLEncoding.EncodeToString(b)
		slog.Warn("No session secret provided; generated random secret (sessions will be invalidated on restart)")
	}

	db, err := store.New(*dbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	hub := ws.NewHub()
	srv := server.New(db, hub, finalSecret, *password, *webRoot)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	// Background scheduler: posts due book-club event embeds to Discord. Tied to
	// a context cancelled on shutdown so the goroutine exits cleanly.
	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	go srv.RunEventScheduler(schedCtx)
	go srv.RunAnnouncementScheduler(schedCtx)

	// Graceful shutdown: listen for SIGINT/SIGTERM.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("App Suite API server starting", "addr", *addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("shutdown signal received, shutting down gracefully…")
	cancelSched() // stop the background event scheduler

	// Give in-flight requests 10 seconds to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close all WebSocket connections first.
	hub.Shutdown(ctx)

	// Then shut down the HTTP server.
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
