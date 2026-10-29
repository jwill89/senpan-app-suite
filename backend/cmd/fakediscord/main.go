// Command fakediscord runs the in-memory fake Discord (internal/discord/discordtest) on a
// local port, for clicking through the app's Discord features without a real bot.
//
// Start it, then run the backend with -discord-api-url pointing at it:
//
//	go run ./cmd/fakediscord -addr 127.0.0.1:8099
//	go run . -addr :8080 -db <scratch>.sqlite -webroot <scratch> -log-file "" -discord-api-url http://127.0.0.1:8099
//
// Paste any token on System -> Discord: the fake accepts every token. It serves the
// Senpan Tea House server (999011226568888410) with a few channels and roles, logs every
// request, and offers control routes for the cases a real server can't be made to produce
// on demand:
//
//	GET  /_fake/messages                       every message still up, as JSON
//	POST /_fake/fail?match=POST%20/channels/&status=500
//	                                           fail the next matching request once
//	POST /_fake/ratelimit?match=POST%20/channels/&seconds=2
//	                                           rate-limit the next matching request once
//	POST /_fake/delete/{id}                    delete a message "in Discord" (the bot's
//	                                           next edit or delete of it gets a 404)
//	POST /_fake/webhook?id=1&token=t&channel=200000000000000003[&guild=...]
//	                                           register an old webhook for the import to
//	                                           find; save https://discord.com/api/webhooks/1/t
//	                                           as its URL in a database from before v70
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"app-suite/internal/discord/discordtest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "listen address (keep it on localhost)")
	flag.Parse()

	if host, _, err := net.SplitHostPort(*addr); err != nil || (host != "127.0.0.1" && host != "localhost") {
		log.Fatal("fakediscord only listens on localhost")
	}

	fake := discordtest.New()
	fake.AnyToken = true

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_fake/messages", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(fake.Messages())
	})
	mux.HandleFunc("POST /_fake/fail", func(w http.ResponseWriter, r *http.Request) {
		status, err := strconv.Atoi(r.URL.Query().Get("status"))
		if err != nil || status < 400 || status > 599 {
			http.Error(w, "status must be 400-599", http.StatusBadRequest)
			return
		}
		fake.Fail(r.URL.Query().Get("match"), status, `{"message":"Failure injected by fakediscord","code":0}`)
		log.Printf("next %q request will fail with %d", r.URL.Query().Get("match"), status)
	})
	mux.HandleFunc("POST /_fake/ratelimit", func(w http.ResponseWriter, r *http.Request) {
		seconds, err := strconv.ParseFloat(r.URL.Query().Get("seconds"), 64)
		if err != nil || seconds <= 0 {
			http.Error(w, "seconds must be positive", http.StatusBadRequest)
			return
		}
		fake.RateLimit(r.URL.Query().Get("match"), seconds)
		log.Printf("next %q request will be rate-limited for %.1fs", r.URL.Query().Get("match"), seconds)
	})
	mux.HandleFunc("POST /_fake/delete/{id}", func(_ http.ResponseWriter, r *http.Request) {
		fake.DeleteInDiscord(r.PathValue("id"))
		log.Printf("message %s deleted in Discord", r.PathValue("id"))
	})
	mux.HandleFunc("POST /_fake/webhook", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("id") == "" || q.Get("token") == "" || q.Get("channel") == "" {
			http.Error(w, "id, token and channel are required", http.StatusBadRequest)
			return
		}
		guild := q.Get("guild")
		if guild == "" {
			guild = discordtest.GuildID
		}
		fake.AddWebhook(q.Get("id"), discordtest.Webhook{
			Token: q.Get("token"), Name: "Old webhook " + q.Get("id"), GuildID: guild, ChannelID: q.Get("channel"),
		})
		log.Printf("webhook %s registered in server %s, channel %s", q.Get("id"), guild, q.Get("channel"))
	})
	mux.Handle("/", fake.Handler())

	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		mux.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})

	log.Printf("fake Discord listening on http://%s (any bot token accepted)", *addr)
	srv := &http.Server{Addr: *addr, Handler: logged, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// statusRecorder notes the status a handler wrote, for the request log. It passes the
// hijacker through so the gateway's websocket upgrade still works.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
