package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osuritz/radix/internal/server/middleware"
)

func TestStaticProvider_Headers(t *testing.T) {
	provider := middleware.NewStaticProvider(map[string]string{
		"Authorization": "Bearer static-token",
		"X-Api-Key":     "key123",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	hdrs, err := provider.Headers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := hdrs.Get("Authorization"); got != "Bearer static-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer static-token")
	}
	if got := hdrs.Get("X-Api-Key"); got != "key123" {
		t.Errorf("X-Api-Key = %q, want %q", got, "key123")
	}
}

func TestStaticProvider_HeadersCloned(t *testing.T) {
	provider := middleware.NewStaticProvider(map[string]string{
		"Authorization": "Bearer original",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	hdrs, err := provider.Headers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify returned headers
	hdrs.Set("Authorization", "Bearer modified")

	// Original should be unchanged
	hdrs2, err := provider.Headers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := hdrs2.Get("Authorization"); got != "Bearer original" {
		t.Errorf("Authorization = %q, want %q (headers were not cloned)", got, "Bearer original")
	}
}

func TestStaticProvider_Name(t *testing.T) {
	provider := middleware.NewStaticProvider(nil)
	if got := provider.Name(); got != "static" {
		t.Errorf("Name() = %q, want %q", got, "static")
	}
}
