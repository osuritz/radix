package middleware_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/server/middleware"
)

// secretProvider returns a header carrying a secret value, for redaction tests.
type secretProvider struct {
	err error
}

func (p *secretProvider) Headers(_ context.Context, _ *http.Request) (http.Header, error) {
	if p.err != nil {
		return nil, p.err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer super-secret-token-value")
	return h, nil
}

func (p *secretProvider) Name() string { return "dynamic" }

func TestInjectHeaders_VerboseLogRedactsValues(t *testing.T) {
	var buf bytes.Buffer
	handler := middleware.InjectHeaders(
		&secretProvider{},
		middleware.WithVerboseLogging(&buf),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	logged := buf.String()
	if !strings.Contains(logged, "Authorization") {
		t.Errorf("log should name the injected header, got %q", logged)
	}
	if strings.Contains(logged, "super-secret-token-value") {
		t.Errorf("log leaked a secret value: %q", logged)
	}
}

func TestInjectHeaders_VerboseLogErrorNoClientLeak(t *testing.T) {
	var buf bytes.Buffer
	handler := middleware.InjectHeaders(
		&secretProvider{err: errors.New("keychain \"work-cli/jwt\" failed")},
		middleware.WithVerboseLogging(&buf),
	)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("next handler must not run on provider error")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	// The client response must not carry the provider's internal error detail.
	if strings.Contains(rec.Body.String(), "work-cli") {
		t.Errorf("client response leaked provider error detail: %q", rec.Body.String())
	}
	// The server-side log may name the source, but never a secret value.
	if !strings.Contains(buf.String(), "dynamic") {
		t.Errorf("verbose log should name the provider, got %q", buf.String())
	}
}
