package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// sseFront builds a routed handler from yamlSrc and starts a real httptest
// server for it. A real server (not httptest.NewRecorder) is required to observe
// streaming/flush behavior, since the recorder buffers and is not an
// http.Flusher.
func sseFront(t *testing.T, yamlSrc string) *httptest.Server {
	t.Helper()
	srv, _ := sseFrontWithMetrics(t, yamlSrc)
	return srv
}

// sseFrontWithMetrics is sseFront with a fakeRecorder wired in so per-command
// mock counters (template renders/errors) can be asserted. It returns the live
// server and the recorder. As with sseFront a real server is required because
// SSE needs an http.Flusher.
func sseFrontWithMetrics(t *testing.T, yamlSrc string) (*httptest.Server, *fakeRecorder) {
	t.Helper()
	rec := &fakeRecorder{}
	store := newStore(t, yamlSrc, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Metrics: rec})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, rec
}

// readSSEEvent reads one SSE event (lines up to and including the blank-line
// terminator) from r, returning the raw block without the terminating blank
// line. It fails the test on a read error or timeout via the caller's deadline.
func readSSEEvent(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE event: %v", err)
		}
		if line == "\n" { // blank line terminates the event
			return b.String()
		}
		b.WriteString(line)
	}
}

// TestSSE_IncrementalDelivery proves scripted events arrive over time, not all
// at once after the handler returns. It mirrors the proxy incremental-flush
// test: each event carries a delay, and the client must observe event N before
// event N+1's delay has elapsed.
func TestSSE_IncrementalDelivery(t *testing.T) {
	const src = `
routes:
  - path: /events
    sse:
      - data: first
        delay: 50ms
      - data: second
        delay: 150ms
`
	srv := sseFront(t, src)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}

	br := bufio.NewReader(resp.Body)

	start := time.Now()
	got1 := readSSEEvent(t, br)
	elapsed1 := time.Since(start)
	if !strings.Contains(got1, "data: first") {
		t.Errorf("event 1 = %q, want a data: first line", got1)
	}
	// The first event waits ~50ms; it must arrive before the second event's
	// cumulative delay (~200ms), proving it is not buffered until the script
	// completes. The upper bound is deliberately generous (well under 200ms but
	// with ample slack over the 50ms target) so the test does not flake on a
	// loaded CI runner while still catching a fully-buffered response.
	if elapsed1 > 180*time.Millisecond {
		t.Errorf("event 1 took %v; appears buffered, not streamed", elapsed1)
	}

	got2 := readSSEEvent(t, br)
	elapsed2 := time.Since(start)
	if !strings.Contains(got2, "data: second") {
		t.Errorf("event 2 = %q, want a data: second line", got2)
	}
	// The second event waits an additional ~150ms after the first.
	if elapsed2 < 150*time.Millisecond {
		t.Errorf("event 2 arrived after %v; expected it to wait for its delay", elapsed2)
	}
}

