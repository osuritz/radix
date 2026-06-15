# Configuration reference

Radix takes settings from three places: CLI flags, `RADIX_*` environment
variables, and a `radix.yml` file — with sensible defaults under all of them.

## Precedence

```
CLI flags  >  environment variables  >  config file  >  defaults
```

So `--port 3000` beats `RADIX_PORT`, which beats `port:` in the file, which beats
the built-in default of `8080`.

## Config file

By default radix looks for a config file in this order:

1. `./radix.yml`
2. `~/.radix.yml`
3. `/etc/radix/radix.yml`

Point at a specific file with `--config` / `-c`. A config file is optional —
flags and defaults cover everything.

## Global flags

These apply to every command:

| Flag | Default | Description |
|------|---------|-------------|
| `--config`, `-c` | search path above | Config file to load |
| `--host` | `localhost` | Host/interface to bind |
| `--port`, `-p` | `8080` | Application port |
| `--verbose`, `-v` | `false` | Verbose request logging (Extended CLF) |
| `--no-color` | `false` | Disable ANSI color in logs |
| `--tls` | `false` | Enable HTTPS/TLS |
| `--cert` | | TLS certificate file |
| `--key` | | TLS private key file |
| `--ca` | | CA certificate for client verification |
| `--client-auth` | `false` | Require client TLS certificates (mTLS) |
| `--tls-min-version` | `1.2` | Minimum TLS version (`1.2` or `1.3`) |
| `--metrics` | `true` | Run the admin server (metrics + `/healthz`) |
| `--metrics-path` | `/_metrics` | Metrics endpoint path (on the admin port) |
| `--metrics-format` | `json` | Metrics format (`json` or `prometheus`) |
| `--metrics-port` | `9090` | Admin port (binds `127.0.0.1`) |

## `radix.yml` keys

A complete annotated config. Every key is optional; values shown are the code
defaults unless noted.

```yaml
# Global settings
port: 8080
host: localhost
verbose: false       # verbose request logging (Extended CLF format)
no_color: false      # disable ANSI color in logs (also off when output is not a TTY)

# TLS (applies to all commands when enabled)
tls:
  enabled: false
  cert: ./certs/cert.pem
  key: ./certs/key.pem
  ca: ./certs/ca.pem
  client_auth: false
  min_version: "1.2"   # "1.2" or "1.3" (code default 1.2)

# Metrics — served on a dedicated admin port (default 9090), NOT the app port.
# The admin server always binds loopback (127.0.0.1).
metrics:
  enabled: true
  path: /_metrics
  format: json         # "json" or "prometheus"
  port: 9090           # must differ from `port`

# serve
serve:
  dir: ./public
  index: index.html
  spa: true
  cors: true
  gzip: true
  cache: "max-age=3600"
  hsts: false              # requires tls.enabled
  hsts_max_age: 31536000   # 1 year; 0 clears the policy
  http_redirect: false     # requires tls.enabled
  http_port: 8080          # must differ from `port` when http_redirect is set

# proxy
proxy:
  target: http://localhost:3000
  timeout: 30s
  websocket: true
  tls_skip_verify: false
  backend_ca: ./certs/backend-ca.pem
  backend_cert: ./certs/client-cert.pem
  backend_key: ./certs/client-key.pem
  flush_interval: -1ns     # -1ns = flush immediately (SSE); 0 = default buffering
  rewrite: ""
  strip_prefix: ""
  cors: false
  # Header injection (Surface A). Values may contain ${...} tokens resolved per
  # request: ${env:NAME} and ${keychain:SERVICE/ACCOUNT}. An unset env var or a
  # keychain miss fails the request with 502; injected values are never logged.
  headers: []
  #   - "X-Auth-Request-Email: ${env:USER_EMAIL}"
  #   - "Authorization: Bearer ${keychain:work-cli/jwt}"
  # Structured header provider (Surface B) — same resolver, validatable form:
  # auth:
  #   provider: headers
  #   config:
  #     headers:
  #       - name: X-Auth-Request-Email
  #         env: USER_EMAIL
  #       - name: Authorization
  #         prefix: "Bearer "
  #         keychain: { service: work-cli, account: jwt }
  #       - name: X-Gateway-Id
  #         value: local-dev

# echo
echo:
  status: 200
  delay: 0
  delay_jitter: 0
  body: ""                 # non-empty returns this literal body instead of echo JSON
  content_type: application/json
  headers: []
  echo_body: true
  echo_headers: true
  echo_query: true
  body_limit: 1048576      # 1MB; exceeding returns 413
  pretty: true
  status_from_path: false  # /404 sets the response status
  delay_from_path: false   # /delay/500ms delays the response (capped at 10s)
  cors: false

# mock
mock:
  builtin: true            # register built-in httpbin-style endpoints
  prefix: ""               # mount built-ins under a prefix, e.g. /_test
  latency: "0"
  latency_jitter: "0"
  fail_rate: 0.0           # percentage 0-100
  fail_status: 500
  cors: false
  routes: ./mocks/routes.yml   # custom routes file
  watch: false             # hot-reload the routes file on change
```

::: tip
The proxy auth `headers` provider covers the common corporate case
(env/keychain-sourced tokens) without writing any code. For custom providers
(Okta, Azure AD) compiled into a fork, see the auth notes in the proxy command's
header section: [proxy](/commands/proxy#inject-headers).
:::

## Environment overrides (`RADIX_*`)

Environment variables override the config file but lose to CLI flags.

::: warning Top-level keys only
`RADIX_*` overrides work for **only the four top-level keys**:

| Variable | Overrides |
|----------|-----------|
| `RADIX_PORT` | `port` |
| `RADIX_HOST` | `host` |
| `RADIX_VERBOSE` | `verbose` |
| `RADIX_NO_COLOR` | `no_color` |

Nested keys are **not** settable via environment variables. `RADIX_METRICS_PORT`,
`RADIX_TLS_ENABLED`, `RADIX_SERVE_SPA`, and the like are silently ignored — the
config loader has no key mapping for nested fields. For anything under
`metrics.`, `tls.`, `serve.`, `proxy.`, `echo.`, or `mock.`, use the config file
or a flag.
:::

```bash
RADIX_PORT=3000 radix serve .        # works — top-level key
RADIX_METRICS_PORT=9099 radix serve  # ignored — admin port stays on 9090
radix serve --metrics-port 9099      # use a flag for nested settings
```

## Validation

`radix validate` enforces these rules (also checked at startup):

- `metrics.port` must be `1..65535` and differ from the app `port`
- `metrics.path` must be non-empty, start with `/`, and not equal `/healthz`
- `serve.hsts` and `serve.http_redirect` both require `tls.enabled`
- `serve.http_port` must differ from `port` when `http_redirect` is set
- `serve.hsts_max_age` must not be negative (`0` clears the policy)

See [validate](/commands/validate).
