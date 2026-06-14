package middleware

import (
	"fmt"
	"sync"
)

// BuiltinHeadersProvider is the reserved provider name selecting the built-in
// structured header provider (Surface B), configured via proxy.auth.config.
// Forks must not register a custom provider under this name.
const BuiltinHeadersProvider = "headers"

var (
	providersMu sync.RWMutex
	providers   = map[string]HeaderProvider{}
)

// AuthSettings bundles the proxy's auth configuration for ResolveAuthProvider.
type AuthSettings struct {
	// Provider is the explicit provider name. "" means auto-detect; the
	// reserved value "headers" selects the built-in structured provider.
	Provider string
	// Config carries provider-specific settings (the built-in "headers"
	// provider reads its config.headers list from here).
	Config map[string]any
	// StaticHeaders are raw "Key: Value" strings from --header / proxy.headers
	// (Surface A). Values may contain ${env:...} / ${keychain:...} tokens.
	StaticHeaders []string
}

// ResolveAuthProvider selects the proxy's HeaderProvider from full auth
// settings. It is the single entry point used by the proxy command and layers
// the structured built-in provider (Surface B) on top of ResolveProvider:
//
//   - Provider == "headers": build the built-in structured provider from
//     Config (Surface B).
//   - otherwise: delegate to ResolveProvider, which handles explicit fork
//     providers, single-provider auto-detection, and the raw-header fallback
//     (Surface A, including ${...} token resolution).
func ResolveAuthProvider(s AuthSettings) (HeaderProvider, error) {
	if s.Provider == BuiltinHeadersProvider {
		specs, err := DecodeHeaderSpecs(s.Config)
		if err != nil {
			return nil, err
		}
		return NewSpecProvider(specs, nil)
	}
	return ResolveProvider(s.Provider, s.StaticHeaders)
}

// RegisterHeaderProvider registers a named HeaderProvider.
// Call this from an init() function in your fork.
func RegisterHeaderProvider(name string, p HeaderProvider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = p
}

// GetHeaderProvider returns a registered provider by name, or nil.
func GetHeaderProvider(name string) HeaderProvider {
	providersMu.RLock()
	defer providersMu.RUnlock()
	return providers[name]
}

// ResolveProvider returns the provider to use based on config and registry state.
//
// When configName is non-empty it selects an explicitly named provider:
//   - registered → that provider, nil error.
//   - not registered → nil and an error (the provider must be compiled in via
//     RegisterHeaderProvider, typically from a fork's init()).
//
// When configName is empty, resolution falls back to auto-detection:
//   - exactly one custom provider registered → that provider.
//   - otherwise, if static headers are configured → a StaticProvider for them.
//   - otherwise → (nil, nil), meaning no header injection.
//
// Auto-detection never errors: an ambiguous registry (two or more providers and
// no explicit name) returns (nil, nil) rather than guessing.
func ResolveProvider(configName string, staticHeaders []string) (HeaderProvider, error) {
	providersMu.RLock()
	defer providersMu.RUnlock()

	if configName != "" {
		p, ok := providers[configName]
		if !ok {
			return nil, fmt.Errorf("auth provider %q is not registered; "+
				"it must be compiled in via RegisterHeaderProvider", configName)
		}
		return p, nil
	}

	if len(providers) == 1 {
		for _, p := range providers {
			return p, nil
		}
	}

	if len(staticHeaders) > 0 {
		parsed := parseHeaders(staticHeaders)
		// When any value carries a ${env:...} / ${keychain:...} token, resolve
		// it per request (Surface A); otherwise keep the cheap static provider.
		if hasTemplates(parsed) {
			return NewResolvingProvider(parsed, nil), nil
		}
		return NewStaticProvider(parsed), nil
	}

	return nil, nil
}

// resetProviders clears the registry (for testing only).
func resetProviders() {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers = map[string]HeaderProvider{}
}
