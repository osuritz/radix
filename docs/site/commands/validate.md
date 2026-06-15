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

## What is checked

For a main config (`radix.yml`):

- YAML/JSON syntax and schema
- Port ranges and referenced file paths
- `metrics.port` is `1..65535` and differs from the app `port`
- `metrics.path` is non-empty, starts with `/`, and isn't the reserved `/healthz`
- `serve.hsts` and `serve.http_redirect` both require `tls.enabled`
- `serve.http_port` differs from `port` when `http_redirect` is set
- `serve.hsts_max_age` is not negative (`0` clears the policy)

For a mock-routes file (`--type mock-routes`): route paths, methods, regex
patterns, templates, and conditions.

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
radix validate ./mock-routes.yml --type mock-routes
```

Auto-detection usually works, but `--type` forces it when the filename is
ambiguous.
