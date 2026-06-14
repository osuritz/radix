# Custom Auth Provider Guide

This guide shows how to fork Radix and add a custom `HeaderProvider` that injects auth headers into proxied requests. After following these steps, every engineer using the forked binary gets auth headers automatically — no per-machine configuration.

## Overview

Radix's proxy command supports pluggable auth via the `HeaderProvider` interface:

```go
type HeaderProvider interface {
    Headers(ctx context.Context, req *http.Request) (http.Header, error)
    Name() string
}
```

If your fork registers exactly one provider, it's used automatically. No YAML config or CLI flags needed.

If your fork registers **multiple** providers, select one by name with `proxy.auth.provider` in `radix.yml`:

```yaml
proxy:
  auth:
    provider: okta        # must match the name passed to RegisterHeaderProvider
    config:               # provider-specific settings (see below)
      audience: api.internal
```

A `provider` name that isn't registered is a hard error — `radix proxy` fails fast at startup rather than silently injecting no headers. With two or more providers compiled in and no `provider` set, none is auto-selected (the proxy falls back to the static `--header`/`proxy.headers` values, if any).

### Provider-specific config (`auth.config`)

`auth.config` is a free-form map for settings your provider needs (audience, scopes, endpoints, etc.). The built-in `HeaderProvider` interface does not consume it today, so it is **read by the provider itself**: a fork that needs these values reads `cfg.Proxy.Auth.Config` (e.g., in `main.go` before registering, or from its own config loader) and passes them into its provider constructor. Treat it as reserved plumbing for forks rather than a value the core interface injects for you.

## Step-by-Step Example: Bearer Token Provider

This example creates a provider that injects a `Authorization: Bearer <token>` header, fetching the token from an internal credential library.

### 1. Fork and clone

```bash
# Fork osuritz/radix on GitHub, then:
git clone https://github.com/yourorg/radix-fork.git
cd radix-fork
```

### 2. Create the provider

Create `internal/auth/bearer.go`:

```go
package auth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/osuritz/radix/internal/server/middleware"
)

// TokenSource abstracts credential fetching. Replace this with your
// internal library (e.g., Okta SDK, vault client, AWS STS, etc.).
type TokenSource interface {
	// Token returns a valid bearer token and its expiry time.
	Token(ctx context.Context) (token string, expiry time.Time, err error)
}

// BearerProvider injects an Authorization: Bearer header into every
// proxied request. It caches the token and refreshes on expiry.
type BearerProvider struct {
	source TokenSource
	mu     sync.RWMutex
	token  string
	expiry time.Time
}

// NewBearerProvider creates a provider backed by the given TokenSource.
func NewBearerProvider(source TokenSource) *BearerProvider {
	return &BearerProvider{source: source}
}

func (b *BearerProvider) Name() string { return "bearer" }

func (b *BearerProvider) Headers(ctx context.Context, req *http.Request) (http.Header, error) {
	token, err := b.validToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("bearer token refresh failed: %w", err)
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return h, nil
}

func (b *BearerProvider) validToken(ctx context.Context) (string, error) {
	// Fast path: read lock, return cached token if still valid.
	b.mu.RLock()
	if time.Now().Before(b.expiry) {
		defer b.mu.RUnlock()
		return b.token, nil
	}
	b.mu.RUnlock()

	// Slow path: write lock, refresh token.
	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check: another goroutine may have refreshed while we waited.
	if time.Now().Before(b.expiry) {
		return b.token, nil
	}

	token, expiry, err := b.source.Token(ctx)
	if err != nil {
		return "", err
	}
	b.token = token
	b.expiry = expiry
	return token, nil
}

func init() {
	// Replace this with your real TokenSource. For example:
	//   source := okta.NewTokenSource(okta.DefaultConfig())
	//   source := vault.NewTokenSource("secret/data/api-token")
	source := &ExampleTokenSource{}

	middleware.RegisterHeaderProvider("bearer", NewBearerProvider(source))
}
```

### 3. Add your TokenSource implementation

In the same file or a separate one, implement `TokenSource` with your internal library. Here's a minimal example that returns a static token (replace with your real credential logic):

```go
// ExampleTokenSource is a placeholder. Replace with your real implementation.
type ExampleTokenSource struct{}

func (e *ExampleTokenSource) Token(ctx context.Context) (string, time.Time, error) {
	// In practice, this calls your internal credential library:
	//   return oktaClient.GetAccessToken(ctx, audience)
	//   return vaultClient.ReadSecret(ctx, "secret/data/api-token")
	return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.example", time.Now().Add(1 * time.Hour), nil
}
```

### 4. Wire it into main.go

Import the auth package so the `init()` function runs:

```go
package main

import (
	_ "github.com/yourorg/radix-fork/internal/auth" // registers bearer provider
)

// The rest of main.go stays unchanged — Radix's existing main() is called.
```

If your fork uses Radix as a library (rather than copying `main.go`), the blank import is all you need.

