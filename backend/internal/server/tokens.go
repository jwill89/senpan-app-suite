package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/model"
)

// A personal access token (internal/auth, made at /api/v1/me/token) lets a client
// that isn't a browser - the FFXIV plugin - act as its account without the
// browser's sign-in. It is sent as `Authorization: Bearer <token>`, the live stream
// included. It resolves to its account and the same guards a session meets then
// apply, so a token never grants more than the account has.

// bearerToken extracts a personal access token from the `Authorization: Bearer
// <token>` header only. Returns "" when absent or not a PAT-shaped value.
//
// A token in the URL is never accepted: it would leak into access logs, browser
// history and Referer headers.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if rest, ok := strings.CutPrefix(h, "Bearer "); ok {
			if tok := strings.TrimSpace(rest); strings.HasPrefix(tok, auth.TokenScheme) {
				return tok
			}
		}
	}
	return ""
}

// userFromToken resolves the active account bearing the request's Authorization
// Bearer PAT, or nil when absent/unmatched/inactive.
func (s *Server) userFromToken(r *http.Request) *model.User {
	return s.userForToken(r.Context(), bearerToken(r))
}

// userForToken resolves the active account for a raw PAT string, or nil when the
// token is empty, matches nothing, or the owning account is inactive. On a hit it
// best-effort stamps the token's last-used time (at most once a minute) so the
// account page can show when the token was last exercised.
func (s *Server) userForToken(ctx context.Context, tok string) *model.User {
	if tok == "" {
		return nil
	}
	hash := auth.HashToken(tok)
	row, err := db.New(s.store.ReadDB()).GetUserByToken(ctx, hash)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("read the account behind a token", "error", err)
		}
		return nil
	}
	u, err := auth.Row(*row).User()
	if err != nil {
		slog.Error("read the account behind a token", "error", err)
		return nil
	}
	if !u.IsActive {
		return nil
	}
	_ = db.New(s.store.DB()).TouchToken(ctx, hash) // best-effort: never blocks a valid request
	return u
}

// account is the account with id, or nil when there is none or it can't be read.
func (s *Server) account(ctx context.Context, id int64) *model.User {
	row, err := db.New(s.store.ReadDB()).GetUser(ctx, id)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("read the account behind a request", "error", err, "user_id", id)
		}
		return nil
	}
	u, err := auth.Row(*row).User()
	if err != nil {
		slog.Error("read the account behind a request", "error", err, "user_id", id)
		return nil
	}
	return u
}
