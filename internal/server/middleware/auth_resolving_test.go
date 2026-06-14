package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderSpec_Template(t *testing.T) {
	tests := []struct {
		name string
		spec HeaderSpec
		want string
	}{
		{
			name: "literal value",
			spec: HeaderSpec{Name: "X-Gateway", Value: "local-dev"},
			want: "local-dev",
		},
		{
			name: "env",
			spec: HeaderSpec{Name: "X-User", Env: "USER_EMAIL"},
			want: "${env:USER_EMAIL}",
		},
		{
			name: "keychain with prefix",
			spec: HeaderSpec{Name: "Authorization", Prefix: "Bearer ", Keychain: &KeychainRef{Service: "work-cli", Account: "jwt"}},
			want: "Bearer ${keychain:work-cli/jwt}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.spec.template()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("template = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeaderSpec_TemplateErrors(t *testing.T) {
	tests := []struct {
		name string
		spec HeaderSpec
	}{
		{"no name", HeaderSpec{Value: "x"}},
		{"no source", HeaderSpec{Name: "X"}},
		{"two sources", HeaderSpec{Name: "X", Value: "a", Env: "B"}},
		{"keychain missing account", HeaderSpec{Name: "X", Keychain: &KeychainRef{Service: "s"}}},
		{"keychain missing service", HeaderSpec{Name: "X", Keychain: &KeychainRef{Account: "a"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.spec.template(); err == nil {
				t.Errorf("template() = nil error, want error")
			}
		})
	}
}

func TestNewSpecProvider_ResolvesHeaders(t *testing.T) {
	t.Setenv("RADIX_TEST_EMAIL", "bob@example.com")
	kc := newFakeKeychain(map[string]string{"work-cli/jwt": "tok-xyz"})

	specs := []HeaderSpec{
		{Name: "X-Auth-Request-Email", Env: "RADIX_TEST_EMAIL"},
		{Name: "Authorization", Prefix: "Bearer ", Keychain: &KeychainRef{Service: "work-cli", Account: "jwt"}},
		{Name: "X-Gateway", Value: "local-dev"},
	}
	provider, err := NewSpecProvider(specs, kc)
	if err != nil {
		t.Fatalf("NewSpecProvider error: %v", err)
	}
	if provider.Name() != "dynamic" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "dynamic")
	}

	hdrs, err := provider.Headers(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("Headers error: %v", err)
	}
	if got := hdrs.Get("X-Auth-Request-Email"); got != "bob@example.com" {
		t.Errorf("X-Auth-Request-Email = %q, want %q", got, "bob@example.com")
	}
	if got := hdrs.Get("Authorization"); got != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok-xyz")
	}
	if got := hdrs.Get("X-Gateway"); got != "local-dev" {
		t.Errorf("X-Gateway = %q, want %q", got, "local-dev")
	}
}

func TestNewSpecProvider_DuplicateName(t *testing.T) {
	specs := []HeaderSpec{
		{Name: "X-Dup", Value: "a"},
		{Name: "X-Dup", Value: "b"},
	}
	if _, err := NewSpecProvider(specs, nil); err == nil {
		t.Fatal("expected error for duplicate header name, got nil")
	}
}

func TestDecodeHeaderSpecs(t *testing.T) {
	raw := map[string]any{
		"headers": []any{
			map[string]any{"name": "X-User", "env": "USER_EMAIL"},
			map[string]any{"name": "Authorization", "prefix": "Bearer ", "keychain": map[string]any{"service": "work-cli", "account": "jwt"}},
		},
	}
	specs, err := DecodeHeaderSpecs(raw)
	if err != nil {
		t.Fatalf("DecodeHeaderSpecs error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(specs))
	}
	if specs[1].Keychain == nil || specs[1].Keychain.Service != "work-cli" {
		t.Errorf("keychain not decoded: %+v", specs[1])
	}
}

func TestDecodeHeaderSpecs_EmptyIsError(t *testing.T) {
	if _, err := DecodeHeaderSpecs(map[string]any{}); err == nil {
		t.Fatal("expected error for empty headers config, got nil")
	}
}

func TestResolveAuthProvider_BuiltinHeaders(t *testing.T) {
	resetProviders()
	settings := AuthSettings{
		Provider: BuiltinHeadersProvider,
		Config: map[string]any{
			"headers": []any{map[string]any{"name": "X-Gateway", "value": "local-dev"}},
		},
	}
	provider, err := ResolveAuthProvider(settings)
	if err != nil {
		t.Fatalf("ResolveAuthProvider error: %v", err)
	}
	if provider == nil || provider.Name() != "dynamic" {
		t.Fatalf("provider = %v, want dynamic provider", provider)
	}
}

func TestResolveAuthProvider_DelegatesToRegistry(t *testing.T) {
	resetProviders()

	// Raw header with a token -> resolving provider (Surface A).
	provider, err := ResolveAuthProvider(AuthSettings{StaticHeaders: []string{"X-User: ${env:USER_EMAIL}"}})
	if err != nil {
		t.Fatalf("ResolveAuthProvider error: %v", err)
	}
	if provider == nil || provider.Name() != "dynamic" {
		t.Errorf("provider = %v, want dynamic provider for templated header", provider)
	}

	// Plain literal header -> static provider.
	provider, err = ResolveAuthProvider(AuthSettings{StaticHeaders: []string{"X-User: literal"}})
	if err != nil {
		t.Fatalf("ResolveAuthProvider error: %v", err)
	}
	if provider == nil || provider.Name() != "static" {
		t.Errorf("provider = %v, want static provider for literal header", provider)
	}
}
