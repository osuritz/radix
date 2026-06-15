package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestRoutes_SequenceCyclesStickOnLast proves a sequence (without repeat)
// advances through its items request-by-request and then sticks on the last
// item for every subsequent request.
func TestRoutes_SequenceCyclesStickOnLast(t *testing.T) {
	const src = `
routes:
  - path: /api/steps
    method: GET
    sequence:
      - body: "one"
      - body: "two"
      - body: "three"
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	get := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/steps", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		return rec.Body.String()
	}

	want := []string{"one", "two", "three", "three", "three"}
	for i, w := range want {
		if got := get(); got != w {
			t.Errorf("request %d body = %q, want %q (advance then stick on last)", i+1, got, w)
		}
	}
}

// TestRoutes_SequenceRepeatLoops proves a sequence with repeat:true loops back
// to the first item after the last.
func TestRoutes_SequenceRepeatLoops(t *testing.T) {
	const src = `
routes:
  - path: /api/loop
    method: GET
    repeat: true
    sequence:
      - body: "a"
      - body: "b"
      - body: "c"
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	get := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loop", nil))
		return rec.Body.String()
	}

	want := []string{"a", "b", "c", "a", "b", "c", "a"}
	for i, w := range want {
		if got := get(); got != w {
			t.Errorf("request %d body = %q, want %q (repeat loops back to first)", i+1, got, w)
		}
	}
}

// TestRoutes_SequenceItemStatusHeaders proves each sequence item carries its own
// status and headers.
func TestRoutes_SequenceItemStatusHeaders(t *testing.T) {
	const src = `
routes:
  - path: /api/codes
    method: GET
    sequence:
      - status: 201
        headers: { X-Step: one }
        body: "created"
      - status: 409
        headers: { X-Step: two }
        body: "conflict"
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/codes", nil))
		return rec
	}

	r1 := do()
	if r1.Code != http.StatusCreated || r1.Body.String() != "created" || r1.Header().Get("X-Step") != "one" {
		t.Errorf("item1: status=%d body=%q X-Step=%q, want 201 created one", r1.Code, r1.Body.String(), r1.Header().Get("X-Step"))
	}
	r2 := do()
	if r2.Code != http.StatusConflict || r2.Body.String() != "conflict" || r2.Header().Get("X-Step") != "two" {
		t.Errorf("item2: status=%d body=%q X-Step=%q, want 409 conflict two", r2.Code, r2.Body.String(), r2.Header().Get("X-Step"))
	}
}

// TestRoutes_SequenceSeqIndependentOfAdvance proves the {{seq}} template counter
// and the sequence selection index are independent: {{seq}} increments once per
// render (and twice within one body if referenced twice), while the sequence
// index advances exactly once per request.
func TestRoutes_SequenceSeqIndependentOfAdvance(t *testing.T) {
	// Each item renders {{seq}} TWICE in its body. If sequence advancement
	// reused the {{seq}} counter, the two-per-render consumption would skew which
	// item is selected. The selection must still be purely request-ordinal.
	const src = `
routes:
  - path: /api/mix
    method: GET
    sequence:
      - body: 'one a={{seq}} b={{seq}}'
      - body: 'two a={{seq}} b={{seq}}'
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	get := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/mix", nil))
		return rec.Body.String()
	}

	// Request 1 -> item "one"; {{seq}} renders 1 then 2 (per-render advance).
	if got := get(); got != "one a=1 b=2" {
		t.Errorf("request 1 body = %q, want %q", got, "one a=1 b=2")
	}
	// Request 2 -> item "two" (index advanced once, not by the two seq calls);
	// {{seq}} continues at 3 then 4.
	if got := get(); got != "two a=3 b=4" {
		t.Errorf("request 2 body = %q, want %q", got, "two a=3 b=4")
	}
	// Request 3 -> sticks on last item "two"; {{seq}} continues at 5 then 6.
	if got := get(); got != "two a=5 b=6" {
		t.Errorf("request 3 body = %q, want %q", got, "two a=5 b=6")
	}
}

