package middleware

import (
	"context"
	"net/http"
	"strings"
)

// StaticProvider injects fixed headers from configuration.
type StaticProvider struct {
	headers http.Header
}

// NewStaticProvider creates a StaticProvider from a map of header key-value pairs.
func NewStaticProvider(headers map[string]string) *StaticProvider {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &StaticProvider{headers: h}
}

// Headers returns a clone of the static headers.
func (s *StaticProvider) Headers(_ context.Context, _ *http.Request) (http.Header, error) {
	return s.headers.Clone(), nil
}

// Name returns the provider name.
func (s *StaticProvider) Name() string { return "static" }

// parseHeaders converts a slice of "Key: Value" strings into a map.
func parseHeaders(headers []string) map[string]string {
	result := make(map[string]string, len(headers))
	for _, h := range headers {
		key, value, ok := strings.Cut(h, ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}