// TestSSE_EventName verifies an event name renders as an `event:` line ahead of
// the data line(s).
func TestSSE_EventName(t *testing.T) {
	const src = `
routes:
  - path: /named
    sse:
      - event: ping
        data: pong
`
	srv := sseFront(t, src)
	resp, err := http.Get(srv.URL + "/named")
	if err != nil {
		t.Fatalf("GET /named: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	got := readSSEEvent(t, br)
	if !strings.Contains(got, "event: ping\n") {
		t.Errorf("event block = %q, want an event: ping line", got)
	}
	if !strings.Contains(got, "data: pong\n") {
		t.Errorf("event block = %q, want a data: pong line", got)
	}
}

// TestSSE_Repeat verifies repeat sends the event the requested number of times
// and repeat_delay spaces the repeats apart.
func TestSSE_Repeat(t *testing.T) {
	const src = `
routes:
  - path: /tick
    sse:
      - data: tick
        repeat: 3
        repeat_delay: 60ms
`
	srv := sseFront(t, src)
	resp, err := http.Get(srv.URL + "/tick")
	if err != nil {
		t.Fatalf("GET /tick: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)

	start := time.Now()
	for i := 0; i < 3; i++ {
		got := readSSEEvent(t, br)
		if !strings.Contains(got, "data: tick") {
			t.Fatalf("repeat %d = %q, want data: tick", i, got)
		}
	}
	// Two repeat_delays (~120ms) separate the three sends.
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("3 repeats took %v; expected repeat_delay spacing", elapsed)
	}

	// No further events: the next read should hit EOF (the stream ends).
	if _, err := br.ReadString('\n'); err == nil {
		t.Errorf("expected stream to end after 3 events, got more data")
	}
}

// TestSSE_TemplatedData proves a data: template renders against the shared
// request-data context (e.g. a path param).
func TestSSE_TemplatedData(t *testing.T) {
	const src = `
routes:
  - path: /items/:id
    sse:
      - data: 'item={{.params.id}}'
`
	srv := sseFront(t, src)
	resp, err := http.Get(srv.URL + "/items/42")
	if err != nil {
		t.Fatalf("GET /items/42: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	got := readSSEEvent(t, br)
	if !strings.Contains(got, "data: item=42\n") {
		t.Errorf("event block = %q, want data: item=42", got)
	}
}

// TestSSE_MultiLineData proves multi-line rendered data is split into one data:
// field per line on the wire.
func TestSSE_MultiLineData(t *testing.T) {
	const src = `
routes:
  - path: /multi
    sse:
      - data: "line one\nline two"
`
	srv := sseFront(t, src)
	resp, err := http.Get(srv.URL + "/multi")
	if err != nil {
		t.Fatalf("GET /multi: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	got := readSSEEvent(t, br)
	if !strings.Contains(got, "data: line one\n") {
		t.Errorf("event block = %q, want a data: line one line", got)
	}
	if !strings.Contains(got, "data: line two\n") {
		t.Errorf("event block = %q, want a data: line two line", got)
	}
}

// TestSSE_CarriageReturnNormalization proves request-controlled data with bare
// "\r" or "\r\n" terminators cannot forge SSE fields: every logical line, no
// matter which newline variant separated it, is re-prefixed with "data: ". A
// payload like "evil\revent: spoof" must NOT yield a wire "event: spoof" line.
func TestSSE_CarriageReturnNormalization(t *testing.T) {
	const src = `
routes:
  - path: /inject
    sse:
      - data: '{{.query.msg}}'
`
	srv := sseFront(t, src)
	// "a\revent: spoof" (CR) and "b\r\nid: 7" (CRLF) are both attacker-style
	// attempts to inject extra SSE fields via the data payload.
	resp, err := http.Get(srv.URL + "/inject?msg=" + url.QueryEscape("a\revent: spoof\r\nid: 7"))
	if err != nil {
		t.Fatalf("GET /inject: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	got := readSSEEvent(t, br)
	// All three logical lines must be data: lines; none may appear as a raw
	// event:/id: field.
	for _, want := range []string{"data: a\n", "data: event: spoof\n", "data: id: 7\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("event block = %q, want it to contain %q", got, want)
		}
	}
	// A forged field would appear at the start of a line (no "data: " prefix).
	for _, forbidden := range []string{"\nevent: spoof", "\nid: 7"} {
		if strings.Contains("\n"+got, forbidden+"\n") {
			t.Errorf("event block = %q, must not contain a forged field %q", got, forbidden)
		}
	}
}

// TestSSE_ClientCancel proves the handler returns promptly when the client
// cancels the request mid-stream (between scripted events). Without honoring the
// context, the handler would block on the long inter-event delay.
func TestSSE_ClientCancel(t *testing.T) {
	const src = `
routes:
  - path: /cancel
    sse:
      - data: first
      - data: second
        delay: 30s
`
	srv := sseFront(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/cancel", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /cancel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	// Read the first event, then cancel before the (30s) second event.
	if got := readSSEEvent(t, br); !strings.Contains(got, "data: first") {
		t.Fatalf("first event = %q, want data: first", got)
	}

	cancel()

	// The body should close promptly (well under the 30s scripted delay).
	done := make(chan struct{})
	go func() {
		_, _ = br.ReadString('\n') // returns once the canceled stream closes
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return promptly after client cancel")
	}
}

// TestSSE_ZeroDelayCancel proves a zero-delay, high-repeat stream stops promptly
// when the client disconnects, exercising the write-error short-circuit rather
// than the inter-event sleepCtx check (which never blocks here). Without bailing
// on the broken-pipe write error, the handler would keep rendering and writing
// the entire (huge) script after the peer is gone.
func TestSSE_ZeroDelayCancel(t *testing.T) {
	const src = `
routes:
  - path: /flood
    sse:
      - data: tick
        repeat: 1000000
`
	srv := sseFront(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/flood", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /flood: %v", err)
	}

	br := bufio.NewReader(resp.Body)
	// Observe the first event, then cancel mid-stream.
	if got := readSSEEvent(t, br); !strings.Contains(got, "data: tick") {
		t.Fatalf("first event = %q, want data: tick", got)
	}
	cancel()
	_ = resp.Body.Close()

	// The handler must return promptly after the disconnect (via the write-error
	// short-circuit), not after streaming all 1,000,000 zero-delay repeats. A
	// route with no isSSE-side cancellation handling would only ever stop at a
	// sleepCtx check, which never fires for a zero-delay stream. We assert the
	// handler goroutine has settled by the deadline; the test harness's race
	// detector and the bounded wait catch a runaway handler.
	deadline := time.After(5 * time.Second)
	done := make(chan struct{})
	go func() {
		// Drain until EOF/closed; this returns once the canceled stream tears down.
		for {
			if _, rerr := br.ReadString('\n'); rerr != nil {
				break
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("handler did not stop promptly after client disconnect on a zero-delay stream")
	}
}

// TestSSE_EndOfScriptCloses proves the stream ends cleanly once the script is
// exhausted (the response body reaches EOF).
func TestSSE_EndOfScriptCloses(t *testing.T) {
	const src = `
routes:
  - path: /done
    sse:
      - data: only
`
	srv := sseFront(t, src)
	resp, err := http.Get(srv.URL + "/done")
	if err != nil {
		t.Fatalf("GET /done: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	if got := readSSEEvent(t, br); !strings.Contains(got, "data: only") {
		t.Fatalf("event = %q, want data: only", got)
	}
	if _, err := br.ReadString('\n'); err == nil {
		t.Errorf("expected EOF after the single scripted event")
	}
}

// nonFlushWriter is an http.ResponseWriter that deliberately does NOT implement
// http.Flusher, used to exercise the SSE handler's flusher-required guard.
// (httptest.ResponseRecorder cannot be used here: it implements http.Flusher.)
type nonFlushWriter struct {
	header http.Header
	status int
}

func (w *nonFlushWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *nonFlushWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *nonFlushWriter) WriteHeader(status int)      { w.status = status }

// TestSSE_RequiresFlusher proves an SSE route returns 500 when the response
// writer cannot stream (does not implement http.Flusher).
func TestSSE_RequiresFlusher(t *testing.T) {
	const src = `
routes:
  - path: /events
    sse:
      - data: hello
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store})
	w := &nonFlushWriter{}
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/events", nil))
	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for a non-flushable writer", w.status)
	}
}

// TestSSE_CompileErrors verifies malformed SSE config fails at compile time with
// a clear error.
func TestSSE_CompileErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "negative repeat",
			src: `
routes:
  - path: /x
    sse:
      - data: a
        repeat: -1
`,
			want: "repeat must not be negative",
		},
		{
			name: "negative delay",
			src: `
routes:
  - path: /x
    sse:
      - data: a
        delay: -1s
`,
			want: "delay and repeat_delay must not be negative",
		},
		{
			name: "negative repeat_delay",
			src: `
routes:
  - path: /x
    sse:
      - data: a
        repeat_delay: -2s
`,
			want: "delay and repeat_delay must not be negative",
		},
		{
			name: "bad data template",
			src: `
routes:
  - path: /x
    sse:
      - data: '{{ .params.id'
`,
			want: "parse template",
		},
		{
			name: "event name with newline",
			src:  "routes:\n  - path: /x\n    sse:\n      - event: \"a\\nb\"\n        data: c\n",
			want: "event name must not contain newlines",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileRoutes([]byte(tt.src), t.TempDir())
			if err == nil {
				t.Fatalf("expected a compile error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestSSE_TemplateRenderMetricsWiring proves an SSE route counts one
// template_renders per event instance actually streamed (so a repeat counts
// once per send), and that an event with no data template is not counted as a
// render. The route below streams three event instances backed by a data
// template (one single event plus two repeats of another) and one event with no
// data template, so the expected render count is 3.
func TestSSE_TemplateRenderMetricsWiring(t *testing.T) {
	const src = `
routes:
  - path: /events
    sse:
      - data: 'item={{.params.id}}'
      - data: 'tick {{seq}}'
        repeat: 2
      - event: ping
`
	srv, rec := sseFrontWithMetrics(t, src)
	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain the whole stream so every scripted event is sent (and counted) before
	// asserting. The script has fixed length and ends in EOF.
	br := bufio.NewReader(resp.Body)
	events := 0
	for {
		if _, rerr := br.ReadString('\n'); rerr != nil {
			break
		}
		// Count event terminators (blank lines) to confirm the stream completed.
		events++
	}

	// One render for the first event, two for the repeated event = 3. The
	// data-less third event must not be counted as a render.
	if got := rec.mockRenders.Load(); got != 3 {
		t.Errorf("mockRenders = %d, want 3 (one per streamed event with a data template, repeats included)", got)
	}
	if got := rec.mockErrors.Load(); got != 0 {
		t.Errorf("mockErrors = %d, want 0", got)
	}
}

// TestSSE_TemplateErrorMetricsWiring proves a data template that errors at
// render time increments template_errors. {{randomChoice}} (no args) errors when
// executed, mirroring the buffered-route error test. Because the SSE headers are
// already sent, the failure is surfaced as an SSE comment and the stream stops,
// so no render is counted for the failing event.
func TestSSE_TemplateErrorMetricsWiring(t *testing.T) {
	const src = `
routes:
  - path: /boom
    sse:
      - data: '{{randomChoice}}'
`
	srv, rec := sseFrontWithMetrics(t, src)
	resp, err := http.Get(srv.URL + "/boom")
	if err != nil {
		t.Fatalf("GET /boom: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain the stream; the handler writes an ": error rendering event" comment
	// and closes once the data template fails.
	br := bufio.NewReader(resp.Body)
	var body strings.Builder
	for {
		line, rerr := br.ReadString('\n')
		body.WriteString(line)
		if rerr != nil {
			break
		}
	}

	if got := rec.mockErrors.Load(); got != 1 {
		t.Errorf("mockErrors = %d, want 1", got)
	}
	if got := rec.mockRenders.Load(); got != 0 {
		t.Errorf("mockRenders = %d, want 0 on render error", got)
	}
	if !strings.Contains(body.String(), ": error rendering event") {
		t.Errorf("stream = %q, want it to contain an SSE error comment", body.String())
	}
}
