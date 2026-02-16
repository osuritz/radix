package middleware

import "sync"

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
// Resolution order:
//  1. If config specifies a provider name, use that (nil if not found)
//  2. If exactly one custom provider is registered, use it automatically
//  3. Fall back to StaticProvider from proxy.headers config
//  4. nil if no headers configured and no providers registered
func ResolveProvider(configName string, staticHeaders []string) HeaderProvider {
	providersMu.RLock()
	defer providersMu.RUnlock()

	if configName != "" {
		return providers[configName]
	}

	if len(providers) == 1 {
		for _, p := range providers {
			return p
		}
	}

	if len(staticHeaders) > 0 {
		return NewStaticProvider(parseHeaders(staticHeaders))
	}

	return nil
}

// resetProviders clears the registry (for testing only).
func resetProviders() {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers = map[string]HeaderProvider{}
}
