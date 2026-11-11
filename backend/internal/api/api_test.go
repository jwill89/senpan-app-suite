package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"app-suite/internal/model"
)

type okOutput struct {
	Body struct {
		OK bool `json:"ok" doc:"Always true."`
	}
}

// guarded registers one operation behind each guard.
func guarded(t *testing.T) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t, config())
	h := handlers{Deps{Logger: slog.Default()}}
	ok := func(context.Context, *struct{}) (*okOutput, error) {
		out := &okOutput{}
		out.Body.OK = true
		return out, nil
	}
	for path, guard := range map[string]func(huma.Context, func(huma.Context)){
		"/signed-in": h.signedIn(api),
		"/raffles":   h.permitted(api, "teahouse-raffles", "festival-garapon"),
		"/admin":     h.admin(api),
	} {
		huma.Register(api, huma.Operation{
			OperationID: "get" + strings.ReplaceAll(path, "/", "-"),
			Method:      http.MethodGet,
			Path:        path,
			Security:    secured,
			Middlewares: huma.Middlewares{guard},
		}, ok)
	}
	return api
}

// TestGuards checks that each guard answers 401 when nobody is signed in, 403 when
// the account lacks the access, and lets the rest through; an admin holds every
// permission.
func TestGuards(t *testing.T) {
	member := &model.User{Username: "member", Permissions: []string{"festival-garapon"}}
	stranger := &model.User{Username: "stranger", Permissions: []string{"bingo-cards"}}
	admin := &model.User{Username: "admin", IsAdmin: true}
	for _, c := range []struct {
		who  *model.User
		path string
		want int
	}{
		{nil, "/signed-in", http.StatusUnauthorized},
		{stranger, "/signed-in", http.StatusOK},
		{nil, "/raffles", http.StatusUnauthorized},
		{stranger, "/raffles", http.StatusForbidden},
		{member, "/raffles", http.StatusOK},
		{admin, "/raffles", http.StatusOK},
		{nil, "/admin", http.StatusUnauthorized},
		{member, "/admin", http.StatusForbidden},
		{admin, "/admin", http.StatusOK},
	} {
		name := "nobody"
		if c.who != nil {
			name = c.who.Username
		}
		who := c.who
		resp := guarded(t).GetCtx(WithWho(context.Background(), func() *model.User { return who }), c.path)
		if resp.Code != c.want {
			t.Errorf("%s GET %s: %d, want %d", name, c.path, resp.Code, c.want)
		}
		if c.want != http.StatusOK && !strings.HasPrefix(resp.Header().Get("Content-Type"), "application/problem+json") {
			t.Errorf("%s GET %s: refused with %q, want problem details", name, c.path, resp.Header().Get("Content-Type"))
		}
	}
}

// TestFailHidesTheError checks that fail logs the real error and answers a 500 that
// names nothing of it.
func TestFailHidesTheError(t *testing.T) {
	var logged bytes.Buffer
	h := handlers{Deps{Logger: slog.New(slog.NewTextHandler(&logged, nil))}}
	err := h.fail("load raffles", errors.New("database is locked: secret detail"))
	var status huma.StatusError
	if !errors.As(err, &status) || status.GetStatus() != http.StatusInternalServerError {
		t.Fatalf("fail = %v, want a 500", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("the 500 says %q, which names the real error", err.Error())
	}
	if !strings.Contains(logged.String(), "secret detail") || !strings.Contains(logged.String(), "load raffles") {
		t.Errorf("the log has %q, want the real error and where", logged.String())
	}
}
