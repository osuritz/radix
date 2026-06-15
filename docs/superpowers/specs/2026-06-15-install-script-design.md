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

The script itself is fetched over HTTPS from a trusted, public GitHub URL. Users who want to audit it first can download and inspect before running:

```bash
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh -o install.sh
# inspect install.sh
bash install.sh
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

**Version vs tag**: GoReleaser strips the leading `v` from `.Version`, so a tag `v0.7.1` produces archive names like `radix_0.7.1_darwin_arm64.tar.gz` (no `v`). The script stores the full tag (e.g. `v0.7.1`) for the download URL, and strips the `v` when constructing the filename.

---

## Section 3: Download & Checksum Verification

All files are downloaded into a temp directory (cleaned up via `trap EXIT`). Both the archive and `checksums.txt` are fetched from the **resolved tag URL** — never from `/releases/latest/download/` — to avoid a TOCTOU race if a new release lands between the two fetches:

```
https://github.com/osuritz/radix/releases/download/<tag>/radix_<ver>_<os>_<arch>.tar.gz
https://github.com/osuritz/radix/releases/download/<tag>/checksums.txt
```

`curl` is required (fail if absent). Downloads use `curl -fsSL` so HTTP errors (404, 403, rate limit) surface as failures rather than silently writing an error page to disk.

Verifies the archive against `checksums.txt` using:
- `shasum -a 256` on macOS
- `sha256sum` on Linux

The verification step asserts **exactly one line** in `checksums.txt` matches the expected filename — zero matches (entry missing) or multiple matches both abort with a clear error.

**Fail-closed**: if verification fails, the checksum tool is not found, or any download step errors, the script aborts before touching anything on the user's system.

Extraction targets only the `radix` binary member into the temp dir (not the whole archive), and confirms the extracted file is executable before proceeding to install.

---

## Section 4: Install Location

Default install path: `~/.radix/bin/radix`

Override via env var — must be exported before the pipe so `bash` sees it:

```bash
# Correct forms:
export RADIX_INSTALL_DIR=/usr/local/bin
curl -fsSL ... | bash

# or inline:
curl -fsSL ... | RADIX_INSTALL_DIR=/usr/local/bin bash
```

The directory is created if it doesn't exist. If the directory is not writable (e.g. `/usr/local/bin` without `sudo`), the script fails with a clear message rather than silently skipping or attempting `sudo`.

On upgrade, the binary is replaced atomically: extracted to a temp file, then `mv`-ed into place, so the old binary is never half-overwritten while in use.

---

## Section 5: PATH Setup

After installing, the script checks whether the install directory is already on `$PATH` (comparing both `$HOME`-relative and fully-expanded paths to avoid false duplicates). If not present:

- Appends the following to the appropriate shell startup file, using **single quotes** so `$HOME` is expanded at shell startup, not at install time:
  ```sh
  export PATH='$HOME/.radix/bin':$PATH  # added by radix installer
  ```
  Target file:
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
- No `sudo` fallback (fail clearly instead)

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
4. Run with `RADIX_INSTALL_DIR=/tmp/test-radix` and verify the PATH step is skipped entirely with a clear hint
5. Run with `RADIX_INSTALL_DIR=/usr/local/bin` as a non-root user and verify the script fails with a clear "not writable" error
6. Corrupt the downloaded archive and verify the script aborts before installing
7. Remove one entry from `checksums.txt` and verify the script aborts on zero matches
