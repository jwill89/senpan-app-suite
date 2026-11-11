// Package apitest holds the API's answers to its OpenAPI document in tests: every
// 2xx answer from an operation must be a response the document declares for it, with
// a body its schema accepts. An event stream is checked event by event as it is read:
// each event must be one the document lists for the stream, with data its schema
// accepts.
//
// Transport does it for a whole test client, so every test that calls an operation
// checks the answer it gets without saying so.
package apitest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/api"
)

// document is the API's as the clients have it (api.Document), built once for the
// test binary without a database.
var document = sync.OnceValue(api.Document)

const eventStream = "text/event-stream"

// Transport wraps next (nil for the default) so that every 2xx answer from an
// operation under api.Prefix is checked against the document; a mismatch fails t.
// A path that is not an operation of the document is left alone. An event stream's
// body is checked as the test reads it, since it doesn't end.
func Transport(t testing.TB, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return roundTrip(func(req *http.Request) (*http.Response, error) {
		resp, err := next.RoundTrip(req)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
			return resp, err
		}
		if strings.HasPrefix(resp.Header.Get("Content-Type"), eventStream) {
			if msg := StreamProblem(req.Method, req.URL.Path, resp.StatusCode); msg != "" {
				t.Errorf("%s %s answered %d against the API document: %s", req.Method, req.URL.Path, resp.StatusCode, msg)
			}
			resp.Body = &eventChecker{t: t, req: req, status: resp.StatusCode, body: resp.Body}
			return resp, nil
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if msg := Problem(req.Method, req.URL.Path, resp.StatusCode, resp.Header.Get("Content-Type"), body); msg != "" {
			t.Errorf("%s %s answered %d against the API document: %s", req.Method, req.URL.Path, resp.StatusCode, msg)
		}
		return resp, nil
	})
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// Problem is what is wrong with an answer against the document, or "" when nothing
// is (or the path is not an operation of the document).
func Problem(method, path string, status int, contentType string, body []byte) string {
	doc := document()
	op := operation(doc, method, path)
	if op == nil {
		return ""
	}
	resp := op.Responses[strconv.Itoa(status)]
	if resp == nil {
		return fmt.Sprintf("the document declares no %d for %s", status, op.OperationID)
	}
	// A body that isn't JSON (a stylesheet, a font) is right when the document declares
	// its type for the status.
	if kind, _, err := mime.ParseMediaType(contentType); err == nil && kind != "application/json" && resp.Content[kind] != nil {
		return ""
	}
	media := resp.Content["application/json"]
	if media == nil || media.Schema == nil {
		if len(body) > 0 && status != http.StatusNoContent {
			return "the document declares no JSON body, but one came"
		}
		return ""
	}
	if !strings.HasPrefix(contentType, "application/json") {
		return fmt.Sprintf("content type %q, where the document declares application/json", contentType)
	}
	return validate(doc, media.Schema, body)
}

// StreamProblem is what is wrong with an event stream answering a request, before
// any event: "" when the document declares one for the operation and status, or the
// path is not an operation of the document.
func StreamProblem(method, path string, status int) string {
	op := operation(document(), method, path)
	if op == nil {
		return ""
	}
	if resp := op.Responses[strconv.Itoa(status)]; resp == nil || resp.Content[eventStream] == nil {
		return fmt.Sprintf("the document declares no event stream answering %d for %s", status, op.OperationID)
	}
	return ""
}

// EventProblem is what is wrong with one event of a stream against the document: its
// name must be one the stream lists (an unnamed event is "message"), and its data
// JSON the schema for that name accepts. "" when nothing is, or the path is not an
// operation of the document.
func EventProblem(method, path string, status int, event string, data []byte) string {
	doc := document()
	op := operation(doc, method, path)
	if op == nil {
		return ""
	}
	resp := op.Responses[strconv.Itoa(status)]
	if resp == nil || resp.Content[eventStream] == nil || resp.Content[eventStream].Schema == nil {
		return fmt.Sprintf("the document declares no event stream answering %d for %s", status, op.OperationID)
	}
	if event == "" {
		event = "message"
	}
	items := resp.Content[eventStream].Schema.Items
	if items == nil {
		return "the stream's schema lists no events"
	}
	events, _ := items.Extensions["oneOf"].([]*huma.Schema)
	for _, e := range events {
		name := e.Properties["event"]
		if name == nil || name.Extensions["const"] != event {
			continue
		}
		return validate(doc, e.Properties["data"], data)
	}
	return fmt.Sprintf("the document lists no %q event for %s", event, op.OperationID)
}

// validate checks a JSON body against a schema of the document.
func validate(doc *huma.OpenAPI, schema *huma.Schema, body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return "the body isn't JSON: " + err.Error()
	}
	res := &huma.ValidateResult{}
	huma.Validate(doc.Components.Schemas, schema, huma.NewPathBuffer([]byte{}, 0), huma.ModeReadFromServer, value, res)
	if len(res.Errors) == 0 {
		return ""
	}
	msgs := make([]string, 0, len(res.Errors))
	for _, e := range res.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// eventChecker passes an event stream's body through as it is read, checking each
// event it completes against the document.
type eventChecker struct {
	t      testing.TB
	req    *http.Request
	status int
	body   io.ReadCloser

	partial []byte
	event   string
	data    []string
}

func (c *eventChecker) Read(p []byte) (int, error) {
	n, err := c.body.Read(p)
	c.partial = append(c.partial, p[:n]...)
	for {
		i := bytes.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}
		c.line(strings.TrimSuffix(string(c.partial[:i]), "\r"))
		c.partial = c.partial[i+1:]
	}
	return n, err
}

