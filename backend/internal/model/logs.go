package model

import (
	"encoding/json"
	"fmt"
)

// LogEntry is one parsed record from the server's JSON log file (slog output).
// Time/Level/Message are the standard slog keys; Fields carries any additional
// structured attributes the log call attached (method, path, status, ip, error,
// …), kept as a free-form map so the viewer renders them generically without the
// model having to know every attribute a call site might add.
type LogEntry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// LogsResponse is the tail of the server log returned by GET /api/logs, after
// level/text filtering and ordered newest-first. Truncated is true when the read
// cap dropped older lines (their history lives in the rotated/compressed files).
// Level is the current process-wide minimum level (so the viewer can reflect
// whether live DEBUG is on).
type LogsResponse struct {
	Entries   []LogEntry `json:"entries"`
	File      string     `json:"file"`
	Truncated bool       `json:"truncated"`
	Level     string     `json:"level"`
}

// LogLevelResponse reports the process-wide minimum log level after a change
// (POST /api/logs/level) — one of "debug", "info", "warn", "error".
type LogLevelResponse struct {
	Level string `json:"level"`
}

// ParseLogEntry parses one slog JSON log line into a LogEntry, promoting the
// standard time/level/msg keys and carrying every other attribute in Fields.
// ok is false for a blank or non-JSON line. Shared by the REST tail endpoint and
// the live WebSocket tail so both surface identical entries.
func ParseLogEntry(line []byte) (LogEntry, bool) {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return LogEntry{}, false
	}
	e := LogEntry{
		Time:    logFieldString(m["time"]),
		Level:   logFieldString(m["level"]),
		Message: logFieldString(m["msg"]),
	}
	delete(m, "time")
	delete(m, "level")
	delete(m, "msg")
	if len(m) > 0 {
		e.Fields = m
	}
	return e, true
}

// logFieldString coerces a decoded JSON value to a string for the promoted
// time/level/msg columns (they are strings in slog output, but stay defensive).
func logFieldString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprint(s)
	}
}
