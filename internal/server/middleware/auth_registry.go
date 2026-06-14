package middleware

import (
	"fmt"
	"sync"
)

var (
	providersMu sync.RWMutex
	providers   = map[string]HeaderProvider{}
)

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
		return NewStaticProvider(parseHeaders(staticHeaders)), nil
	}

	return nil, nil
}

// resetProviders clears the registry (for testing only).
func resetProviders() {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers = map[string]HeaderProvider{}
}
