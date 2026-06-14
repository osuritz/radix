package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-viper/mapstructure/v2"
)

// ResolvingProvider injects headers whose values may contain ${env:...} and
// ${keychain:...} tokens. Values are resolved per request (with an internal
// TTL cache for keychain reads) so a rotated credential is picked up without
// restarting radix. It backs both authoring surfaces:
//
//   - Surface A: raw "Key: Value" header strings (--header / proxy.headers)
//     whose values contain ${...} tokens.
//   - Surface B: the structured proxy.auth.config.headers list, which is
//     compiled down to the same value templates.
type ResolvingProvider struct {
	templates map[string]string // header name -> value template
	resolver  *valueResolver
}

// NewResolvingProvider builds a provider from header name -> value templates.
// A nil KeychainReader uses the default OS credential store backend.
func NewResolvingProvider(templates map[string]string, kc KeychainReader) *ResolvingProvider {
	return &ResolvingProvider{
		templates: templates,
		resolver:  newValueResolver(kc),
	}
}

// Headers resolves each template and returns the resulting headers. A resolution
// failure (unset env var, keychain miss, malformed token) is returned as an
// error so the injection middleware fails the request loud (502) rather than
// proxying without credentials.
func (p *ResolvingProvider) Headers(_ context.Context, _ *http.Request) (http.Header, error) {
	h := http.Header{}
	for name, tmpl := range p.templates {
		val, err := p.resolver.resolve(tmpl)
		if err != nil {
			return nil, fmt.Errorf("resolving header %q: %w", name, err)
		}
		h.Set(name, val)
	}
	return h, nil
}

// Name returns the provider name.
func (p *ResolvingProvider) Name() string { return "dynamic" }

// KeychainRef identifies a secret in the OS credential store.
type KeychainRef struct {
	Service string `mapstructure:"service"`
	Account string `mapstructure:"account"`
}

// HeaderSpec is one structured header definition (Surface B). Exactly one value
// source — Value, Env, or Keychain — must be set. Prefix, if present, is
// prepended to the resolved value (e.g. Prefix "Bearer " with a keychain JWT).
type HeaderSpec struct {
	Name     string       `mapstructure:"name"`
	Value    string       `mapstructure:"value"`    // literal value
	Env      string       `mapstructure:"env"`      // environment variable name
	Keychain *KeychainRef `mapstructure:"keychain"` // OS credential store reference
	Prefix   string       `mapstructure:"prefix"`   // optional value prefix
}

// template compiles the spec into a value template understood by valueResolver,
// validating that exactly one value source is configured.
func (s HeaderSpec) template() (string, error) {
	if s.Name == "" {
		return "", fmt.Errorf("header spec is missing a name")
	}

	var token string
	sources := 0
	if s.Value != "" {
		token = s.Value
		sources++
	}
	if s.Env != "" {
		token = "${env:" + s.Env + "}"
		sources++
	}
	if s.Keychain != nil {
		if s.Keychain.Service == "" || s.Keychain.Account == "" {
			return "", fmt.Errorf("header %q: keychain requires both service and account", s.Name)
		}
		token = "${keychain:" + s.Keychain.Service + "/" + s.Keychain.Account + "}"
		sources++
	}

	switch {
	case sources == 0:
		return "", fmt.Errorf("header %q: one of value, env, or keychain must be set", s.Name)
	case sources > 1:
		return "", fmt.Errorf("header %q: only one of value, env, or keychain may be set", s.Name)
	}

	return s.Prefix + token, nil
}

// NewSpecProvider builds a ResolvingProvider from structured header specs.
// A nil KeychainReader uses the default OS credential store backend.
func NewSpecProvider(specs []HeaderSpec, kc KeychainReader) (*ResolvingProvider, error) {
	templates := make(map[string]string, len(specs))
	for _, spec := range specs {
		tmpl, err := spec.template()
		if err != nil {
			return nil, err
		}
		if _, dup := templates[spec.Name]; dup {
			return nil, fmt.Errorf("duplicate header %q in auth config", spec.Name)
		}
		templates[spec.Name] = tmpl
	}
	return NewResolvingProvider(templates, kc), nil
}

// DecodeHeaderSpecs decodes the structured proxy.auth.config map (Surface B)
// into header specs. The expected shape is {"headers": [ {name, ...}, ... ]}.
func DecodeHeaderSpecs(raw map[string]any) ([]HeaderSpec, error) {
	var cfg struct {
		Headers []HeaderSpec `mapstructure:"headers"`
	}
	if err := mapstructure.Decode(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid auth config: %w", err)
	}
	if len(cfg.Headers) == 0 {
		return nil, fmt.Errorf("auth provider %q requires a non-empty config.headers list", BuiltinHeadersProvider)
	}
	return cfg.Headers, nil
}
