# Troubleshooting / FAQ

Common errors and quick fixes.

## Port already in use

radix can't bind because something else holds the port.

```bash
radix serve --port 3000          # pick another port
```

Find and stop the existing process:

```bash
lsof -i :8080                     # macOS/Linux — what's on the port
```

## I can't reach `/_metrics` on the app port

Metrics moved to the **admin port** (default `9090`, loopback-only). The
application port no longer serves `/_metrics`.

```bash
curl http://127.0.0.1:9090/_metrics    # not http://localhost:8080/_metrics
```

Use `--metrics-port` if you changed it. See
[Observability](/guides/observability).

## "metrics port must differ from app port"

`metrics.port` can't equal `port`. They share a default-ish neighborhood but must
be distinct values:

```bash
radix serve --port 9090 --metrics-port 9091
```

## Browser says the certificate isn't trusted

A self-signed cert isn't trusted until you import the CA. Import `ca.pem` into
your OS/browser trust store — see the [TLS guide](/guides/tls#_2-trust-the-ca).
Make sure the hostname you're visiting was passed to `gencert --host`.

## HSTS or HTTP redirect "does nothing"

`--hsts` and `--http-redirect` require `--tls`; radix rejects the combination
without it. And `--http-port` must differ from `--port`:

```bash
radix serve --tls --cert cert.pem --key key.pem \
  --port 8443 --http-redirect --http-port 8080 --hsts
```

## A mock route isn't matching

Check, in order:

- **Method.** A route with `method: GET` won't match a `POST`. Drop `method`/
  `methods` to match any method.
- **Custom vs built-in.** Custom routes win over built-ins, but only if they
  actually match — review the [matching priority](/guides/mock#matching-priority).
- **Regex anchoring.** `regex:` patterns are **not** auto-anchored. Use `^...$`
  to match the whole path, or the pattern matches anywhere in it.
- **Param vs exact.** `/api/users/:id` matches `/api/users/42`, not
  `/api/users`.

## An environment variable isn't taking effect

Only the four **top-level** keys are settable via `RADIX_*`: `RADIX_PORT`,
`RADIX_HOST`, `RADIX_VERBOSE`, `RADIX_NO_COLOR`. Nested keys
(`RADIX_METRICS_PORT`, `RADIX_TLS_ENABLED`, `RADIX_SERVE_SPA`, …) are silently
ignored.

```bash
RADIX_METRICS_PORT=9099 radix serve    # ignored
radix serve --metrics-port 9099        # use a flag
```

Or set it in the config file. See
[Configuration](/configuration#environment-overrides-radix).

## A flag and the config file disagree

CLI flags win. The precedence is: **flags > environment variables > config file >
defaults**. So `--port 3000` overrides `RADIX_PORT`, which overrides `port:` in
the file.

## The proxy returns 502 on a header I injected

That's by design — fail loud. An unset `${env:NAME}` variable or a missing
`${keychain:SERVICE/ACCOUNT}` entry fails the request with `502` rather than
proxying unauthenticated. Confirm the env var is exported or the keychain entry
exists. See [proxy header injection](/commands/proxy#inject-headers).

## SSE / streaming responses arrive all at once

That's usually the client buffering, not radix. Use `curl -N`:

```bash
curl -N http://localhost:8080/api/stream/42
```

For a proxied streaming backend, keep `--flush-interval` negative (the default
`-1ns` flushes immediately) rather than `0`. See [proxy](/commands/proxy#streaming-and-sse-backends).