// TestRoutes_RandomWeightedDistribution proves weighted-random selection honors
// the configured weights over a large number of requests, within a generous
// tolerance (the package RNG is not seedable here, so proportions are asserted,
// not exact counts).
func TestRoutes_RandomWeightedDistribution(t *testing.T) {
	const src = `
routes:
  - path: /api/dice
    method: GET
    random:
      - weight: 70
        response: { status: 200, body: "A" }
      - weight: 20
        response: { status: 200, body: "B" }
      - weight: 10
        response: { status: 200, body: "C" }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	const n = 20000
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dice", nil))
		counts[rec.Body.String()]++
	}

	// Expected proportions: A=0.70, B=0.20, C=0.10. Allow ±5 percentage points.
	want := map[string]float64{"A": 0.70, "B": 0.20, "C": 0.10}
	for arm, frac := range want {
		got := float64(counts[arm]) / float64(n)
		if got < frac-0.05 || got > frac+0.05 {
			t.Errorf("arm %q proportion = %.3f, want ~%.2f (±0.05); counts=%v", arm, got, frac, counts)
		}
	}
}

// TestRoutes_RandomSingleArm proves a single-arm random selection always serves
// that arm.
func TestRoutes_RandomSingleArm(t *testing.T) {
	const src = `
routes:
  - path: /api/only
    method: GET
    random:
      - weight: 1
        response: { status: 202, body: "solo" }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/only", nil))
		if rec.Code != http.StatusAccepted || rec.Body.String() != "solo" {
			t.Fatalf("request %d: status=%d body=%q, want 202 solo", i+1, rec.Code, rec.Body.String())
		}
	}
}

// TestRoutes_RandomDominantWeight proves an arm with an overwhelming weight
// relative to a tiny-weight arm is selected almost always — and the tiny arm is
// still reachable in principle (we only assert the dominant arm dominates).
func TestRoutes_RandomDominantWeight(t *testing.T) {
	const src = `
routes:
  - path: /api/skew
    method: GET
    random:
      - weight: 999
        response: { status: 200, body: "big" }
      - weight: 1
        response: { status: 200, body: "tiny" }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	const n = 5000
	big := 0
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skew", nil))
		if rec.Body.String() == "big" {
			big++
		}
	}
	if frac := float64(big) / float64(n); frac < 0.95 {
		t.Errorf("dominant arm proportion = %.3f, want >= 0.95", frac)
	}
}

// TestRoutes_SequenceConcurrent hammers a repeating sequence route from many
// goroutines and asserts (under -race) there are no data races and that every
// request advanced the counter exactly once: the multiset of returned indices is
// balanced across the cycle.
func TestRoutes_SequenceConcurrent(t *testing.T) {
	const src = `
routes:
  - path: /c
    method: GET
    repeat: true
    sequence:
      - body: "0"
      - body: "1"
      - body: "2"
      - body: "3"
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	const goroutines, perG = 50, 200
	const total = goroutines * perG // 10000, a multiple of the 4-item cycle
	results := make(chan string, total)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/c", nil))
				results <- rec.Body.String()
			}
		}()
	}
	wg.Wait()
	close(results)

	counts := map[string]int{}
	for r := range results {
		counts[r]++
	}
	// Every request advanced the atomic counter exactly once, so across a whole
	// number of cycles each of the 4 bodies must appear exactly total/4 times.
	wantEach := total / 4
	for _, body := range []string{"0", "1", "2", "3"} {
		if counts[body] != wantEach {
			t.Errorf("body %q count = %d, want %d (balanced cycle under concurrency)", body, counts[body], wantEach)
		}
	}
}

// TestRoutes_RandomConcurrent hammers a random route from many goroutines and
// asserts (under -race) no panics/races and that the distribution still holds.
func TestRoutes_RandomConcurrent(t *testing.T) {
	const src = `
routes:
  - path: /r
    method: GET
    random:
      - weight: 50
        response: { status: 200, body: "x" }
      - weight: 50
        response: { status: 200, body: "y" }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	const goroutines, perG = 50, 200
	const total = goroutines * perG
	results := make(chan string, total)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r", nil))
				results <- rec.Body.String()
			}
		}()
	}
	wg.Wait()
	close(results)

	counts := map[string]int{}
	for r := range results {
		counts[r]++
	}
	for _, body := range []string{"x", "y"} {
		frac := float64(counts[body]) / float64(total)
		if frac < 0.40 || frac > 0.60 {
			t.Errorf("body %q proportion = %.3f, want ~0.50 (±0.10); counts=%v", body, frac, counts)
		}
	}
}

// TestRoutes_SequenceAlongsidePlainRoute proves a sequence route coexists with
// an ordinary route in the same file without affecting it (backward-compat).
func TestRoutes_SequenceAlongsidePlainRoute(t *testing.T) {
	const src = `
routes:
  - path: /plain
    method: GET
    response: { status: 200, body: "static" }
  - path: /seq
    method: GET
    sequence:
      - body: "first"
      - body: "second"
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	get := func(p string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec.Body.String()
	}

	// The plain route is unaffected no matter how many times it is hit.
	for i := 0; i < 3; i++ {
		if got := get("/plain"); got != "static" {
			t.Errorf("plain request %d = %q, want static", i+1, got)
		}
	}
	// The sequence route advances independently.
	if got := get("/seq"); got != "first" {
		t.Errorf("seq request 1 = %q, want first", got)
	}
	if got := get("/seq"); got != "second" {
		t.Errorf("seq request 2 = %q, want second", got)
	}
}