### 5. Build and use

```bash
go build -o bin/radix ./cmd/radix

# That's it. The bearer provider is compiled in and auto-detected:
./bin/radix proxy http://backend-service:8080

# Every request proxied to the backend now includes:
#   Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.example
```

No `radix.yml` changes. No CLI flags. Every engineer who builds from the fork gets the same behavior.

## Testing Your Provider

Write a test in `internal/auth/bearer_test.go`:

```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mockTokenSource struct {
	token  string
	expiry time.Time
	err    error
	calls  int
	mu     sync.Mutex
}

func (m *mockTokenSource) Token(ctx context.Context) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.token, m.expiry, m.err
}

func TestBearerProvider_Headers(t *testing.T) {
	source := &mockTokenSource{
		token:  "test-token-123",
		expiry: time.Now().Add(1 * time.Hour),
	}
	provider := NewBearerProvider(source)

	req := httptest.NewRequest("GET", "/api/data", nil)
	hdrs, err := provider.Headers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := hdrs.Get("Authorization")
	want := "Bearer test-token-123"
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestBearerProvider_CachesToken(t *testing.T) {
	source := &mockTokenSource{
		token:  "cached-token",
		expiry: time.Now().Add(1 * time.Hour),
	}
	provider := NewBearerProvider(source)

	req := httptest.NewRequest("GET", "/", nil)

	// Call twice — source should only be hit once (cached).
	provider.Headers(context.Background(), req)
	provider.Headers(context.Background(), req)

	source.mu.Lock()
	defer source.mu.Unlock()
	if source.calls != 1 {
		t.Errorf("TokenSource called %d times, want 1 (should cache)", source.calls)
	}
}

func TestBearerProvider_RefreshesExpiredToken(t *testing.T) {
	source := &mockTokenSource{
		token:  "first-token",
		expiry: time.Now().Add(-1 * time.Second), // already expired
	}
	provider := NewBearerProvider(source)

	req := httptest.NewRequest("GET", "/", nil)
	provider.Headers(context.Background(), req)

	// Update source for second call
	source.mu.Lock()
	source.token = "refreshed-token"
	source.expiry = time.Now().Add(1 * time.Hour)
	source.mu.Unlock()

	hdrs, _ := provider.Headers(context.Background(), req)
	got := hdrs.Get("Authorization")
	if got != "Bearer refreshed-token" {
		t.Errorf("expected refreshed token, got %q", got)
	}
}

func TestBearerProvider_ConcurrentAccess(t *testing.T) {
	source := &mockTokenSource{
		token:  "concurrent-token",
		expiry: time.Now().Add(1 * time.Hour),
	}
	provider := NewBearerProvider(source)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			hdrs, err := provider.Headers(context.Background(), req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if hdrs.Get("Authorization") != "Bearer concurrent-token" {
				t.Errorf("unexpected header value")
			}
		}()
	}
	wg.Wait()
}
```

Run with race detection:

```bash
go test -race ./internal/auth/
```

## Real-World TokenSource Examples

### Okta

```go
type OktaTokenSource struct {
    client   *okta.Client
    audience string
}

func (o *OktaTokenSource) Token(ctx context.Context) (string, time.Time, error) {
    resp, err := o.client.GetAccessToken(ctx, o.audience)
    if err != nil {
        return "", time.Time{}, fmt.Errorf("okta: %w", err)
    }
    return resp.AccessToken, resp.ExpiresAt, nil
}
```

### Vault

```go
type VaultTokenSource struct {
    client *vault.Client
    path   string
}

func (v *VaultTokenSource) Token(ctx context.Context) (string, time.Time, error) {
    secret, err := v.client.Logical().ReadWithContext(ctx, v.path)
    if err != nil {
        return "", time.Time{}, fmt.Errorf("vault: %w", err)
    }
    token := secret.Data["token"].(string)
    ttl, _ := secret.Data["ttl"].(json.Number).Int64()
    return token, time.Now().Add(time.Duration(ttl) * time.Second), nil
}
```

### AWS STS (AssumeRole)

```go
type STSTokenSource struct {
    client  *sts.Client
    roleARN string
}

func (s *STSTokenSource) Token(ctx context.Context) (string, time.Time, error) {
    result, err := s.client.AssumeRole(ctx, &sts.AssumeRoleInput{
        RoleArn:         &s.roleARN,
        RoleSessionName: aws.String("radix-proxy"),
    })
    if err != nil {
        return "", time.Time{}, fmt.Errorf("sts: %w", err)
    }
    return *result.Credentials.SessionToken, *result.Credentials.Expiration, nil
}
```

## Related Documentation

- [IMPLEMENTATION_PLAN.md Section 15](../IMPLEMENTATION_PLAN.md#15-auth-extensions--middleware-extensibility) — Full design specification
- [COMMAND_DESIGN.md — Proxy Command](./COMMAND_DESIGN.md#proxy-command) — Proxy command reference
