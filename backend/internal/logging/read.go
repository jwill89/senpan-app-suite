package logging

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"

	"app-suite/internal/model"
)

// ReadCap is the most of the active log file a read takes: its last 4 MB.
const ReadCap = 4 << 20

// Filter narrows a read: a level and those above it ("" for every level), text the
// line must hold (case-insensitive, "" for any), and the most entries to return.
type Filter struct {
	Level string
	Query string
	Limit int
}

// Read returns the newest entries of the JSON log file at path that pass f, newest
// first, and whether the file was longer than ReadCap, so older entries were not
// read. A file that doesn't exist yet reads as empty.
func Read(path string, f Filter) (entries []model.LogEntry, truncated bool, err error) {
	raw, truncated, err := tail(path, ReadCap)
	if os.IsNotExist(err) {
		return []model.LogEntry{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	minLevel, hasMin := LevelValue(f.Level)
	query := []byte(strings.ToLower(strings.TrimSpace(f.Query)))
	entries = []model.LogEntry{}
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		e, ok := model.ParseLogEntry(line)
		if !ok {
			continue // not a JSON object
		}
		if hasMin {
			if lvl, ok := LevelValue(e.Level); !ok || lvl < minLevel {
				continue
			}
		}
		if len(query) > 0 && !bytes.Contains(bytes.ToLower(line), query) {
			continue
		}
		entries = append(entries, e)
	}
	slices.Reverse(entries) // the file is oldest first; a reader wants the newest
	if f.Limit > 0 && len(entries) > f.Limit {
		entries = entries[:f.Limit]
	}
	return entries, truncated, nil
}

// tail reads the last capBytes of a file, dropping the partial line it starts in.
func tail(path string, capBytes int64) (data []byte, truncated bool, err error) {
	f, err := os.Open(path) //nolint:gosec // the operator's -log-file, never a request's
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }() // opened to read: closing it has nothing to report
	fi, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	start := int64(0)
	if fi.Size() > capBytes {
		start, truncated = fi.Size()-capBytes, true
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
			buf = buf[i+1:]
		}
	}
	return buf, truncated, nil
}

// LevelValue is a level's name ("debug", "info", "warn" or "warning", "error", any
// case) as slog's number, and whether it is one.
func LevelValue(name string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
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

// LevelName is the name a level is shown under: "debug", "info", "warn" or "error".
func LevelName(l slog.Level) string {
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
