package middleware

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	keyring "github.com/zalando/go-keyring"
)

// KeychainReader reads a secret from an OS credential store, identified by a
// service and account. It is abstracted so the backend can be swapped (e.g. a
// fake in tests).
type KeychainReader interface {
	Get(service, account string) (string, error)
}

// keyringReader is the default KeychainReader, backed by go-keyring. It maps to
// the macOS Keychain (via the `security` tool), the Windows Credential Manager,
// and the Linux Secret Service (over D-Bus) respectively.
type keyringReader struct{}

func (keyringReader) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

// keychainTTL bounds how long a resolved keychain secret is cached. Header
// values are resolved per request so a rotated token is picked up without
// restarting radix, but reading the OS credential store on every request is
// expensive (a subprocess on macOS). A short TTL keeps the value fresh while
// avoiding a lookup per request.
const keychainTTL = 10 * time.Second

// valueResolver expands ${scheme:arg} tokens in header value templates. It is
// safe for concurrent use.
type valueResolver struct {
	keychain KeychainReader

	mu    sync.Mutex
	cache map[string]cachedSecret // keyed by "service/account"
}

type cachedSecret struct {
	value   string
	expires time.Time
}

// newValueResolver builds a resolver. A nil KeychainReader uses the default OS
// credential store backend.
func newValueResolver(kc KeychainReader) *valueResolver {
	if kc == nil {
		kc = keyringReader{}
	}
	return &valueResolver{keychain: kc, cache: map[string]cachedSecret{}}
}

// resolve expands every ${scheme:arg} token in template; literal text passes
// through unchanged. Supported schemes:
//
//	${env:NAME}                 -> value of environment variable NAME
//	${keychain:SERVICE/ACCOUNT} -> secret from the OS credential store
//
// An unset env var, a failed keychain lookup, an unknown scheme, or a malformed
// token is an error, so a misconfigured source fails loud rather than silently
// proxying a request without the credentials a gateway would require.
func (r *valueResolver) resolve(template string) (string, error) {
	if !strings.Contains(template, "${") {
		return template, nil
	}
	var b strings.Builder
	rest := template
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		rest = rest[start+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated ${...} token")
		}
		val, err := r.resolveToken(rest[:end])
		if err != nil {
			return "", err
		}
		b.WriteString(val)
		rest = rest[end+1:]
	}
	return b.String(), nil
}

func (r *valueResolver) resolveToken(token string) (string, error) {
	scheme, arg, ok := strings.Cut(token, ":")
	if !ok {
		return "", fmt.Errorf("invalid token %q: expected scheme:arg (e.g. env:NAME)", token)
	}
	switch scheme {
	case "env":
		val, ok := os.LookupEnv(arg)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", arg)
		}
		return val, nil
	case "keychain":
		service, account, ok := strings.Cut(arg, "/")
		if !ok || service == "" || account == "" {
			return "", fmt.Errorf("invalid keychain reference %q: expected SERVICE/ACCOUNT", arg)
		}
		return r.keychainGet(service, account)
	default:
		return "", fmt.Errorf("unknown value source %q in token %q", scheme, token)
	}
}

func (r *valueResolver) keychainGet(service, account string) (string, error) {
	key := service + "/" + account

	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.cache[key]; ok && time.Now().Before(c.expires) {
		return c.value, nil
	}
	val, err := r.keychain.Get(service, account)
	if err != nil {
		return "", fmt.Errorf("keychain lookup %q failed: %w", key, err)
	}
	r.cache[key] = cachedSecret{value: val, expires: time.Now().Add(keychainTTL)}
	return val, nil
}

// hasTemplates reports whether any header value contains a ${...} token.
func hasTemplates(headers map[string]string) bool {
	for _, v := range headers {
		if strings.Contains(v, "${") {
			return true
		}
	}
	return false
}
