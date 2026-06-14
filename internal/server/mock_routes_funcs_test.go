package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// renderFunc compiles a single-route YAML whose body is bodyTmpl and returns the
// rendered body for a GET /t, failing the test on a non-200 status.
func renderFunc(t *testing.T, bodyTmpl string) string {
	t.Helper()
	src := "routes:\n  - path: /t\n    response: { status: 200, body: '" + bodyTmpl + "' }\n"
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/t", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("render %q: status %d (body %q)", bodyTmpl, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestRouteFuncs_RandomFloat(t *testing.T) {
	t.Run("within range", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			out := renderFunc(t, "{{randomFloat 1.5 2.5}}")
			f, err := strconv.ParseFloat(out, 64)
			if err != nil {
				t.Fatalf("randomFloat output %q not a float: %v", out, err)
			}
			if f < 1.5 || f >= 2.5 {
				t.Fatalf("randomFloat = %v, want [1.5, 2.5)", f)
			}
		}
	})

	t.Run("swapped bounds", func(t *testing.T) {
		// min > max must be tolerated by swapping, yielding [2, 5).
		for i := 0; i < 200; i++ {
			out := renderFunc(t, "{{randomFloat 5.0 2.0}}")
			f, err := strconv.ParseFloat(out, 64)
			if err != nil {
				t.Fatalf("randomFloat output %q not a float: %v", out, err)
			}
			if f < 2.0 || f >= 5.0 {
				t.Fatalf("randomFloat(5,2) = %v, want [2, 5)", f)
			}
		}
	})

	t.Run("equal bounds", func(t *testing.T) {
		out := renderFunc(t, "{{randomFloat 3.0 3.0}}")
		f, err := strconv.ParseFloat(out, 64)
		if err != nil || f != 3.0 {
			t.Fatalf("randomFloat(3,3) = %q (%v), want 3", out, err)
		}
	})
}

func TestRouteFuncs_RandomChoice(t *testing.T) {
	t.Run("membership", func(t *testing.T) {
		allowed := map[string]bool{"red": true, "green": true, "blue": true}
		for i := 0; i < 200; i++ {
			out := renderFunc(t, `{{randomChoice "red" "green" "blue"}}`)
			if !allowed[out] {
				t.Fatalf("randomChoice = %q, not a provided option", out)
			}
		}
	})

	t.Run("single arg", func(t *testing.T) {
		if out := renderFunc(t, `{{randomChoice "only"}}`); out != "only" {
			t.Fatalf("randomChoice single = %q, want only", out)
		}
	})

	t.Run("no args errors with 500", func(t *testing.T) {
		src := "routes:\n  - path: /t\n    response: { status: 200, body: '{{randomChoice}}' }\n"
		rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/t", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("randomChoice with no args: status %d, want 500", rec.Code)
		}
	})
}

func TestRouteFuncs_Lorem(t *testing.T) {
	tests := []struct {
		name      string
		tmpl      string
		wantWords int
	}{
		{"five words", "{{lorem 5}}", 5},
		{"one word", "{{lorem 1}}", 1},
		{"zero words empty", "{{lorem 0}}", 0},
		{"negative empty", "{{lorem -3}}", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderFunc(t, tt.tmpl)
			if tt.wantWords == 0 {
				if out != "" {
					t.Fatalf("lorem = %q, want empty", out)
				}
				return
			}
			words := strings.Fields(out)
			if len(words) != tt.wantWords {
				t.Fatalf("lorem word count = %d (%q), want %d", len(words), out, tt.wantWords)
			}
			for _, w := range words {
				found := false
				for _, lw := range loremWords {
					if w == lw {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("lorem produced unknown word %q", w)
				}
			}
		})
	}
}

