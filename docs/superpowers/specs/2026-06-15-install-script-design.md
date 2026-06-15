# Install Script Design

**Date**: 2026-06-15
**Feature**: `scripts/install.sh` — one-liner binary installer for Radix
**Status**: Approved for implementation

---

## Goal

Let users install Radix without the Go toolchain:

```bash
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash
```

---

## Section 1: Platform Detection

Detects OS (`uname -s`) and arch (`uname -m`), mapping to GoReleaser archive names:

| OS      | Arch    | Archive suffix          |
|---------|---------|-------------------------|
| macOS   | arm64   | `darwin_arm64.tar.gz`   |
| macOS   | x86_64  | `darwin_x86_64.tar.gz`  |
| Linux   | x86_64  | `linux_x86_64.tar.gz`   |
| Linux   | aarch64 | `linux_arm64.tar.gz`    |

Windows is explicitly unsupported — the `curl | bash` pattern does not apply there. The script fails fast with a clear, human-readable error for any unrecognised OS or arch.

---

## Section 2: Release Resolution

Fetches the latest release tag from the GitHub API (no auth required for public repos):

```
https://api.github.com/repos/osuritz/radix/releases/latest
```

Extracts `tag_name` using only `grep` and `sed` — no `jq` dependency. The resolved version is printed before downloading:

```
→ Installing radix v0.7.1 (darwin/arm64)
```

**Version pinning**: set `RADIX_VERSION=v0.7.0` to install a specific release instead of latest.

---

## Section 3: Download & Checksum Verification

Downloads into a temp directory (cleaned up via `trap EXIT`):

1. `radix_<ver>_<os>_<arch>.tar.gz` — the platform archive from the GitHub release
2. `checksums.txt` — GoReleaser-generated SHA256 manifest from the same release

Verifies the archive against `checksums.txt` using:
- `shasum -a 256` on macOS
- `sha256sum` on Linux

**Fail-closed**: if verification fails, or neither tool is found, the script aborts before touching anything on the user's system.

---

## Section 4: Install Location

Default install path: `~/.radix/bin/radix`

Override via env var:

```bash
RADIX_INSTALL_DIR=/usr/local/bin curl -fsSL ... | bash
```

The directory is created if it doesn't exist. On upgrade, the binary is replaced atomically: extracted to a temp file, then `mv`-ed into place, so the old binary is never half-overwritten while in use.

---

## Section 5: PATH Setup

After installing, the script checks whether the install directory is already on `$PATH`. If not:

- Appends `export PATH="$HOME/.radix/bin:$PATH"  # added by radix installer` to:
  - `~/.zshrc` if `$SHELL` contains `zsh`
  - `~/.bashrc` if it exists, otherwise `~/.profile`
- Prints a notice:
  ```
  → Added ~/.radix/bin to PATH in ~/.zshrc
    Restart your shell or run: source ~/.zshrc
  ```

Shell file modification only happens for the default `~/.radix/bin` install path. If `RADIX_INSTALL_DIR` is set (to any value), the PATH step is skipped entirely and the script prints:

```
→ Installed to /your/custom/dir/radix
  Make sure /your/custom/dir is on your PATH.
```

---

## Non-Goals

- No uninstall flag (can be added later)
- No version-pin flag (env var override is sufficient for now)
- No fish shell support
- No Windows support
- No SSH signature verification (SHA256 checksum is sufficient for a dev tool)

---

## Files Changed

| File | Change |
|------|--------|
| `scripts/install.sh` | New file |
| `README.md` | Update installation section to feature the one-liner |

---

## Testing

Manual smoke test on macOS (arm64) and Linux (x86_64):
1. Run the one-liner against a real release and verify the binary executes (`radix version`)
2. Re-run the script to verify idempotent upgrade behaviour
3. Run with `RADIX_VERSION=<older-tag>` and verify version pinning works
4. Run with `RADIX_INSTALL_DIR=/tmp/test-radix` and verify the PATH step is skipped (already-on-PATH case)
5. Corrupt the downloaded archive and verify the script aborts before installing
