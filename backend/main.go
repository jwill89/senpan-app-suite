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
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/logging"
	"app-suite/internal/model"
	"app-suite/internal/secretbox"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

// main keeps the process's exit status truthful. run does the real work and
// returns an error for anything fatal; a non-zero exit is what tells systemd's
// Restart=on-failure that this was a crash and not a clean stop. Exiting here
// rather than inside run means run's deferred cleanup (db.Close, the log closer)
// has already completed by the time the process goes away.
func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "/opt/app-suite/data/database.sqlite", "SQLite database path")
	webRoot := flag.String("webroot", "/var/www/www.yoursite.com", "Web root directory for static assets (e.g. image uploads)")
	secret := flag.String("secret", "", "Session cookie secret (env APPSUITE_SESSION_SECRET or random if empty)")
	corsOrigins := flag.String("cors-origins", "", "Comma-separated CORS allowlist of cross-origin sites (env APPSUITE_CORS_ORIGINS; empty = same-origin only, no CORS headers)")
	turnstileSecret := flag.String("turnstile-secret", "", "Cloudflare Turnstile secret key (env APPSUITE_TURNSTILE_SECRET; empty = login bot check disabled)")
	turnstileSiteKey := flag.String("turnstile-sitekey", "", "Cloudflare Turnstile public site key (env APPSUITE_TURNSTILE_SITEKEY)")
	logFile := flag.String("log-file", "/var/log/senpan/senpan.log", "Rotating JSON log file path (daily midnight rotation); empty = stdout only")
	discordDryRun := flag.Bool("discord-dry-run", false, "Suppress every Discord bot write (post, edit, delete) and only log it (env APPSUITE_DISCORD_DRY_RUN=1); dev safety")
	discordRedirect := flag.String("discord-redirect-channel", "", "Send every new Discord bot post to this channel ID instead (env APPSUITE_DISCORD_REDIRECT_CHANNEL); dev safety")
	discordAPI := flag.String("discord-api-url", "", "Talk to a local fake Discord at this http://localhost URL instead of discord.com (env APPSUITE_DISCORD_API_URL); local testing only")
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

	// CORS allowlist: flag > env. Normally empty - the SPA and API are
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
	// Only a configured secret can protect stored values: a random one is gone at the next
	// restart, and anything sealed under it with it.
	secretConfigured := finalSecret != ""
	if finalSecret == "" {
		// Generate a secure random 32-byte secret
		b := make([]byte, 32)
		_, err := rand.Read(b)
		if err != nil {
			return fmt.Errorf("generate random session secret: %w", err)
		}
		finalSecret = base64.RawURLEncoding.EncodeToString(b)
		slog.Warn("No session secret provided; generated random secret (sessions will be invalidated on restart)")
	}

	db, err := store.New(*dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
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

	// The Discord bot. Its token is stored sealed under a key derived from the session
	// secret, so a copy of the database is useless without this server's secret; with no
	// configured secret there is no key, and the Discord page says a token can't be stored.
	var tokenBox *secretbox.Box
	if secretConfigured {
		tokenBox, err = secretbox.New(finalSecret, server.DiscordTokenPurpose)
		if err != nil {
			return fmt.Errorf("derive the discord token key: %w", err)
		}
	}
	apiBase := *discordAPI
	if apiBase == "" {
		apiBase = os.Getenv("APPSUITE_DISCORD_API_URL")
	}
	if apiBase != "" {
		// A fake Discord for local testing only. Anything else would send the bot token
		// somewhere it doesn't belong, so refuse to start rather than guess.
		if u, perr := url.Parse(apiBase); perr != nil || u.Scheme != "http" ||
			(u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
			return fmt.Errorf("-discord-api-url must be an http://localhost or http://127.0.0.1 address")
		}
		slog.Warn("DISCORD FAKE: the bot talks to a local fake Discord", "url", apiBase)
	}
	botDryRun := *discordDryRun || os.Getenv("APPSUITE_DISCORD_DRY_RUN") == "1"
	botRedirect := *discordRedirect
	if botRedirect == "" {
		botRedirect = os.Getenv("APPSUITE_DISCORD_REDIRECT_CHANNEL")
	}
	switch {
	case botDryRun:
		slog.Warn("DISCORD BOT DRY RUN: every bot post, edit and delete is suppressed and only logged")
	case botRedirect != "":
		slog.Warn("DISCORD BOT REDIRECT: every new bot post goes to one channel", "channel", botRedirect)
	}
	bot := discord.NewService(apiBase, "")
	srv.SetDiscord(bot, tokenBox, botDryRun, botRedirect)

	// Live log tail: forward each JSON log line to admin WebSocket clients as a
	// {"type":"log","entry":...} message. Gated on an admin actually watching so the
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

	// Tea-room lock scheduler: unlocks rooms whose set expiry has passed and tells
	// the admin clients (the in-game plugin alerts its operator). Same shared
	// context, so it stops sweeping before the database closes.
	roomLockDone := make(chan struct{})
	go func() {
		defer close(roomLockDone)
		srv.RunTeaRoomLockScheduler(schedCtx)
	}()

	// The bot's gateway session (keeps it online). Same shared context; it stops after the
	// schedulers that might still be posting through it.
	botDone := make(chan struct{})
	go func() {
		defer close(botDone)
		bot.Run(schedCtx)
	}()

	// In-place edits of tea-room and affiliate posts, queued by their handlers and the
	// lock sweeper. On shutdown it sends what is still pending before returning.
	discordEditsDone := make(chan struct{})
	go func() {
		defer close(discordEditsDone)
		srv.RunDiscordWorker(schedCtx)
	}()

	// The one-time move off webhooks: look up any old webhook URLs the v70 migration set
	// aside and give their systems the channels they posted to. Quick, and a no-op once
	// done; waited for on shutdown so a result it has just written isn't cut off.
	importDone := make(chan struct{})
	go func() {
		defer close(importDone)
		srv.RunDiscordImport(schedCtx)
	}()

	// Graceful shutdown: listen for SIGINT/SIGTERM.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// A failed ListenAndServe (e.g. the port is already in use) must run the SAME
	// graceful-shutdown path below rather than exiting on the spot - otherwise the
	// deferred cleanup (db.Close, scheduler cancel) is skipped and in-flight state
	// can be left inconsistent. Send the error to a buffered channel the select
	// awaits, then return it so the process still exits non-zero.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("App Suite API server starting", "addr", *addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Non-nil only when the server itself failed. A signal-driven stop is a clean
	// exit; a bind failure is not, and reporting both as success is what let a
	// backend that could never start sit there looking healthy.
	var fatal error
	select {
	case <-shutdown:
		slog.Info("shutdown signal received, shutting down gracefully...")
	case err := <-serverErr:
		slog.Error("server failed, shutting down", "error", err)
		fatal = err
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

	// And the tea-room lock sweeper, so an unlock it has already written is not
	// interrupted part-way by db.Close(). Bounded by the same shutdown deadline.
	select {
	case <-roomLockDone:
	case <-ctx.Done():
		slog.Warn("tea-room lock scheduler did not stop within the shutdown deadline")
	}

	// The Discord edit worker, which may still be sending the last queued edits (and so
	// writing their records), then the bot's gateway session, and the webhook import.
	select {
	case <-discordEditsDone:
	case <-ctx.Done():
		slog.Warn("discord edit worker did not stop within the shutdown deadline")
	}
	select {
	case <-botDone:
	case <-ctx.Done():
		slog.Warn("discord gateway did not stop within the shutdown deadline")
	}
	select {
	case <-importDone:
	case <-ctx.Done():
		slog.Warn("discord webhook import did not stop within the shutdown deadline")
	}

	// Close all WebSocket connections first.
	hub.Shutdown(ctx)

	// Then shut down the HTTP server.
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("server stopped")
	return fatal
}
