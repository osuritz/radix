# radix validate

Check a config or routes file for syntax and correctness without starting a
server.

```bash
radix validate [config-file] [flags]
```

With no argument it validates `./radix.yml`.

## When to use it

Catch a bad config before you ship it — in CI, a pre-commit hook, or just before
running. It checks more than YAML syntax: it enforces the same rules radix
applies at startup.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--strict` | `false` | Fail on warnings, not just errors |
| `--type` | `auto` | Config type: `main`, `mock-routes`, or `auto`-detect |

In `auto` mode a file whose top level contains a `routes` key (the mock-routes
schema) is validated as a mock-routes file; anything else — including a file
with only a top-level `settings:` key, which could equally be a main config
carrying a stray block — is validated as a main config. Use
`--type mock-routes` to validate a settings-only routes file. Any other
`--type` value is an error.

## What is checked

For a main config (`radix.yml`):

- YAML/JSON syntax and schema
- Port ranges and referenced file paths
- `metrics.port` is `1..65535` and differs from the app `port`
- `metrics.path` is non-empty, starts with `/`, and isn't the reserved `/healthz`
- `serve.hsts` and `serve.http_redirect` both require `tls.enabled`
- `serve.http_port` differs from `port` when `http_redirect` is set
- `serve.hsts_max_age` is not negative (`0` clears the policy)

For a mock-routes file (auto-detected or `--type mock-routes`): the file is
compiled with the same loader `radix mock` uses, so route paths, methods,
regex patterns, response templates, conditions, sse/sequence/random selectors,
and the `settings:` block are all checked. On success the number of compiled
routes is reported; a file that compiles but defines no routes is a warning
(fatal under `--strict`).

## Examples

### Validate the default config

```bash
radix validate
```

### Validate a specific file, strict

```bash
radix validate ./radix.yml --strict
```

`--strict` turns warnings into failures — useful in CI.

### Validate a routes file

```bash
radix validate ./mock-routes.yml                     # auto-detected
radix validate ./routes.yml --type mock-routes       # forced
```

Auto-detection keys off the file's top-level `routes` key, so it usually just
works; `--type mock-routes` forces the mode when the content is ambiguous
(e.g. an empty skeleton file or a settings-only file).

```
Validating configuration: /path/to/mock-routes.yml

✓ Syntax: OK
✓ Routes: 15 compiled

✓ Mock routes file is valid: /path/to/mock-routes.yml
```