func TestRouteFuncs_Hash(t *testing.T) {
	// Independently-known digest vectors for the fixed input "radix" (verified
	// out-of-band via shasum/md5). Hardcoding the expecteds avoids recomputing
	// them with crypto/* in the test tree and asserts against known-good values.
	const (
		radixSHA256 = "da7f85eaf3d0452479031da124d28778aaf15cc756a6c909d7dc708fade343f0"
		radixSHA1   = "5f33e8ddd36b0c849687df732835b9abbe9b347b"
		radixMD5    = "be4ecdb8a8ebc5a7a7740d21d2b71462"
	)

	tests := []struct {
		name string
		tmpl string
		want string
	}{
		// Known SHA-256 vector for the empty string (well-published constant).
		{"sha256 empty vector", `{{hash "sha256" ""}}`, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"sha256", `{{hash "sha256" "radix"}}`, radixSHA256},
		{"sha1", `{{hash "sha1" "radix"}}`, radixSHA1},
		{"md5", `{{hash "md5" "radix"}}`, radixMD5},
		// Algorithm name is case-insensitive.
		{"uppercase algo", `{{hash "SHA256" "radix"}}`, radixSHA256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := renderFunc(t, tt.tmpl); out != tt.want {
				t.Fatalf("%s = %q, want %q", tt.tmpl, out, tt.want)
			}
		})
	}

	t.Run("unknown algo errors with 500", func(t *testing.T) {
		src := "routes:\n  - path: /t\n    response: { status: 200, body: '{{hash \"crc32\" \"x\"}}' }\n"
		rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/t", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("hash unknown algo: status %d, want 500", rec.Code)
		}
	})
}

func TestRouteFuncs_NowLayout(t *testing.T) {
	t.Run("default RFC3339", func(t *testing.T) {
		out := renderFunc(t, "{{now}}")
		if _, err := time.Parse(time.RFC3339, out); err != nil {
			t.Fatalf("now = %q, not RFC3339: %v", out, err)
		}
	})

	t.Run("custom layout", func(t *testing.T) {
		out := renderFunc(t, `{{now "2006-01-02"}}`)
		parsed, err := time.Parse("2006-01-02", out)
		if err != nil {
			t.Fatalf("now layout = %q, not parseable with 2006-01-02: %v", out, err)
		}
		// The rendered date should be today's UTC date.
		if got, want := parsed.Format("2006-01-02"), time.Now().UTC().Format("2006-01-02"); got != want {
			t.Fatalf("now date = %q, want %q", got, want)
		}
	})

	t.Run("time-only layout", func(t *testing.T) {
		out := renderFunc(t, `{{now "15:04:05"}}`)
		if _, err := time.Parse("15:04:05", out); err != nil {
			t.Fatalf("now time layout = %q, not parseable: %v", out, err)
		}
	})
}

func TestRouteFuncs_Faker(t *testing.T) {
	// The core assertion of the issue: the dotted {{faker.name}} form must
	// evaluate under Go's text/template. Render each field and sanity-check shape.
	t.Run("name", func(t *testing.T) {
		out := renderFunc(t, "{{faker.name}}")
		if len(strings.Fields(out)) != 2 {
			t.Fatalf("faker.name = %q, want two words", out)
		}
	})

	t.Run("email", func(t *testing.T) {
		out := renderFunc(t, "{{faker.email}}")
		if !strings.Contains(out, "@") || !strings.Contains(out, ".") {
			t.Fatalf("faker.email = %q, want an address", out)
		}
	})

	t.Run("phone", func(t *testing.T) {
		out := renderFunc(t, "{{faker.phone}}")
		if !strings.HasPrefix(out, "(") || !strings.Contains(out, ")") || !strings.Contains(out, "-") {
			t.Fatalf("faker.phone = %q, want a formatted phone number", out)
		}
	})

	t.Run("address", func(t *testing.T) {
		out := renderFunc(t, "{{faker.address}}")
		if !strings.Contains(out, ",") || strings.Fields(out)[0] == "" {
			t.Fatalf("faker.address = %q, want a street + city", out)
		}
	})

	t.Run("multiple fields in one template", func(t *testing.T) {
		// Confirms the whole record renders together (each faker call is fresh).
		out := renderFunc(t, "{{faker.name}}|{{faker.email}}|{{faker.phone}}|{{faker.address}}")
		parts := strings.Split(out, "|")
		if len(parts) != 4 {
			t.Fatalf("faker multi = %q, want 4 parts", out)
		}
		for i, p := range parts {
			if strings.TrimSpace(p) == "" {
				t.Fatalf("faker multi part %d empty in %q", i, out)
			}
		}
	})
}

