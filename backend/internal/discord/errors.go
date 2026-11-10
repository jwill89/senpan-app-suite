package discord

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Kind classifies why a post to Discord failed, by what the caller can do about it.
type Kind int

const (
	// Ambiguous is a failure after the request may have reached Discord (a timeout,
	// a reset connection): the message may have been posted, so a caller must not
	// retry blindly.
	Ambiguous Kind = iota + 1
	// Unreachable is Discord answering with a 5xx, or the request never leaving
	// (DNS, a refused connection): nothing was posted, and trying again later can
	// work.
	Unreachable
	// RateLimited is Discord answering 429; Error.RetryAfter says when to try again.
	RateLimited
	// Refused is any other 4xx: a deleted webhook, a bad token, a payload Discord
	// rejects. Trying again as it is will not work.
	Refused
)

// Error is a failed post. Its message never carries the webhook URL, whose path is
// its secret token.
type Error struct {
	Kind Kind
	// Status is Discord's HTTP status, 0 when it never answered.
	Status int
	// RetryAfter is how long Discord asked to wait, for RateLimited.
	RetryAfter time.Duration
	// Message is Discord's own reason, or what went wrong reaching it.
	Message string
	// Err is the underlying cause, when there is one.
	Err error
}

func (e *Error) Error() string {
	switch {
	case e.Kind == Ambiguous:
		return "discord webhook delivery ambiguous (transport failure): " + e.Message
	case e.Status != 0:
		return fmt.Sprintf("webhook returned status %d: %s", e.Status, e.Message)
	default:
		return "webhook could not be reached: " + e.Message
	}
}

func (e *Error) Unwrap() error { return e.Err }

// IsAmbiguous reports whether err is a post that may or may not have gone out.
func IsAmbiguous(err error) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Kind == Ambiguous
}

// HTTPStatus is the HTTP status an API answers when err failed a post, and how long
// to wait when that is a 429. A rate limit is 429, a refusal (the webhook or its
// payload is wrong, which is this server's configuration) 500, and Discord not
// answering or not being reachable 503; anything else is 500.
func HTTPStatus(err error) (status int, retryAfter time.Duration) {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return http.StatusInternalServerError, 0
	}
	switch e.Kind {
	case RateLimited:
		return http.StatusTooManyRequests, e.RetryAfter
	case Unreachable, Ambiguous:
		return http.StatusServiceUnavailable, 0
	case Refused:
		return http.StatusInternalServerError, 0
	default:
		return http.StatusInternalServerError, 0
	}
}
