# Mock guide

`radix mock` ships httpbin-style endpoints out of the box and lets you layer
custom YAML routes on top — templated bodies, conditional responses, sequenced
and weighted-random responses, and SSE streaming. This guide is the deep dive;
for the flag list see the [mock command](/commands/mock).

```bash
radix mock                                   # built-ins only
radix mock examples/mock-routes.yml          # built-ins + custom routes
radix mock --routes routes.yml --watch       # hot-reload on save
```

## Built-in endpoints

Available immediately with `radix mock` (mount them under a prefix with
`--prefix /_test`):

| Endpoint | Returns |
|----------|---------|
| `/get` `/post` `/put` `/patch` `/delete` | Request echo for that method |
| `/anything` | Request echo for any method |
| `/headers` | The request headers |
| `/ip` | The client IP |
| `/user-agent` | The `User-Agent` header |
| `/uuid` | A random v4 UUID |
| `/status/{code}` | A response with that status code |
| `/delay/{n}` | Responds after `n` seconds |
| `/bytes/{n}` | `n` bytes of data |
| `/json` `/html` `/xml` | A sample payload in that format |

## Custom YAML routes

A routes file has a `settings:` block and a list of `routes:`. Each route has a
`path`, an optional `method`/`methods`, and a `response` (or `conditions:` or
`sse:`).

```yaml
routes:
  - path: /api/health
    method: GET
    response:
      status: 200
      headers:
        Content-Type: application/json
      body: '{"status":"ok"}'
```

### Matching priority

When a request arrives, radix picks the first route that matches, in this order:

1. **exact path + matching method**
2. **exact path + any method** (route has no `method`/`methods`)
3. **`:param` path**
4. **`regex:` path**
5. **`/*` trailing glob**
6. **built-in endpoint**
7. **fallback** (`404` or `proxy`)

### Path kinds

| Kind | Example | Notes |
|------|---------|-------|
| Exact | `/api/health` | Literal match |
| Param | `/api/users/:id` | `:id` captured into `.params.id` |
| Regex | `regex:^/api/v[0-9]+/users$` | Go `regexp`, **not** auto-anchored — use `^...$` |
| Glob | `/assets/*` | Prefix match on everything under `/assets/` |

