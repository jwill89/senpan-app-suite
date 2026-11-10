package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/logging"
)

// LogEntry is one line of the server's JSON log.
type LogEntry struct {
	Time    string         `json:"time" doc:"When it was logged, RFC 3339."`
	Level   string         `json:"level" doc:"Its level, as logged: DEBUG, INFO, WARN or ERROR."`
	Message string         `json:"message" doc:"The log message."`
	Fields  map[string]any `json:"fields,omitempty" doc:"Its other attributes, by name."`
}

// Logs is a page of the server's log, newest first.
type Logs struct {
	Entries   []LogEntry `json:"entries" nullable:"false" doc:"The entries that pass the filter, newest first."`
	File      string     `json:"file" doc:"The log file read; empty when the server logs to stdout only."`
	Truncated bool       `json:"truncated" doc:"Whether the file was longer than the 4 MB a read takes, so older entries were not searched."`
	Level     string     `json:"level" enum:"debug,info,warn,error" doc:"The level the server is logging at now."`
}

// LogLevel is the level the server logs at.
type LogLevel struct {
	Level string `json:"level" enum:"debug,info,warn,error" doc:"The least severe level written to the log. Debug adds a detailed line for every request."`
}

type logsInput struct {
	Level string `query:"level" enum:"debug,info,warn,error" doc:"Only this level and those above it."`
	Q     string `query:"q" maxLength:"200" doc:"Only lines holding this text, in any case."`
	Limit int    `query:"limit" minimum:"1" maximum:"1000" default:"200" doc:"The most entries to return."`
}

type logsOutput struct {
	Body Logs
}

type logLevelInput struct {
	Body LogLevel
}

type logLevelOutput struct {
	Body LogLevel
}

func (h handlers) registerLogs(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-logs",
		Method:      http.MethodGet,
		Path:        "/logs",
		Summary:     "Read the server's log",
		Description: "The newest entries of the active log file, filtered. An admin's live stream carries each line as it is written (log).",
		Tags:        []string{"System"},
		Security:    secured,
		Middlewares: huma.Middlewares{h.admin(api)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, h.listLogs)

	huma.Register(api, huma.Operation{
		OperationID: "update-log-level",
		Method:      http.MethodPut,
		Path:        "/logs/level",
		Summary:     "Set the level the server logs at",
		Description: "Takes effect at once and lasts until the server restarts, which goes back to info.",
		Tags:        []string{"System"},
		Security:    secured,
		Middlewares: huma.Middlewares{h.admin(api)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, func(_ context.Context, in *logLevelInput) (*logLevelOutput, error) {
		lvl, _ := logging.LevelValue(in.Body.Level)
		logging.SetLevel(lvl)
		slog.Info("log level changed", "level", logging.LevelName(lvl))
		return &logLevelOutput{Body: LogLevel{Level: logging.LevelName(lvl)}}, nil
	})
}

func (h handlers) listLogs(_ context.Context, in *logsInput) (*logsOutput, error) {
	out := Logs{Entries: []LogEntry{}, Level: logging.LevelName(logging.CurrentLevel())}
	if h.LogFile != nil {
		out.File = h.LogFile()
	}
	if out.File == "" {
		return &logsOutput{Body: out}, nil
	}
	entries, truncated, err := logging.Read(out.File, logging.Filter{Level: in.Level, Query: in.Q, Limit: in.Limit})
	if err != nil {
		return nil, h.fail("read the log", err)
	}
	out.Truncated = truncated
	for _, e := range entries {
		out.Entries = append(out.Entries, LogEntry{Time: e.Time, Level: e.Level, Message: e.Message, Fields: e.Fields})
	}
	return &logsOutput{Body: out}, nil
}
