package middleware

import (
	"context"
	"net/http"
	"testing"
)

// testProvider is a minimal HeaderProvider for registry tests.
type testProvider struct {
	name string
}

func (p *testProvider) Headers(_ context.Context, _ *http.Request) (http.Header, error) {
	h := http.Header{}
	h.Set("X-Provider", p.name)
	return h, nil
}

func (p *testProvider) Name() string { return p.name }

func TestRegisterAndGetHeaderProvider(t *testing.T) {
	resetProviders()

	p := &testProvider{name: "okta"}
	RegisterHeaderProvider("okta", p)

	got := GetHeaderProvider("okta")
	if got == nil {
		t.Fatal("GetHeaderProvider returned nil for registered provider")
	}
	if got.Name() != "okta" {
		t.Errorf("Name() = %q, want %q", got.Name(), "okta")
	}
}

func TestGetHeaderProvider_Unknown(t *testing.T) {
	resetProviders()

	got := GetHeaderProvider("nonexistent")
	if got != nil {
		t.Errorf("GetHeaderProvider returned %v, want nil", got)
	}
}

func TestResolveProvider_ExplicitConfigName(t *testing.T) {
	resetProviders()

	RegisterHeaderProvider("azure", &testProvider{name: "azure"})
	RegisterHeaderProvider("okta", &testProvider{name: "okta"})

	// Explicit selection among multiple registered providers.
	got, err := ResolveProvider("azure", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("ResolveProvider returned nil")
	}
	if got.Name() != "azure" {
		t.Errorf("Name() = %q, want %q", got.Name(), "azure")
	}
}

func TestResolveProvider_ExplicitConfigName_NotFound(t *testing.T) {
	resetProviders()

	got, err := ResolveProvider("nope", nil)
	if err == nil {
		t.Fatal("ResolveProvider returned nil error, want error for unregistered provider")
	}
	if got != nil {
		t.Errorf("ResolveProvider returned %v, want nil for missing provider", got)
	}
}

func TestResolveProvider_AutoDetectSingle(t *testing.T) {
	resetProviders()

	RegisterHeaderProvider("okta", &testProvider{name: "okta"})

	got, err := ResolveProvider("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("ResolveProvider returned nil, want auto-detected provider")
	}
	if got.Name() != "okta" {
		t.Errorf("Name() = %q, want %q", got.Name(), "okta")
	}
}

func TestResolveProvider_StaticFallback(t *testing.T) {
	resetProviders()

	headers := []string{"Authorization: Bearer token123", "X-Api-Key: key"}
	got, err := ResolveProvider("", headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("ResolveProvider returned nil, want StaticProvider")
	}
	if got.Name() != "static" {
		t.Errorf("Name() = %q, want %q", got.Name(), "static")
	}
}

func TestResolveProvider_NilWhenNothingConfigured(t *testing.T) {
	resetProviders()

	got, err := ResolveProvider("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("ResolveProvider returned %v, want nil", got)
	}
}

func TestResolveProvider_AmbiguousMultipleProviders(t *testing.T) {
	resetProviders()

	RegisterHeaderProvider("okta", &testProvider{name: "okta"})
	RegisterHeaderProvider("azure", &testProvider{name: "azure"})

	// No config name specified, multiple providers => should not auto-detect
	// and must not error (auto-detection stays silent on ambiguity).
	got, err := ResolveProvider("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("ResolveProvider returned %v, want nil for ambiguous providers", got)
	}
}

func TestResolveProvider_AmbiguousWithStaticFallback(t *testing.T) {
	resetProviders()

	RegisterHeaderProvider("okta", &testProvider{name: "okta"})
	RegisterHeaderProvider("azure", &testProvider{name: "azure"})

	// Multiple providers but static headers provided => static fallback
	headers := []string{"X-Fallback: yes"}
	got, err := ResolveProvider("", headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("ResolveProvider returned nil, want StaticProvider fallback")
	}
	if got.Name() != "static" {
		t.Errorf("Name() = %q, want %q", got.Name(), "static")
	}
}