::: warning Regex is not anchored
`regex:` patterns use Go [`regexp`](https://pkg.go.dev/regexp) semantics and
match if found *anywhere* in the path. Anchor with `^...$` to match the whole
path, or `/api/v1/users` would also match `/x/api/v1/users/y`.
:::

### Methods

Use `method:` for one verb or `methods:` for several:

```yaml
- path: /api/resource
  methods: [GET, POST, PUT, DELETE]
  response:
    status: 200
    body: '{"method":"{{.method}}","path":"{{.path}}"}'
```

### Response from a file

Read the body from a file (path is relative to the routes file's directory).
The file contents are templated too:

```yaml
- path: /api/products
  method: GET
  response:
    headers:
      Content-Type: application/json
    file: ./mocks/products.json
```

## Templating

Bodies are Go [`text/template`](https://pkg.go.dev/text/template). The dot gives
you the request; functions generate data.

### Request data context

| Expression | What it is |
|------------|------------|
| `.method` | Request method |
| `.path` | Request path |
| `.params.id` | Captured `:id` path parameter |
| `.query.q` | Query parameter `q` |
| `.headers.X` | Request header `X` |
| `.body.field` | Top-level scalar of the parsed JSON body, or a form value |

::: tip Header names with dashes
`.headers.Content-Type` isn't a valid template field. Use `index`:
<span v-pre>`{{index .headers "Content-Type"}}`</span>.
:::

```yaml
- path: /api/users/:id
  method: GET
  response:
    status: 200
    headers:
      Content-Type: application/json
    body: |
      {
        "id": "{{.params.id}}",
        "name": "User {{.params.id}}",
        "email": "user{{.params.id}}@example.com"
      }
```

```bash
curl localhost:8080/api/users/42
# {"id":"42","name":"User 42","email":"user42@example.com"}
```

### Generator functions

::: v-pre

| Function | Result |
|----------|--------|
| `{{uuid}}` | Random v4 UUID |
| `{{now}}` / `{{now "2006-01-02"}}` | Current UTC time (RFC3339, or a Go layout) |
| `{{timestamp}}` | Current Unix time (seconds) |
| `{{random low high}}` | Random int in `[low, high)` |
| `{{randomFloat min max}}` | Random float64 in `[min, max)` |
| `{{randomChoice "a" "b" ...}}` | One argument at random |
| `{{randomString n}}` | `n` random alphanumeric characters |
| `{{lorem n}}` | `n` lorem-ipsum words |
| `{{seq}}` | Per-route counter from 1 (resets on reload) |
| `{{hash "sha256" "text"}}` | Hex digest (`sha256`, `sha1`, or `md5`) |
| `{{faker.name}}` `.email` `.phone` `.address` | Placeholder identity data |
| `{{env "VAR"}}` | Environment variable value |
| `{{base64 "s"}}` | Base64-encoded string |

:::

```yaml
- path: /api/fake/user
  method: GET
  response:
    status: 200
    headers:
      Content-Type: application/json
    body: |
      {
        "seq": {{seq}},
        "id": "{{uuid}}",
        "name": "{{faker.name}}",
        "email": "{{faker.email}}",
        "role": "{{randomChoice "admin" "editor" "viewer"}}",
        "score": {{randomFloat 0 100}},
        "bio": "{{lorem 8}}",
        "etag": "{{hash "sha256" "user"}}",
        "created_on": "{{now "2006-01-02"}}"
      }
```

## Conditional responses

A route with `conditions:` picks its response by matching request content. Arms
are evaluated top to bottom; the **first** arm whose every `match` entry is
satisfied wins.

Match keys are dotted and prefixed with `body.`, `query.`, or `headers.`. A
value of `"*"` means "present with any non-empty value"; any other value is an
exact match. `body.<field>` resolves a **top-level scalar** of the parsed JSON
object (or a form value) — nested paths and objects/arrays aren't match targets.

```yaml
- path: /api/auth/login
  method: POST
  conditions:
    - match:                         # correct credentials
        body.username: admin
        body.password: secret
      response:
        status: 200
        headers: { Content-Type: application/json }
        body: '{"token":"{{uuid}}","user":"{{.body.username}}"}'
    - match:                         # any username, wrong creds
        body.username: "*"
      response:
        status: 401
        body: '{"error":"invalid credentials"}'
    - default: true                  # nothing matched
      response:
        status: 400
        body: '{"error":"username is required"}'
```

```bash
curl -X POST localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' -d '{"username":"admin","password":"secret"}'
# {"token":"<uuid>","user":"admin"}

curl -X POST localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' -d '{"username":"admin","password":"nope"}'
# {"error":"invalid credentials"}
```

::: tip
Body matching needs the body to be parseable, so send the right `Content-Type`
(`application/json` for JSON, `application/x-www-form-urlencoded` for form data).
:::

Every non-`default` arm needs at least one `match` rule; use `default: true` for
an unconditional last arm.

**Precedence when serving:** winning arm → `default: true` arm → the route's
top-level `response` (only if one was provided) → `404`. A route with no
conditions and an absent/empty `response` serves `200` with an empty body.

You can also branch on a query parameter or header, with a top-level `response`
as the fallback:

```yaml
- path: /api/protected
  method: GET
  conditions:
    - match:
        headers.Authorization: "Bearer valid-token"
      response:
        status: 200
        body: '{"data":"secret"}'
  response:
    status: 401
    body: '{"error":"missing or invalid authorization"}'
```

## Server-Sent Events

An `sse:` block streams a `text/event-stream` response of scripted events
instead of a single body (it replaces `response`/`conditions` for the route).
Each event can set:

| Field | Meaning |
|-------|---------|
| `delay` | Wait before sending the event (Go duration or seconds) |
| `event` | Optional SSE event name (an `event:` line) |
| `data` | Templated payload (same data/funcs as a body; multi-line becomes multiple `data:` lines) |
| `repeat` | Send the event this many times (default 1) |
| `repeat_delay` | Wait between successive repeats |

The handler flushes after each event so clients receive them incrementally, and
returns promptly when the client disconnects.

```yaml
- path: /api/stream/:id
  method: GET
  sse:
    - event: open
      data: '{"status":"connected","id":"{{.params.id}}"}'
    - data: '{"seq":{{seq}},"ts":"{{now}}","value":{{randomFloat 0 100}}}'
      repeat: 5
      repeat_delay: 1s
    - event: close
      delay: 500ms
      data: '{"status":"done"}'
```

```bash
curl -N http://localhost:8080/api/stream/42
```

`-N` disables curl's buffering so you see each event as it arrives.

## Sequenced responses

A `sequence:` block returns a different response on each successive request —
handy for polling flows where the state advances (`pending` → `running` →
`done`). Each item is an inline response (`status`/`headers`/`body`-or-`file`),
and the block replaces `response`/`conditions` for the route.

```yaml
- path: /api/poll
  method: POST
  repeat: true            # loop back to the first item after the last
  sequence:
    - { status: 202, body: '{"state":"pending"}' }
    - { status: 202, body: '{"state":"running"}' }
    - { status: 200, body: '{"state":"done"}' }
```

```bash
curl -X POST localhost:8080/api/poll   # {"state":"pending"}
curl -X POST localhost:8080/api/poll   # {"state":"running"}
curl -X POST localhost:8080/api/poll   # {"state":"done"}
```

Each request advances one step. With `repeat: true` the cycle loops back to the
first item; with `repeat` omitted (the default) the sequence advances to the last
item and **sticks** there for every later request. The position is per-route and
resets on hot-reload.

::: tip
<span v-pre>`{{seq}}`</span> is a separate counter from the sequence position, so
rendering it in a body never skews which item is served next.
:::

## Weighted-random responses

A `random:` block picks one arm per request with probability
`weight / sum(weights)` — useful for chaos-testing a client against a realistic
mix of success and error responses. Each `weight` is a positive integer.

```yaml
- path: /api/flaky
  method: GET
  random:
    - weight: 70          # ~70% success
      response: { status: 200, body: '{"result":"ok"}' }
    - weight: 20          # ~20% server error
      response: { status: 500, body: '{"error":"random failure"}' }
    - weight: 10          # ~10% unavailable
      response: { status: 503, body: '{"error":"service unavailable"}' }
```

```bash
# Repeat to see the mix; status codes vary by weight
for i in $(seq 10); do curl -s -o /dev/null -w "%{http_code} " localhost:8080/api/flaky; done
# 200 200 500 200 503 200 200 200 500 200
```

::: warning One response engine per route
`sequence`, `random`, `sse`, and `conditions`/`response` are mutually exclusive —
a route uses at most one. (`websocket` is not yet supported; it's ignored if
present.)
:::

## Settings and chaos

The `settings:` block (overridden by explicitly-set CLI flags) controls global
behavior:

```yaml
settings:
  latency: 0           # fixed latency on every response
  latency_jitter: 0    # random jitter added to latency
  fail_rate: 0         # percentage of requests to fail (0-100)
  fail_status: 500     # status used for injected failures
  cors: false
  fallback:
    type: "404"        # "404" or "proxy"
    proxy_target: ""   # backend for fallback: proxy
```

A `fallback.type: proxy` forwards unmatched requests to `proxy_target`, letting
you mock a few routes and pass the rest through to a real backend.

::: tip
An explicit `cors: false` or `fail_rate: 0` in the file is honored as written —
an explicit zero/false is distinct from an omitted field.
:::

## Hot-reload

`--watch` reloads the routes file when it changes on disk. On save, these take
effect immediately: routes, the fallback, and the global latency/fail-rate
settings. A broken edit is rejected and the previous good config keeps serving.

```bash
radix mock --routes routes.yml --watch
```

::: warning Reload caveats
CORS is applied once at startup and is **not** hot-reloaded. The <span v-pre>`{{seq}}`</span>
per-route counter resets to 1 on every reload. Explicitly-set CLI flags always
win over the file, including after a reload.
:::
