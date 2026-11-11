package apitest

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestProblemCatchesAnswersTheDocumentDoesNotDeclare checks that the checker passes
// an answer the document declares and catches one it doesn't.
func TestProblemCatchesAnswersTheDocumentDoesNotDeclare(t *testing.T) {
	const json = "application/json"
	for _, c := range []struct {
		name, method, path string
		status             int
		body               string
		bad                bool
	}{
		{"the declared answer", http.MethodGet, "/api/v1/version", 200, `{"version":"5.0.0"}`, false},
		{"a trailing slash", http.MethodGet, "/api/v1/version/", 200, `{"version":"5.0.0"}`, false},
		{"a wrong type", http.MethodGet, "/api/v1/version", 200, `{"version":5}`, true},
		{"a missing field", http.MethodGet, "/api/v1/version", 200, `{}`, true},
		{"a field the document lacks", http.MethodGet, "/api/v1/version", 200, `{"version":"5.0.0","extra":1}`, true},
		{"an undeclared status", http.MethodGet, "/api/v1/version", 201, `{"version":"5.0.0"}`, true},
		{"a route not on huma", http.MethodGet, "/api/cards", 200, `[]`, false},
	} {
		got := Problem(c.method, c.path, c.status, json, []byte(c.body))
		if (got != "") != c.bad {
			t.Errorf("%s: Problem = %q, want a problem: %v", c.name, got, c.bad)
		}
	}
}

// TestEventProblem checks that each event of a stream is held to the schema the
// document gives its name.
func TestEventProblem(t *testing.T) {
	const at = `"at":"2026-11-09T20:00:00Z"`
	for _, c := range []struct {
		name, path, event, data string
		bad                     bool
	}{
		{"a staff draw", "/api/v1/events", "draw",
			`{"id":5,"type":"draw","gameId":3,` + at + `,"payload":{"drawn":{"number":7,"letter":"B","callOrder":1},"winners":[]}}`, false},
		{"a board's draw", "/api/v1/board/ABC123/events", "draw",
			`{"id":5,"type":"draw","gameId":3,` + at + `,"payload":{"drawn":{"number":7,"letter":"B","callOrder":1}}}`, false},
		{"the staff's draw on a board", "/api/v1/board/ABC123/events", "draw",
			`{"id":5,"type":"draw","gameId":3,` + at + `,"payload":{"drawn":{"number":7,"letter":"B","callOrder":1},"winners":[]}}`, true},
		{"a ping", "/api/v1/events", "ping", `{"id":0,"type":"ping","gameId":null,` + at + `,"payload":{}}`, false},
		{"a null payload", "/api/v1/events", "ping", `{"id":0,"type":"ping","gameId":null,` + at + `,"payload":null}`, true},
		{"a type not its name", "/api/v1/events", "ping", `{"id":0,"type":"draw","gameId":null,` + at + `,"payload":{}}`, true},
		{"an event the stream lacks", "/api/v1/board/ABC123/events", "log", `{}`, true},
		{"an unnamed event", "/api/v1/events", "", `{}`, true},
		{"a route not on huma", "/api/raffles", "x", `{}`, false},
	} {
		got := EventProblem(http.MethodGet, c.path, 200, c.event, []byte(c.data))
		if (got != "") != c.bad {
			t.Errorf("%s: EventProblem = %q, want a problem: %v", c.name, got, c.bad)
		}
	}
	if StreamProblem(http.MethodGet, "/api/v1/events", 200) != "" || StreamProblem(http.MethodGet, "/api/v1/version", 200) == "" {
		t.Error("StreamProblem: the staff stream is declared, the version isn't a stream")
	}
}

// TestEventCheckerReadsEvents checks that the checker reads a stream through,
// failing a test on an event the document doesn't have.
func TestEventCheckerReadsEvents(t *testing.T) {
	stream := "id: 9\nevent: ping\ndata: {\"id\":0,\"type\":\"ping\",\"gameId\":null,\"at\":\"2026-11-09T20:00:00Z\",\"payload\":{}}\n\n" +
		": a comment\n\n"
	req, _ := http.NewRequest(http.MethodGet, "http://x/api/v1/events", nil)
	rec := &recordingTB{TB: t}
	c := &eventChecker{t: rec, req: req, status: 200, body: io.NopCloser(strings.NewReader(stream))}
	var got []Event
	for e := range Events(c) {
		got = append(got, e)
	}
	if len(got) != 1 || got[0].ID != "9" || got[0].Name != "ping" || rec.failed {
		t.Errorf("events %+v, failed %v", got, rec.failed)
	}

	bad := &eventChecker{t: rec, req: req, status: 200, body: io.NopCloser(strings.NewReader("event: nope\ndata: {}\n\n"))}
	for range Events(bad) {
	}
	if !rec.failed {
		t.Error("an event the document lacks passed")
	}
}

type recordingTB struct {
	testing.TB
	failed bool
}

func (r *recordingTB) Errorf(string, ...any) { r.failed = true }
