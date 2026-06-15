# radix version

Print version, commit, build date, and builder information.

```bash
radix version [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--short` | Print only the version number |
| `--json` | Print as JSON |

## Output

```bash
radix version
```

```
radix v0.6.0
commit: a1b2c3d
built at: 2026-06-15
built by: goreleaser
goos: darwin
goarch: arm64
compiler: go1.25.0
```

The fields are filled in at build time (values above are illustrative). A
released binary shows a tag version and `built by: goreleaser`; a `go install`
or local `make build` shows `dev` for the version and `unknown` for the builder.

### Just the number

```bash
radix version --short
```

### JSON

For scripts and tooling:

```bash
radix version --json
```
