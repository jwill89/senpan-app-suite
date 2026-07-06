// Package logging configures the process-wide slog logger.
//
// The app logs structured JSON. Output always goes to stdout (captured by
// systemd/journald for ops), and — when a file path is configured — ALSO to a
// rotating file so the in-app log viewer has a durable, machine-readable source
// that survives restarts and can be tailed without SSH.
//
// Rotation is handled by timberjack (a lumberjack fork with wall-clock
// scheduling): the active file rotates daily at local midnight, rotated files
// are zstd-compressed, and retention is bounded by count and age so the log
// directory can't grow without limit.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/DeRuina/timberjack"
)

// tailSink taps the log stream for the live viewer. It is an io.Writer plugged
// into the log MultiWriter, so its Write receives each fully-formatted JSON log
// line (all attributes included) exactly once. It forwards the raw line to a
// callback registered later — once the WebSocket hub exists — and is a cheap
// no-op until then (and whenever nobody is watching). Tapping the stream here
// (rather than tailing the file) means the live tail also works when file
// logging is disabled, and never has to deal with rotation.
type sink struct {
	mu sync.RWMutex
	cb func([]byte)
}

var tailSink = &sink{}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.RLock()
	cb := s.cb
	s.mu.RUnlock()
	if cb != nil {
		cb(p) // cb must parse synchronously and not retain p (slog reuses the buffer)
	}
	return len(p), nil
}

// SetTailSink registers the callback invoked with each JSON log line for the
// live tail. Pass nil to detach. Wired from main once the WebSocket hub exists.
func SetTailSink(cb func(line []byte)) {
	tailSink.mu.Lock()
	tailSink.cb = cb
	tailSink.mu.Unlock()
}

// Rotation/retention defaults for the file sink. MaxSize is a safety cap that
// forces a mid-day rotation if a single day's log somehow gets huge; normal
// rotation is time-based (daily at midnight) via RotateAt.
const (
	logMaxSizeMB  = 100 // rotate a single file if it exceeds this (safety net)
	logMaxBackups = 14  // keep at most this many rotated files
	logMaxAgeDays = 30  // and none older than this
)

// levelVar is the process-wide minimum log level, held in a slog.LevelVar so it
// can be changed at runtime (see SetLevel) — the admin log viewer flips DEBUG on
// and off live without a restart, and the change takes effect immediately across
// stdout, the file, and the live tail.
var levelVar = new(slog.LevelVar)

// SetLevel changes the minimum log level at runtime (thread-safe).
func SetLevel(l slog.Level) { levelVar.Set(l) }

// CurrentLevel reports the active minimum log level.
func CurrentLevel() slog.Level { return levelVar.Level() }

// Setup installs slog's default logger as a JSON handler at the given initial
// level (changeable later via SetLevel). It always writes to stdout; when
// filePath is non-empty it additionally writes to a rotating file at that path
// (daily midnight rotation, zstd, bounded retention), creating the parent
// directory if needed.
//
// It returns the file sink as an io.Closer (nil when no file is configured) so
// the caller can Close it on shutdown to flush and stop the rotation goroutine,
// and a non-nil error if the file sink could NOT be set up. Even on that error
// the default logger is still installed (stdout only), so logging always works
// and the caller can just log the error as a warning.
func Setup(filePath string, level slog.Level) (io.Closer, error) {
	levelVar.Set(level)

	// stdout (journald) + the live-tail tap are always present; the rotating file
	// is added when configured.
	writers := []io.Writer{os.Stdout, tailSink}

	var closer io.Closer
	var setupErr error
	if filePath != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			setupErr = err
		} else {
			tj := &timberjack.Logger{
				Filename:    filePath,
				MaxSize:     logMaxSizeMB,
				MaxBackups:  logMaxBackups,
				MaxAge:      logMaxAgeDays,
				Compression: "zstd",            // more efficient than gzip
				LocalTime:   true,              // rotated names + midnight in local time
				RotateAt:    []string{"00:00"}, // rotate daily at local midnight
			}
			writers = append(writers, tj)
			closer = tj
		}
	}

	handler := slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: levelVar})
	slog.SetDefault(slog.New(handler))
	return closer, setupErr
}