// line takes one line of the stream: a field, a comment, or the blank line that ends
// an event.
func (c *eventChecker) line(l string) {
	switch {
	case l == "":
		if len(c.data) > 0 {
			data := []byte(strings.Join(c.data, "\n"))
			if msg := EventProblem(c.req.Method, c.req.URL.Path, c.status, c.event, data); msg != "" {
				c.t.Errorf("%s %s sent an event against the API document: %s: %s", c.req.Method, c.req.URL.Path, msg, data)
			}
		}
		c.event, c.data = "", nil
	case strings.HasPrefix(l, ":"):
	default:
		field, value, _ := strings.Cut(l, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			c.event = value
		case "data":
			c.data = append(c.data, value)
		}
	}
}

func (c *eventChecker) Close() error { return c.body.Close() }

// Events reads an event stream's events from r as they come, for a test to follow:
// each is sent on the channel, which closes when the stream ends. The channel holds
// plenty, so a test that stops reading early doesn't hold the reader up.
func Events(r io.Reader) <-chan Event {
	out := make(chan Event, 1024)
	go func() {
		defer close(out)
		scan := bufio.NewScanner(r)
		scan.Buffer(make([]byte, 0, 64*1024), 1<<20)
		var e Event
		var data []string
		for scan.Scan() {
			l := scan.Text()
			if l != "" {
				field, value, _ := strings.Cut(l, ":")
				value = strings.TrimPrefix(value, " ")
				switch field {
				case "id":
					e.ID = value
				case "event":
					e.Name = value
				case "data":
					data = append(data, value)
				}
				continue
			}
			if len(data) > 0 {
				e.Data = []byte(strings.Join(data, "\n"))
				out <- e
			}
			e, data = Event{}, nil
		}
	}()
	return out
}

// Event is one event of a stream as Events reads it.
type Event struct {
	// ID is the SSE id, "" when the event carries none.
	ID string
	// Name is the SSE event name.
	Name string
	// Data is the event's data.
	Data []byte
}

// operation finds the document's operation for a request path, matching each
// {parameter} segment to any value, and preferring the template with the most fixed
// segments, as the mux does (/raffles/public over /raffles/{id}). A trailing slash
// answers as the path without it.
func operation(doc *huma.OpenAPI, method, path string) *huma.Operation {
	rest, ok := strings.CutPrefix(path, api.Prefix)
	if !ok {
		return nil
	}
	want := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	var best *huma.Operation
	bestFixed := -1
	for template, item := range doc.Paths {
		have := strings.Split(template, "/")
		if len(have) != len(want) {
			continue
		}
		fixed, match := 0, true
		for i := range have {
			switch {
			case have[i] == want[i]:
				fixed++
			case strings.HasPrefix(have[i], "{") && strings.HasSuffix(have[i], "}"):
			default:
				match = false
			}
		}
		if op := byMethod(item, method); match && op != nil && fixed > bestFixed {
			best, bestFixed = op, fixed
		}
	}
	return best
}

func byMethod(item *huma.PathItem, method string) *huma.Operation {
	switch method {
	case http.MethodGet, http.MethodHead:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPut:
		return item.Put
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
	}
	return nil
}
