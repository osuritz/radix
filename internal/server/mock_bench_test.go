package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// benchRouteCount is the size of the benchmark routes table, approximating a
// mid-sized real mock config.
const benchRouteCount = 50

// newBenchCompiledRoutes compiles a routes table of benchRouteCount exact
// GET routes (/api/resource0 ... /api/resourceN-1), failing the benchmark on a
// compile error.
func newBenchCompiledRoutes(b *testing.B) *CompiledRoutes {
	b.Helper()
	var sb strings.Builder
	sb.WriteString("routes:\n")
	for i := 0; i < benchRouteCount; i++ {
		fmt.Fprintf(&sb, "  - path: /api/resource%d\n", i)
		sb.WriteString("    method: GET\n")
		sb.WriteString("    response: { status: 200, body: \"ok\" }\n")
	}
	compiled, err := CompileRoutes([]byte(sb.String()), b.TempDir())
	if err != nil {
		b.Fatalf("CompileRoutes: %v", err)
	}
	return compiled
}

func BenchmarkRoutes_MatchHit(b *testing.B) {
	compiled := newBenchCompiledRoutes(b)
	// The last route in the table is the worst-case exact match (routes within
	// a priority tier are scanned in file order).
	path := fmt.Sprintf("/api/resource%d", benchRouteCount-1)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := compiled.match(http.MethodGet, path); !ok {
			b.Fatalf("match(%q) = miss, want hit", path)
		}
	}
}

func BenchmarkRoutes_MatchMiss(b *testing.B) {
	compiled := newBenchCompiledRoutes(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := compiled.match(http.MethodGet, "/nope/does-not-exist"); ok {
			b.Fatal("match = hit, want miss")
		}
	}
}

// BenchmarkRoutes_TemplateRender serves a templated param route end-to-end
// through the routed handler, covering match, template-data assembly
// (params/query/headers), and body rendering.
func BenchmarkRoutes_TemplateRender(b *testing.B) {
	const src = `
routes:
  - path: /users/:id
    method: GET
    response:
      status: 200
      headers: { Content-Type: application/json }
      body: '{"id": "{{.params.id}}", "q": "{{.query.q}}", "n": {{seq}}}'
`
	compiled, err := CompileRoutes([]byte(src), b.TempDir())
	if err != nil {
		b.Fatalf("CompileRoutes: %v", err)
	}
	store := NewRoutesStore("routes.yml", compiled, nil)
	handler := NewRoutedHandler(RoutedHandlerConfig{Store: store})

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/users/42?q=hello", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want 200", rec.Code)
		}
	}
}
