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
	"strings"
	"syscall"
	"time"

	"app-suite/internal/logging"
	"app-suite/internal/model"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "/opt/app-suite/data/database.sqlite", "SQLite database path")
	webRoot := flag.String("webroot", "/var/www/www.yoursite.com", "Web root directory for static assets (e.g. image uploads)")
	secret := flag.String("secret", "", "Session cookie secret (env APPSUITE_SESSION_SECRET or random if empty)")
	corsOrigins := flag.String("cors-origins", "", "Comma-separated CORS allowlist of cross-origin sites (env APPSUITE_CORS_ORIGINS; empty = same-origin only, no CORS headers)")
	turnstileSecret := flag.String("turnstile-secret", "", "Cloudflare Turnstile secret key (env APPSUITE_TURNSTILE_SECRET; empty = login bot check disabled)")
	turnstileSiteKey := flag.String("turnstile-sitekey", "", "Cloudflare Turnstile public site key (env APPSUITE_TURNSTILE_SITEKEY)")
	logFile := flag.String("log-file", "/var/log/senpan/senpan.log", "Rotating JSON log file path (daily midnight rotation); empty = stdout only")
	flag.Parse()

	// Logging: install the JSON slog handler before anything logs. Always writes
	// to stdout (journald); also to the rotating file when -log-file is set (prod
	// default). On a file-sink failure we still log to stdout and warn. Dev on a
	// non-Linux box should pass -log-file="" to avoid creating a /var/log tree.
	logCloser, logErr := logging.Setup(*logFile, slog.LevelInfo)
	if logCloser != nil {
		defer logCloser.Close()
	}
	if logErr != nil {
		slog.Warn("file logging disabled; using stdout only", "path", *logFile, "error", logErr)
	}

	// CORS allowlist: flag > env. Normally empty — the SPA and API are
	// same-origin in both prod (Apache) and dev (Vite proxies /api).
	originsCSV := *corsOrigins
	if originsCSV == "" {
		originsCSV = os.Getenv("APPSUITE_CORS_ORIGINS")
	}
	var allowedOrigins []string
	if originsCSV != "" {
		allowedOrigins = strings.Split(originsCSV, ",")
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

	// Cloudflare Turnstile login bot check: flag > env. Disabled when no secret.
	tsSecret := *turnstileSecret
	if tsSecret == "" {
		tsSecret = os.Getenv("APPSUITE_TURNSTILE_SECRET")
	}
	tsSiteKey := *turnstileSiteKey
	if tsSiteKey == "" {
		tsSiteKey = os.Getenv("APPSUITE_TURNSTILE_SITEKEY")
	}

	hub := ws.NewHub()
	srv := server.New(db, hub, finalSecret, *webRoot, allowedOrigins)
	srv.SetTurnstile(tsSecret, tsSiteKey)
	srv.SetLogFile(*logFile) // GET /api/logs tails this file

	// Live log tail: forward each JSON log line to admin WebSocket clients as a
	// {"type":"log","entry":…} message. Gated on an admin actually watching so the
	// parse/broadcast is skipped entirely otherwise, and lossy so a burst can't
	// disconnect anyone. model.ParseLogEntry runs synchronously and copies out of
	// the line, so the slog buffer is not retained.
	logging.SetTailSink(func(line []byte) {
		if !hub.HasAdminClients() {
			return
		}
		if entry, ok := model.ParseLogEntry(line); ok {
			hub.BroadcastLog(map[string]any{"type": "log", "entry": entry})
		}
	})
	srv.SetOpenAPISpec(openAPISpec) // GET /api/docs + /api/openapi.yaml
	if tsSecret != "" {
		slog.Info("Cloudflare Turnstile bot check enabled on the login form")
	} else {
		slog.Warn("Cloudflare Turnstile not configured; login bot check disabled")
	}

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv,
		// Timeouts guard against slow-client / idle-socket resource exhaustion.
		// Deliberately NO WriteTimeout: the /api/ws WebSocket is a long-lived
		// connection a global write deadline would sever. ReadHeaderTimeout bounds
		// the slow-headers (Slowloris) attack; ReadTimeout bounds the full request
		// read; IdleTimeout reaps kept-alive connections.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Background scheduler: posts due announcement embeds to Discord. Tied to a
	// context cancelled on shutdown so the goroutine exits cleanly. schedDone is
	// closed when it has fully returned, so shutdown can wait for an in-flight
	// sweep before closing the DB.
	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		srv.RunAnnouncementScheduler(schedCtx)
	}()

	// Automatic bingo-draw scheduler: draws a number on the admin-chosen interval
	// while a game has "auto" switched on. Shares the shutdown-cancelled context so
	// it stops drawing before the database closes.
	autoDone := make(chan struct{})
	go func() {
		defer close(autoDone)
		srv.RunAutoDrawScheduler(schedCtx)
	}()

	// Graceful shutdown: listen for SIGINT/SIGTERM.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// A failed ListenAndServe (e.g. the port is already in use) must run the SAME
	// graceful-shutdown path below rather than os.Exit(1) — otherwise the deferred
	// cleanup (db.Close, scheduler cancel) is skipped and in-flight state can be
	// left inconsistent. Send the error to a buffered channel the select awaits.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("App Suite API server starting", "addr", *addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-shutdown:
		slog.Info("shutdown signal received, shutting down gracefully…")
	case err := <-serverErr:
		slog.Error("server failed, shutting down", "error", err)
	}
	cancelSched() // stop the background announcement scheduler

	// Give in-flight requests 10 seconds to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for the announcement scheduler to finish any in-flight sweep before
	// the deferred db.Close(): a Discord post that already succeeded must get its
	// cursor advanced (MarkAnnouncementPosted), or it re-posts on the next boot.
	// Bounded by the shutdown deadline so a hung post can't block forever.
	select {
	case <-schedDone:
	case <-ctx.Done():
		slog.Warn("announcement scheduler did not stop within the shutdown deadline")
	}

	// Likewise wait for the auto-draw scheduler to return so an in-flight draw
	// finishes writing before db.Close(). Bounded by the same shutdown deadline.
	select {
	case <-autoDone:
	case <-ctx.Done():
		slog.Warn("auto-draw scheduler did not stop within the shutdown deadline")
	}

	// Close all WebSocket connections first.
	hub.Shutdown(ctx)

	// Then shut down the HTTP server.
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