func TestRouteFuncs_Seq(t *testing.T) {
	t.Run("increments from one per route", func(t *testing.T) {
		src := "routes:\n  - path: /t\n    response: { status: 200, body: '{{seq}}' }\n"
		store := newStore(t, src, t.TempDir())
		h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
		for want := 1; want <= 5; want++ {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t", nil))
			if rec.Body.String() != strconv.Itoa(want) {
				t.Fatalf("seq call %d = %q, want %d", want, rec.Body.String(), want)
			}
		}
	})

	t.Run("counter is per-route", func(t *testing.T) {
		// Two distinct routes must not share a counter.
		src := "routes:\n" +
			"  - path: /a\n    response: { status: 200, body: '{{seq}}' }\n" +
			"  - path: /b\n    response: { status: 200, body: '{{seq}}' }\n"
		store := newStore(t, src, t.TempDir())
		h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
		hit := func(p string) string {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
			return rec.Body.String()
		}
		if got := hit("/a"); got != "1" {
			t.Fatalf("/a first = %q, want 1", got)
		}
		if got := hit("/a"); got != "2" {
			t.Fatalf("/a second = %q, want 2", got)
		}
		if got := hit("/b"); got != "1" {
			t.Fatalf("/b first = %q, want 1 (separate counter)", got)
		}
	})

	t.Run("shared across a route's condition arms", func(t *testing.T) {
		// A route's top-level body and its condition arms draw from one counter.
		src := "routes:\n" +
			"  - path: /t\n" +
			"    conditions:\n" +
			"      - match: { query.arm: \"yes\" }\n" +
			"        response: { status: 200, body: 'arm-{{seq}}' }\n" +
			"      - default: true\n" +
			"        response: { status: 200, body: 'def-{{seq}}' }\n"
		store := newStore(t, src, t.TempDir())
		h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
		hit := func(p string) string {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
			return rec.Body.String()
		}
		if got := hit("/t?arm=yes"); got != "arm-1" {
			t.Fatalf("first (matched arm) = %q, want arm-1", got)
		}
		if got := hit("/t"); got != "def-2" {
			t.Fatalf("second (default arm) = %q, want def-2 (shared counter)", got)
		}
	})

	t.Run("resets on reload", func(t *testing.T) {
		// Recompiling (hot-reload) allocates a fresh counter, so seq restarts at 1.
		dir := t.TempDir()
		path := filepath.Join(dir, "routes.yml")
		body := []byte("routes:\n  - path: /t\n    response: { status: 200, body: '{{seq}}' }\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write routes: %v", err)
		}
		compiled, err := LoadRoutes(path)
		if err != nil {
			t.Fatalf("LoadRoutes: %v", err)
		}
		store := NewRoutesStore(path, compiled, nil)
		h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
		hit := func() string {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t", nil))
			return rec.Body.String()
		}
		if got := hit(); got != "1" {
			t.Fatalf("pre-reload first = %q, want 1", got)
		}
		if got := hit(); got != "2" {
			t.Fatalf("pre-reload second = %q, want 2", got)
		}
		// Reload from the same file (Reload recompiles -> fresh compiledRoute).
		if err := store.Reload(); err != nil {
			t.Fatalf("Reload: %v", err)
		}
		if got := hit(); got != "1" {
			t.Fatalf("post-reload first = %q, want 1 (counter must reset)", got)
		}
	})

	t.Run("concurrent increments are race-clean and contiguous", func(t *testing.T) {
		// Drive a single route's seq closure from many goroutines and confirm the
		// values returned are exactly 1..N with no duplicates or gaps.
		const n = 500
		counter := new(atomic.Uint64)
		fm := routeFuncMap(counter)
		seqFn, ok := fm["seq"].(func() uint64)
		if !ok {
			t.Fatalf("seq func has unexpected type %T", fm["seq"])
		}
		var wg sync.WaitGroup
		seen := make([]uint64, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				seen[idx] = seqFn()
			}(i)
		}
		wg.Wait()
		got := make(map[uint64]bool, n)
		for _, v := range seen {
			if v < 1 || v > n {
				t.Fatalf("seq value %d out of range [1,%d]", v, n)
			}
			if got[v] {
				t.Fatalf("seq value %d returned more than once", v)
			}
			got[v] = true
		}
		if len(got) != n {
			t.Fatalf("distinct seq values = %d, want %d", len(got), n)
		}
	})
}
