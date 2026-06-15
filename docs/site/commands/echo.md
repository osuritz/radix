# radix echo

Respond to every request with a JSON description of that request.

```bash
radix echo [flags]
```

Run it, then point any client at `http://localhost:8080` and read back exactly
what it sent: method, headers, query, body, client/server info, TLS state, and
timing. Great for debugging webhooks and HTTP clients.

## When to use it

Inspect what a webhook or SDK actually sends, confirm a header is set, or stand
in for a slow/flaky API while you test client behavior (delays, status codes).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--status`, `-s` | `200` | Default response status code |
| `--body` | | Fixed response body; overrides the echo JSON (empty = echo mode) |
| `--content-type` | `application/json` | Response `Content-Type` |
| `--header` | | Add a response header, `Key: Value` (repeatable) |
| `--echo-body` | `true` | Include the request body in the response |
| `--echo-headers` | `true` | Include request headers in the response |
| `--echo-query` | `true` | Include query parameters in the response |
| `--pretty` | `true` | Pretty-print the JSON response |
| `--body-limit` | `1048576` | Max request body bytes (1 MB); exceeding returns `413` |
| `--delay` | `0` | Delay before responding (e.g. `2s`, `500ms`) |
| `--delay-jitter` | `0` | Random jitter added to the delay |
| `--status-from-path` | `false` | Derive status from the path (`/404` → 404) |
| `--delay-from-path` | `false` | Derive delay from the path (`/delay/500ms`), capped at 10s |
| `--cors` | `false` | Add permissive CORS headers |

Global flags apply too — see [Configuration](/configuration#global-flags).

## Response shape

```bash
curl -X POST 'localhost:8080/anything?foo=bar' \
  -H 'Content-Type: application/json' -d '{"hi":"there"}'
```

```json
{
  "client": { "ip": "127.0.0.1", "port": "52484", "remote_addr": "127.0.0.1:52484" },
  "echo": { "delay_applied": "0s", "request_id": "b41fc88f14e0e65e", "version": "dev" },
  "request": {
    "body": { "hi": "there" },
    "body_raw": "{\"hi\":\"there\"}",
    "body_size": 14,
    "cookies": {},
    "headers": { "Content-Type": ["application/json"], "User-Agent": ["curl/8.7.1"] },
    "method": "POST",
    "path": "/anything",
    "query": { "foo": ["bar"] },
    "url": "/anything?foo=bar"
  },
  "server": { "host": "localhost:8080", "protocol": "HTTP/1.1" },
  "timing": { "timestamp": "2026-06-14T21:33:04-07:00", "unix": 1781497984, "unix_nano": 1781497984279859000 },
  "tls": { "cipher_suite": "", "client_cert": null, "enabled": false, "server_name": "", "version": "" }
}
```

Over HTTPS the `tls` block is populated; with client-auth, `tls.client_cert`
reports the presented certificate (see the [TLS guide](/guides/tls)).

## Examples

### Path-based status and delay

```bash
radix echo --status-from-path --delay-from-path
```

```bash
curl -i localhost:8080/404          # responds 404
curl localhost:8080/delay/500ms     # responds after 500ms (capped at 10s)
```

### Simulate a slow API with jitter

```bash
radix echo --delay 500ms --delay-jitter 200ms
```

Each response waits 500ms plus up to 200ms of random jitter.

### Fixed body instead of echo

```bash
radix echo --body '{"message":"OK"}' --status 201
```

Any non-empty `--body` returns that literal body for every request; clear it to
return to echo mode.
