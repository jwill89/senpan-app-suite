package model

import (
	"encoding/json"
	"fmt"
)

// LogEntry is one parsed record from the server's JSON log file (slog output).
// Time/Level/Message are the standard slog keys; Fields carries any additional
// structured attributes the log call attached (method, path, status, ip, error,
// ...), kept as a free-form map so the viewer renders them generically without the
// model having to know every attribute a call site might add.
type LogEntry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// ParseLogEntry parses one slog JSON log line into a LogEntry, promoting the
// standard time/level/msg keys and carrying every other attribute in Fields.
// ok is false for a blank or non-JSON line. Shared by the REST tail endpoint and
// the live log events so both surface identical entries.
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
