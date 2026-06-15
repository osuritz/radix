# Logging

Every command logs each request through an access-log middleware. Three formats
are available; `dev` is the default.

## Dev format (default)

A single-line, color-coded format tuned for reading at a glance. Columns, in
order: a dimmed short timestamp (`HH:MM:SS`), the method (color-coded, padded so
paths align), the path (padded; overly long paths truncated with a single `…`),
an optional dimmed `→ target` column, the status code (color-coded), the
latency, and — only when the response had a body — a human-readable size.

```
14:23:01 GET     /index.html                  200 12ms 2.3KB
14:23:01 POST    /api/users                   201 8ms 142B
14:23:01 DELETE  /users/123                    204 5ms
```

A zero-size response (like the `204` above) omits the size column.

### The `→ target` column

The arrow tells the request story left to right —
`METHOD /path → target STATUS latency [size]` — and appears only when it's
meaningful:

- **`radix proxy`** shows the upstream the request was forwarded to (e.g.
  `localhost:3000`).
- **`radix serve --spa`** shows `fallback` only when a request is served the SPA
  index because the path didn't exist. Real files get no target column.

```
14:23:01 GET     /api/users                   → localhost:3000 200 12ms 2.3KB
14:23:01 GET     /dashboard                   → fallback 200 3ms 1.0KB
```

When no target applies, the line is just the layout above — no arrow, no extra
spacing.

## CLF and Extended CLF

For ingestion by standard log tooling:

- **`clf`** — Common Log Format.
- **`extended_clf`** — Common Log Format plus referrer and user-agent. This is
  what `--verbose` (`-v`) selects.

Both CLF formats are byte-for-byte the classic layouts and are unaffected by the
color settings below.

```bash
radix serve --verbose    # extended_clf
```

## Color control

Color for the `dev` format is decided once at startup, first match wins:

1. `--no-color` (or `no_color` in config) → color **off**.
2. else `NO_COLOR` is set and non-empty → color **off**.
3. else `FORCE_COLOR` or `CLICOLOR_FORCE` is set and non-empty → color **on**
   (only overrides the TTY check below; it can't re-enable color past steps 1–2).
4. else the output is not a TTY (redirected to a file or pipe) → color **off**.
5. otherwise → color **on**.

```bash
radix serve --no-color        # force plain output
NO_COLOR=1 radix serve        # same, via environment
FORCE_COLOR=1 radix serve | tee log.txt   # keep color through a pipe
```

::: warning
TTY detection is a stdlib-only character-device heuristic, not a true `isatty`:
character devices like `/dev/null` read as a TTY. For those edge cases, set the
overrides above explicitly.
:::