// TestRoutes_SequencePerRouteResetOnReload proves each route owns its own
// sequence counter (and that compiling a fresh CompiledRoutes restarts it),
// mirroring how a hot-reload resets the per-route counters.
func TestRoutes_SequencePerRouteResetOnReload(t *testing.T) {
	const src = `
routes:
  - path: /s
    method: GET
    sequence:
      - body: "1"
      - body: "2"
      - body: "3"
`
	build := func() http.Handler {
		store := newStore(t, src, t.TempDir())
		return NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	}
	get := func(h http.Handler) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/s", nil))
		return rec.Body.String()
	}

	h1 := build()
	if got := get(h1); got != "1" {
		t.Fatalf("h1 request 1 = %q, want 1", got)
	}
	if got := get(h1); got != "2" {
		t.Fatalf("h1 request 2 = %q, want 2", got)
	}
	// A freshly compiled handler (simulating a reload) restarts the sequence.
	h2 := build()
	if got := get(h2); got != "1" {
		t.Errorf("h2 (reload) request 1 = %q, want 1 (per-route counter resets on reload)", got)
	}
}

// TestSequenceRandom_CompileErrors asserts each invalid selector configuration
// fails at load time with a helpful, specific error message.
func TestSequenceRandom_CompileErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "empty sequence",
			src: `
routes:
  - path: /x
    sequence: []
    repeat: true
`,
			want: "sequence must have at least one response",
		},
		{
			name: "empty random",
			src: `
routes:
  - path: /x
    random: []
`,
			want: "random must have at least one arm",
		},
		{
			name: "zero weight",
			src: `
routes:
  - path: /x
    random:
      - weight: 0
        response: { status: 200, body: a }
`,
			want: "weight must be a positive integer",
		},
		{
			name: "negative weight",
			src: `
routes:
  - path: /x
    random:
      - weight: -3
        response: { status: 200, body: a }
`,
			want: "weight must be a positive integer",
		},
		{
			name: "sequence + response",
			src: `
routes:
  - path: /x
    response: { status: 200, body: top }
    sequence:
      - body: a
`,
			want: "sequence cannot be combined with response or conditions",
		},
		{
			name: "sequence + conditions",
			src: `
routes:
  - path: /x
    sequence:
      - body: a
    conditions:
      - match: { body.x: "*" }
        response: { status: 200, body: c }
`,
			want: "sequence cannot be combined with response or conditions",
		},
		{
			name: "random + response",
			src: `
routes:
  - path: /x
    response: { status: 200, body: top }
    random:
      - weight: 1
        response: { status: 200, body: a }
`,
			want: "random cannot be combined with response or conditions",
		},
		{
			name: "random + conditions",
			src: `
routes:
  - path: /x
    random:
      - weight: 1
        response: { status: 200, body: a }
    conditions:
      - match: { body.x: "*" }
        response: { status: 200, body: c }
`,
			want: "random cannot be combined with response or conditions",
		},
		{
			name: "sse + sequence",
			src: `
routes:
  - path: /x
    sse:
      - data: hi
    sequence:
      - body: a
`,
			want: "only one of sse, sequence, or random",
		},
		{
			name: "sse + random",
			src: `
routes:
  - path: /x
    sse:
      - data: hi
    random:
      - weight: 1
        response: { status: 200, body: a }
`,
			want: "only one of sse, sequence, or random",
		},
		{
			name: "repeat without sequence",
			src: `
routes:
  - path: /x
    repeat: true
    response: { status: 200, body: a }
`,
			want: "repeat is only valid with a sequence",
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

// TestRoutes_RandomArmTemplating proves a chosen random arm's body template is
// rendered (the arm responses go through the normal template path).
func TestRoutes_RandomArmTemplating(t *testing.T) {
	const src = `
routes:
  - path: /tmpl
    method: GET
    random:
      - weight: 1
        response: { status: 200, body: 'p={{.path}}' }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tmpl", nil))
	if got := rec.Body.String(); got != "p=/tmpl" {
		t.Errorf("body = %q, want p=/tmpl (arm body templated)", got)
	}
}
