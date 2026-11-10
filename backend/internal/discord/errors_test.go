package discord

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// post sends one embed to a webhook that answers as respond says, and returns the
// error.
func post(t *testing.T, respond http.HandlerFunc) error {
	t.Helper()
	srv := httptest.NewServer(respond)
	defer srv.Close()
	return (&Sender{}).PostEmbed(context.Background(), Target{Kind: "test"}, srv.URL, Embed{Title: "Hi"})
}

func TestStatusIsClassified(t *testing.T) {
	cases := []struct {
		name       string
		respond    http.HandlerFunc
		kind       Kind
		status     int
		wantHTTP   int
		retryAfter time.Duration
		message    string
	}{
		{
			name: "unknown webhook is refused",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Unknown Webhook","code":10015}`))
			},
			kind: Refused, status: 404, wantHTTP: http.StatusInternalServerError, message: "Unknown Webhook (code 10015)",
		},
		{
			name: "a rate limit reads the header",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
			},
			kind: RateLimited, status: 429, wantHTTP: http.StatusTooManyRequests, retryAfter: 7 * time.Second, message: "no response body",
		},
		{
			name: "a rate limit falls back to the body",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message":"You are being rate limited.","retry_after":1.5}`))
			},
			kind: RateLimited, status: 429, wantHTTP: http.StatusTooManyRequests, retryAfter: 1500 * time.Millisecond, message: "You are being rate limited.",
		},
		{
			name: "a 5xx is unreachable",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			},
			kind: Unreachable, status: 502, wantHTTP: http.StatusServiceUnavailable, message: "no response body",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := post(t, tc.respond)
			e, ok := errors.AsType[*Error](err)
			if !ok {
				t.Fatalf("error %v is not a *discord.Error", err)
			}
			if e.Kind != tc.kind || e.Status != tc.status || e.RetryAfter != tc.retryAfter || e.Message != tc.message {
				t.Errorf("got kind=%v status=%d retryAfter=%v message=%q; want %v %d %v %q",
					e.Kind, e.Status, e.RetryAfter, e.Message, tc.kind, tc.status, tc.retryAfter, tc.message)
			}
			status, wait := HTTPStatus(err)
			if status != tc.wantHTTP || wait != tc.retryAfter {
				t.Errorf("HTTPStatus = %d, %v; want %d, %v", status, wait, tc.wantHTTP, tc.retryAfter)
			}
		})
	}
}

func TestATimeoutIsAmbiguous(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-release
	}))
	defer srv.Close()
	defer close(release)

	s := &Sender{HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	err := s.PostEmbed(context.Background(), Target{Kind: "test"}, srv.URL, Embed{Title: "Hi"})
	if !IsAmbiguous(err) {
		t.Fatalf("a timed-out post = %v; want an ambiguous error", err)
	}
	if status, _ := HTTPStatus(err); status != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d; want 503", status)
	}
}

func TestARefusedConnectionIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	gone := srv.URL
	srv.Close()

	err := (&Sender{}).PostEmbed(context.Background(), Target{Kind: "test"}, gone, Embed{Title: "Hi"})
	e, ok := errors.AsType[*Error](err)
	if !ok || e.Kind != Unreachable {
		t.Fatalf("a refused connection = %v; want an unreachable error", err)
	}
	if status, _ := HTTPStatus(err); status != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d; want 503", status)
	}
}

func TestHTTPStatusOfOtherErrors(t *testing.T) {
	if status, _ := HTTPStatus(errors.New("encode embed")); status != http.StatusInternalServerError {
		t.Errorf("HTTPStatus(plain error) = %d; want 500", status)
	}
}

func TestAnErrorNeverCarriesTheToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/api/webhooks/1/SECRETTOKEN"
	srv.Close()

	err := (&Sender{}).PostEmbed(context.Background(), Target{Kind: "test"}, url, Embed{Title: "Hi"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); strings.Contains(got, "SECRETTOKEN") || strings.Contains(got, srv.URL) {
		t.Errorf("error %q carries the webhook URL", got)
	}
}
