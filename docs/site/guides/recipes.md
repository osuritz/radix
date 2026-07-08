# Recipes

Real-world setups, each built from flags and config keys that exist today.
Every recipe is copy-paste runnable; follow the links for the full reference
on each command.

[[toc]]

## SPA dev server with an API proxy

Serve a built single-page app and put a CORS-enabled proxy in front of the
backend API — no framework dev server, no nginx config.

Terminal 1 — the app. `--spa` serves `index.html` for any path that isn't a
real file, so client-side routes deep-link correctly:

```bash
radix serve ./dist --spa --port 3000
```

Terminal 2 — the API. The proxy strips the `/api` prefix before forwarding, so
the frontend calls `http://localhost:8080/api/users` and the backend sees
`/users`. `--cors` adds the headers the browser needs for the cross-origin
call from `:3000`:

```bash
radix proxy http://localhost:8000 --strip-prefix /api --cors
```

Point the frontend's API base URL at `http://localhost:8080/api` and you have
the classic dev-server-plus-proxy setup in two commands.

::: tip Mock what doesn't exist yet
If parts of the API aren't built, swap the proxy for `radix mock` with a
`fallback.type: proxy` — mocked routes answer locally and everything else
passes through to the real backend. See the next recipe and the
[mock guide](/guides/mock#settings-and-chaos).
:::

See [serve](/commands/serve) and [proxy](/commands/proxy).

## Mock a third-party API with hot reload

Develop against a paid or rate-limited third-party API without hitting it.
Describe the endpoints you use in a routes file:

```yaml
# thirdparty.yml
settings:
  fallback:
    type: proxy                       # anything unmocked hits the real API
    proxy_target: https://api.thirdparty.example

routes:
  - path: /v1/customers/:id
    method: GET
    response:
      status: 200
      headers: { Content-Type: application/json }
      body: |
        {
          "id": "{{.params.id}}",
          "name": "{{faker.name}}",
          "email": "{{faker.email}}",
          "created": "{{now}}"
        }

  - path: /v1/charges
    method: POST
    response:
      status: 201
      headers: { Content-Type: application/json }
      body: '{"id":"ch_{{randomString 14}}","amount":{{.body.amount}},"status":"succeeded"}'
```

Run it with hot reload and point your app's API base URL at
`http://localhost:8080`:

```bash
radix mock --routes thirdparty.yml --watch
```

Edit the file and save — routes, the fallback, and latency/fail-rate settings
take effect immediately; a broken edit is rejected and the previous good
config keeps serving. To rehearse how your app copes with a degraded
upstream, add chaos without touching the file:

```bash
radix mock --routes thirdparty.yml --watch --latency 300ms --fail-rate 10
```

See the [mock command](/commands/mock) and the [mock guide](/guides/mock)
for conditions, sequences, weighted-random responses, and SSE.

## HTTPS locally with a trusted CA

Reproduce production HTTPS behavior (secure cookies, service workers, mixed
content) without certificate warnings.

Generate a CA and a server certificate signed by it — list every hostname and
IP you'll use so they end up in the SANs:

```bash
radix gencert --host localhost,127.0.0.1 --output ./certs
```

Trust the CA once (full per-platform commands are in the generated
`./certs/README.txt` and the [TLS guide](/guides/tls#_2-trust-the-ca)):

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ./certs/ca.pem
```

Then serve over HTTPS — with an HTTP listener that 308-redirects, like
production:

```bash
radix serve ./dist --spa \
  --tls --cert ./certs/cert.pem --key ./certs/key.pem \
  --port 8443 \
  --http-redirect --http-port 8080
```

`https://localhost:8443` loads with a green padlock; `http://localhost:8080`
redirects to it. The same `--tls`/`--cert`/`--key` flags work for `proxy`,
`echo`, and `mock`. See [gencert](/commands/gencert) and the
[TLS guide](/guides/tls).

## Debug webhooks with echo

See exactly what a webhook provider sends — method, headers, signature,
body — before writing a line of handler code:

```bash
radix echo --verbose
```

Point the provider (or your tunnel — ngrok, cloudflared, etc.) at
`http://localhost:8080` and every delivery comes back — and is logged — as
structured JSON: `request.headers` shows the signature header, `request.body`
the parsed payload, `request.body_raw` the exact bytes to verify the
signature against.

Then rehearse the failure paths. Derive the response status from the request
path to test the provider's retry behavior, or delay responses to test its
timeout handling:

```bash
radix echo --status-from-path    # point the webhook at /500 → provider sees a 500
radix echo --delay 25s           # does the provider give up and retry?
```

Large payloads are capped at 1 MB by default; raise the cap with
`--body-limit` if your provider sends more. See [echo](/commands/echo) for
the full response shape.

## Corporate API proxy with a keychain-sourced token

Talk to a token-protected internal API from local tools without pasting the
token into shell history, env files, or code. Store it once in the OS
credential store (macOS Keychain, Windows Credential Manager, Linux Secret
Service):

```bash
# macOS — prompts for the secret
security add-generic-password -s work-cli -a jwt -w
```

Then run a local proxy that injects the header on every request, reading the
token fresh from the keychain (with a ~10s cache), so a rotated token is
picked up without restarting radix:

```bash
radix proxy https://api.corp.example \
  --header 'Authorization: Bearer ${keychain:work-cli/jwt}'
```

Note the **single quotes** — the `${keychain:...}` token is resolved by
radix, not your shell. Your tools now call `http://localhost:8080` and reach
the API authenticated.

The same setup as config, using the structured `headers` provider:

```yaml
# radix.yml
proxy:
  target: https://api.corp.example
  auth:
    provider: headers
    config:
      headers:
        - name: Authorization
          prefix: "Bearer "
          keychain: { service: work-cli, account: jwt }
        - name: X-Auth-Request-Email
          env: USER_EMAIL
```

```bash
radix proxy --config ./radix.yml
```

::: warning Fail loud, never log secrets
A keychain miss or unset env var fails the request with `502` rather than
proxying unauthenticated, and resolved values are never written to logs —
verbose injection logging emits header *names* only.
:::

See [proxy header injection](/commands/proxy#inject-headers) and
[Configuration](/configuration#radix-yml-keys).
