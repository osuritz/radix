# radix proxy

Reverse-proxy incoming requests to a backend service.

```bash
radix proxy [target] [flags]
```

The target is positional or set with `--target`. Requests to
`http://localhost:8080` are forwarded to the backend with `X-Forwarded-*`
headers set automatically.

## When to use it

Put a layer in front of a backend that lacks it: terminate TLS for a plain-HTTP
service, add CORS, strip or rewrite a path prefix, or inject an auth header so
you can talk to a protected API from local code.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target` | | Backend URL (or pass it positionally) |
| `--timeout` | `30s` | Backend response timeout (e.g. `30s`, `1m`) |
| `--strip-prefix` | | Strip this path prefix before forwarding |
| `--rewrite` | | Path rewrite rule, `from:to` |
| `--header` | | Add a request header, `Key: Value` (repeatable) |
| `--cors` | `false` | Add permissive CORS headers to proxied responses |
| `--websocket` | `false` | Enable explicit WebSocket support |
| `--tls-skip-verify` | `false` | Skip TLS verification for the backend connection |
| `--flush-interval` | `-1ns` | Response flush interval: negative = flush immediately (SSE/streaming), `0` = default buffering, `100ms` = periodic |

Global flags apply too — see [Configuration](/configuration#global-flags).

## Examples

### Proxy to a local backend

```bash
radix proxy http://localhost:3000
```

### Strip or rewrite the path

```bash
# /api/users → /users on the backend
radix proxy http://localhost:3000 --strip-prefix /api

# /v1/users → /v2/users on the backend
radix proxy http://localhost:3000 --rewrite /v1:/v2
```

### Inject headers

`--header` is repeatable. Values can contain tokens that are resolved fresh on
every request, so a rotated secret is picked up without restarting radix:

- `${env:NAME}` — value of environment variable `NAME`
- `${keychain:SERVICE/ACCOUNT}` — secret from the OS credential store

```bash
radix proxy http://localhost:3000 \
  --header "X-Auth-Request-Email: ${env:USER_EMAIL}" \
  --header "Authorization: Bearer ${keychain:work-cli/jwt}"
```

::: warning Fail loud, never log secrets
An unset env var or a keychain miss fails the request with `502` rather than
proxying unauthenticated. Resolved values are never written to logs — verbose
injection logging emits header *names* only. Keychain reads are cached for about
10 seconds.
:::

For structured, validatable header config (the `proxy.auth.provider: headers`
form) and corporate auth providers, see [Configuration](/configuration#radix-yml-keys).

### Streaming and SSE backends

The default `--flush-interval -1ns` flushes each write immediately, which is
what you want for Server-Sent Events and chunked streaming. Set `0` to restore
Go's default response buffering:

```bash
radix proxy http://localhost:3000 --flush-interval 0
```

### HTTPS in front, mTLS to the backend

```bash
radix proxy https://backend.internal \
  --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --tls-skip-verify
```

`--tls`/`--cert`/`--key` terminate HTTPS for clients; `--tls-skip-verify`
relaxes verification of the *backend* certificate. See the
[TLS guide](/guides/tls) for backend client certs.
