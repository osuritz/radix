package middleware

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeKeychain is a test KeychainReader that records lookups.
type fakeKeychain struct {
	mu    sync.Mutex
	vals  map[string]string
	calls map[string]int
	err   error
}

func newFakeKeychain(vals map[string]string) *fakeKeychain {
	return &fakeKeychain{vals: vals, calls: map[string]int{}}
}

func (f *fakeKeychain) Get(service, account string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := service + "/" + account
	f.calls[key]++
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.vals[key]
	if !ok {
		return "", errors.New("secret not found")
	}
	return v, nil
}

func (f *fakeKeychain) callCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

func TestValueResolver_LiteralPassthrough(t *testing.T) {
	r := newValueResolver(newFakeKeychain(nil))
	got, err := r.resolve("Bearer plain-literal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bearer plain-literal" {
		t.Errorf("resolve = %q, want %q", got, "Bearer plain-literal")
	}
}

func TestValueResolver_Env(t *testing.T) {
	t.Setenv("RADIX_TEST_USER", "alice@example.com")
	r := newValueResolver(nil)

	got, err := r.resolve("${env:RADIX_TEST_USER}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice@example.com" {
		t.Errorf("resolve = %q, want %q", got, "alice@example.com")
	}
}

func TestValueResolver_EnvUnsetIsError(t *testing.T) {
	r := newValueResolver(nil)
	if _, err := r.resolve("${env:RADIX_DEFINITELY_UNSET_VAR}"); err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestValueResolver_Keychain(t *testing.T) {
	kc := newFakeKeychain(map[string]string{"work-cli/jwt": "tok-123"})
	r := newValueResolver(kc)

	got, err := r.resolve("Bearer ${keychain:work-cli/jwt}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bearer tok-123" {
		t.Errorf("resolve = %q, want %q", got, "Bearer tok-123")
	}
}

func TestValueResolver_KeychainCachedWithinTTL(t *testing.T) {
	kc := newFakeKeychain(map[string]string{"work-cli/jwt": "tok-123"})
	r := newValueResolver(kc)

	for i := 0; i < 3; i++ {
		if _, err := r.resolve("${keychain:work-cli/jwt}"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if n := kc.callCount("work-cli/jwt"); n != 1 {
		t.Errorf("keychain Get called %d times, want 1 (cached within TTL)", n)
	}
}

func TestValueResolver_KeychainExpired(t *testing.T) {
	kc := newFakeKeychain(map[string]string{"work-cli/jwt": "tok-123"})
	r := newValueResolver(kc)

	if _, err := r.resolve("${keychain:work-cli/jwt}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Force the cached entry to be stale.
	r.mu.Lock()
	r.cache["work-cli/jwt"] = cachedSecret{value: "tok-123", expires: time.Unix(0, 0)}
	r.mu.Unlock()

	if _, err := r.resolve("${keychain:work-cli/jwt}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := kc.callCount("work-cli/jwt"); n != 2 {
		t.Errorf("keychain Get called %d times, want 2 (re-read after expiry)", n)
	}
}

func TestValueResolver_KeychainErrorPropagates(t *testing.T) {
	kc := newFakeKeychain(nil)
	kc.err = errors.New("keychain locked")
	r := newValueResolver(kc)

	if _, err := r.resolve("${keychain:work-cli/jwt}"); err == nil {
		t.Fatal("expected error from keychain lookup, got nil")
	}
}

func TestValueResolver_MultipleTokens(t *testing.T) {
	t.Setenv("RADIX_TEST_REGION", "us")
	kc := newFakeKeychain(map[string]string{"svc/acct": "xyz"})
	r := newValueResolver(kc)

	got, err := r.resolve("${env:RADIX_TEST_REGION}-${keychain:svc/acct}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "us-xyz" {
		t.Errorf("resolve = %q, want %q", got, "us-xyz")
	}
}

func TestValueResolver_Errors(t *testing.T) {
	r := newValueResolver(newFakeKeychain(nil))
	tests := []struct {
		name string
		tmpl string
	}{
		{"unterminated", "${env:FOO"},
		{"no colon", "${notascheme}"},
		{"unknown scheme", "${vault:secret/foo}"},
		{"keychain missing slash", "${keychain:noslash}"},
		{"keychain empty account", "${keychain:svc/}"},
		{"keychain empty service", "${keychain:/acct}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := r.resolve(tt.tmpl); err == nil {
				t.Errorf("resolve(%q) = nil error, want error", tt.tmpl)
			}
		})
	}
}

func TestHasTemplates(t *testing.T) {
	if hasTemplates(map[string]string{"A": "literal", "B": "also literal"}) {
		t.Error("hasTemplates = true for literal-only headers, want false")
	}
	if !hasTemplates(map[string]string{"A": "literal", "B": "${env:X}"}) {
		t.Error("hasTemplates = false when a token is present, want true")
	}
}
