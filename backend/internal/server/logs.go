package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"app-suite/internal/logging"
	"app-suite/internal/model"
)

// Log-viewer read limits. The tail cap bounds the work a single request can do
// regardless of file size (older history lives in the rotated files); the entry
// limit bounds the response payload.
const (
	logsReadCapBytes = 4 << 20 // read at most the last 4 MB of the active log
	logsDefaultLimit = 200
	logsMaxLimit     = 1000
)

// slogLevelValue maps an slog level name (as written to the JSON log, e.g.
// "INFO"/"WARN") to its numeric severity, so the viewer can filter by a minimum
// level. Case-insensitive; ok is false for an empty/unknown value.
func slogLevelValue(name string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DEBUG":
		return -4, true
	case "INFO":
		return 0, true
	case "WARN", "WARNING":
		return 4, true
	case "ERROR":
		return 8, true
	}
	return 0, false
}

// logLevelName maps an slog.Level to the lowercase API/UI name.
func logLevelName(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// parseLogLevelName maps an API level name to an slog.Level for SetLevel.
func parseLogLevelName(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return 0, false
}

// handleLogs returns the tail of the server's JSON log file, newest-first,
// optionally filtered by a minimum level and a case-insensitive substring query.
// Admin-only: the log stream contains IPs, usernames, and internal error detail.
//
//	Endpoint: GET /api/logs?level=<debug|info|warn|error>&q=<text>&limit=<n>
//	Auth:     admin
//	Response: {"entries":[LogEntry…], "file":"…", "truncated":bool, "level":"…"}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	curLevel := logLevelName(logging.CurrentLevel())
	// No file sink configured (e.g. -log-file=""): nothing to show, not an error.
	if s.logFile == "" {
		writeJSON(w, http.StatusOK, model.LogsResponse{Entries: []model.LogEntry{}, Level: curLevel})
		return
	}

	limit := logsDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, logsMaxLimit)
		}
	}
	minLevel, hasMin := slogLevelValue(r.URL.Query().Get("level"))
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	raw, truncated, err := tailFile(s.logFile, logsReadCapBytes)
	if err != nil {
		if os.IsNotExist(err) {
			// The file appears once the first line is logged; treat as empty.
			writeJSON(w, http.StatusOK, model.LogsResponse{Entries: []model.LogEntry{}, File: s.logFile, Level: curLevel})
			return
		}
		writeInternalError(w, "read log file", err)
		return
	}

	entries := parseLogLines(raw, minLevel, hasMin, query)
	slices.Reverse(entries) // file is oldest-first; the viewer wants newest-first
	if len(entries) > limit {
		entries = entries[:limit]
	}
	writeJSON(w, http.StatusOK, model.LogsResponse{Entries: entries, File: s.logFile, Truncated: truncated, Level: curLevel})
}

// handleLogLevelSet changes the process-wide minimum log level at runtime
// (admin-only). Turning DEBUG on captures far more detail; turning it back to
// INFO quiets it again — no restart, effective immediately across stdout, the
// rotating file, and the live tail. Reverts to the startup default on restart.
//
//	Endpoint: POST /api/logs/level  {"level":"debug|info|warn|error"}
//	Auth:     admin
func (s *Server) handleLogLevelSet(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	body, err := readJSON[struct {
		Level string `json:"level"`
	}](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	lvl, ok := parseLogLevelName(body.Level)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid level (use debug, info, warn, or error)")
		return
	}
	logging.SetLevel(lvl)
	// Logged at INFO so the change itself is always recorded (audit trail), even
	// when switching down to a level that would suppress its own DEBUG line.
	slog.Info("log level changed", "level", logLevelName(lvl))
	writeJSON(w, http.StatusOK, model.LogLevelResponse{Level: logLevelName(lvl)})
}

// tailFile reads at most the last capBytes of the file. When the file is larger
// than the cap it seeks to the tail and drops the first (partial) line, and
// reports truncated=true so the caller can tell the viewer older lines exist.
func tailFile(path string, capBytes int64) (data []byte, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	start := int64(0)
	if fi.Size() > capBytes {
		start = fi.Size() - capBytes
		truncated = true
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, false, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}
	if truncated {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:] // discard the partial first line
		}
	}
	return buf, truncated, nil
}

// parseLogLines parses NDJSON log lines into entries, skipping blank and
// non-JSON lines (jlv does the same). When hasMin is set, entries below minLevel
// are dropped; when query is non-empty, only lines containing it (case-
// insensitive, matched against the whole raw line) are kept. Per-line promotion
// of time/level/msg → typed fields is shared with the live tail via
// model.ParseLogEntry.
func parseLogLines(raw []byte, minLevel int, hasMin bool, query string) []model.LogEntry {
	out := []model.LogEntry{}
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		e, ok := model.ParseLogEntry(line)
		if !ok {
			continue // not a JSON object — skip
		}
		if hasMin {
			if lvl, ok := slogLevelValue(e.Level); !ok || lvl < minLevel {
				continue
			}
		}
		if query != "" && !bytes.Contains(bytes.ToLower(line), []byte(query)) {
			continue
		}
		out = append(out, e)
	}
	return out
}
