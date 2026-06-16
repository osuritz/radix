# Getting started

Install radix and have a server running in under a minute.

## Install

### One-liner (macOS and Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash
```

Installs the latest release to `~/.radix/bin` and adds it to your PATH. No Go
toolchain required. To pin a version or install to a custom directory:

```bash
# Pin a specific release
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | RADIX_VERSION=v0.7.1 bash

# Install to a custom directory (PATH step is skipped — you manage PATH)
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | RADIX_INSTALL_DIR=/usr/local/bin bash
```

### go install

If you have Go 1.25+ installed:

```bash
go install github.com/osuritz/radix/cmd/radix@latest
```

This drops the `radix` binary in `$(go env GOPATH)/bin`. Make sure that's on
your `PATH`.

### Prebuilt binary

Grab a release archive from the
[releases page](https://github.com/osuritz/radix/releases) — `.tar.gz` for
Linux/macOS, `.zip` for Windows — plus `checksums.txt` to verify it.

```bash
# Linux/macOS — replace the version, OS, and arch to match the asset you downloaded
tar -xzf radix_0.7.1_darwin_arm64.tar.gz
chmod +x radix
sudo mv radix /usr/local/bin/    # or anywhere on your PATH

radix version
```

On Windows, unzip the `.zip` and put `radix.exe` somewhere on your `PATH`.

::: tip Planned
Homebrew and Scoop packaging are on the roadmap.
:::

## Your first `serve`

Serve the current directory as static files:

```bash
radix serve .
```

That's live at `http://localhost:8080`. Drop an
`index.html` in the directory and reload. Building a single-page app? Add
`--spa` so unknown paths fall back to `index.html`:

```bash
radix serve ./dist --spa
```

## Your first `proxy`

Point radix at a backend and it forwards everything to it — handy for putting a
TLS or CORS layer in front of a service that has neither:

```bash
radix proxy http://localhost:3000
```

Requests to `http://localhost:8080` now go to your backend on port 3000, with
`X-Forwarded-*` headers set for you.

## What you get for free

Whatever mode you run, radix also starts an admin server on `127.0.0.1:9090`:

```bash
curl http://127.0.0.1:9090/healthz    # {"status":"ok",...}
curl http://127.0.0.1:9090/_metrics   # request metrics as JSON
```

It binds loopback only, so metrics and health never leak even when the app port
binds `0.0.0.0`. See [Observability](/guides/observability) for the details.

## Next steps

- [Commands](/commands/serve) — every mode, flag by flag
- [Configuration](/configuration) — the `radix.yml` file and env overrides
- [Mocking guide](/guides/mock) — custom routes, templating, SSE
- [TLS guide](/guides/tls) — local HTTPS and mTLS
- [Troubleshooting](/troubleshooting) — common errors and fixes
