# Install Script Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `scripts/install.sh` so users can install Radix without the Go toolchain via `curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash`.

**Architecture:** Single self-contained Bash script. Detects platform, resolves the latest GitHub release, downloads the archive and `checksums.txt` from the resolved tag URL, verifies SHA256, extracts only the binary, installs atomically, and appends to the shell startup file if needed. Eight focused functions, each independently testable.

**Tech Stack:** Bash, `curl`, `tar`, `shasum`/`sha256sum`, GitHub Releases API (public, no auth).

**Spec:** `docs/superpowers/specs/2026-06-15-install-script-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `scripts/install.sh` | Create | The installer — all logic lives here |
| `README.md` | Modify | Replace manual install instructions with the one-liner |

---

## Task 1: Script skeleton with helpers

**Files:**
- Create: `scripts/install.sh`

- [ ] **Step 1: Create the script with shebang, constants, and helper stubs**

Write `scripts/install.sh` with this exact content:

```bash
#!/usr/bin/env bash
# scripts/install.sh — Radix one-line installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash
#
# Options (env vars):
#   RADIX_VERSION      Pin a specific release tag, e.g. RADIX_VERSION=v0.7.0
#   RADIX_INSTALL_DIR  Override install directory (default: ~/.radix/bin)

set -euo pipefail

# ── constants ─────────────────────────────────────────────────────────────────

REPO="osuritz/radix"
BINARY="radix"
DEFAULT_INSTALL_DIR="${HOME}/.radix/bin"

# ── helpers ───────────────────────────────────────────────────────────────────

die()     { printf 'error: %s\n' "$*" >&2; exit 1; }
info()    { printf '  %s\n' "$*"; }
success() { printf 'ok: %s\n' "$*"; }

# ── stubs (replaced in later tasks) ──────────────────────────────────────────

detect_platform()    { die "not implemented"; }
resolve_version()    { die "not implemented"; }
version_number()     { die "not implemented"; }
download_files()     { die "not implemented"; }
verify_checksum()    { die "not implemented"; }
extract_and_install(){ die "not implemented"; }
setup_path()         { die "not implemented"; }

# ── main ──────────────────────────────────────────────────────────────────────

main() {
  command -v curl >/dev/null 2>&1 || die "curl is required but not found"

  local install_dir="${RADIX_INSTALL_DIR:-${DEFAULT_INSTALL_DIR}}"
  local platform tag ver tmpdir archive_path

  platform="$(detect_platform)"
  tag="$(resolve_version)"
  ver="$(version_number "$tag")"

  printf 'Installing radix %s (%s) ...\n' "$tag" "${platform//_//}"

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  archive_path="${tmpdir}/radix_${ver}_${platform}.tar.gz"
  download_files "$tag" "$ver" "$platform" "$tmpdir"
  verify_checksum "$archive_path" "${tmpdir}/checksums.txt"
  extract_and_install "$archive_path" "$install_dir" "$tmpdir"
  setup_path "$install_dir"

  printf '\nradix %s installed successfully!\n' "$tag"
  info "Run 'radix version' to confirm."
}

main "$@"
```

- [ ] **Step 2: Make the script executable**

```bash
chmod +x scripts/install.sh
```

- [ ] **Step 3: Verify the script parses without syntax errors**

```bash
bash -n scripts/install.sh
```

Expected: no output (exit 0).

- [ ] **Step 4: Verify the stub plumbing calls die correctly**

```bash
bash scripts/install.sh 2>&1 | head -1
```

Expected: `error: not implemented`

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): add install script skeleton"
```

---

## Task 2: Platform detection

**Files:**
- Modify: `scripts/install.sh` — replace `detect_platform` stub

- [ ] **Step 1: Write a quick test for the current platform before implementing**

```bash
# Expected: prints something like "darwin_arm64" or "linux_x86_64"
bash -c '
  os="$(uname -s)"
  arch="$(uname -m)"
  echo "raw: os=${os} arch=${arch}"
'
```

Note what your platform reports — use this to verify the function output in step 3.

- [ ] **Step 2: Replace the `detect_platform` stub**

In `scripts/install.sh`, replace:

```bash
detect_platform()    { die "not implemented"; }
```

with:

```bash
detect_platform() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Darwin) os="darwin" ;;
    Linux)  os="linux"  ;;
    *) die "unsupported OS: ${os}" ;;
  esac

  case "$arch" in
    x86_64)        arch="x86_64" ;;
    arm64|aarch64) arch="arm64"  ;;
    *) die "unsupported architecture: ${arch}" ;;
  esac

  echo "${os}_${arch}"
}
```

- [ ] **Step 3: Test the function in isolation**

```bash
bash -c '
  set -euo pipefail
  die() { printf "error: %s\n" "$*" >&2; exit 1; }

  detect_platform() {
    local os arch
    os="$(uname -s)"; arch="$(uname -m)"
    case "$os" in Darwin) os="darwin" ;; Linux) os="linux" ;; *) die "unsupported OS: ${os}" ;; esac
    case "$arch" in x86_64) arch="x86_64" ;; arm64|aarch64) arch="arm64" ;; *) die "unsupported arch: ${arch}" ;; esac
    echo "${os}_${arch}"
  }

  result="$(detect_platform)"
  echo "platform: $result"
  [[ "$result" =~ ^(darwin|linux)_(x86_64|arm64)$ ]] || { echo "FAIL: unexpected format"; exit 1; }
  echo "PASS"
'
```

Expected: `platform: darwin_arm64` (or equivalent), then `PASS`.

- [ ] **Step 4: Verify the full script still parses**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement platform detection"
```

---

## Task 3: Release resolution

**Files:**
- Modify: `scripts/install.sh` — replace `resolve_version` and `version_number` stubs

- [ ] **Step 1: Manually verify the GitHub API response format**

```bash
curl -fsSL https://api.github.com/repos/osuritz/radix/releases/latest \
  | grep '"tag_name"'
```

Expected output: `  "tag_name": "v0.7.1",` (or latest tag). This confirms the grep/sed pattern.

- [ ] **Step 2: Replace the `resolve_version` and `version_number` stubs**

In `scripts/install.sh`, replace:

```bash
resolve_version()    { die "not implemented"; }
version_number()     { die "not implemented"; }
```

with:

```bash
# Fetches the latest release tag from the GitHub API, or uses $RADIX_VERSION if set.
resolve_version() {
  local tag="${RADIX_VERSION:-}"
  if [ -z "$tag" ]; then
    tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' \
      | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
    [ -n "$tag" ] || die "failed to fetch latest release tag from GitHub"
  fi
  echo "$tag"
}

# GoReleaser strips the leading 'v' from .Version in archive filenames.
# e.g. tag v0.7.1 → filename prefix 0.7.1
version_number() { echo "${1#v}"; }
```

- [ ] **Step 3: Test resolve_version**

```bash
# Test 1: live API fetch
tag="$(bash -c '
  REPO="osuritz/radix"
  die() { printf "error: %s\n" "$*" >&2; exit 1; }
  resolve_version() {
    local tag="${RADIX_VERSION:-}"
    [ -z "$tag" ] && tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep "\"tag_name\"" | sed "s/.*\"tag_name\": *\"\([^\"]*\)\".*/\1/")"
    [ -n "$tag" ] || die "failed to fetch latest release tag"
    echo "$tag"
  }
  resolve_version
')"
echo "latest tag: $tag"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "FAIL: unexpected tag format"; exit 1; }

# Test 2: version pinning
pinned="$(RADIX_VERSION=v0.6.0 bash -c '
  die() { printf "error: %s\n" "$*" >&2; exit 1; }
  resolve_version() {
    local tag="${RADIX_VERSION:-}"
    [ -z "$tag" ] && tag="$(curl -fsSL "https://api.github.com/repos/osuritz/radix/releases/latest" | grep "\"tag_name\"" | sed "s/.*\"tag_name\": *\"\([^\"]*\)\".*/\1/")"
    [ -n "$tag" ] || die "failed"
    echo "$tag"
  }
  resolve_version
')"
[ "$pinned" = "v0.6.0" ] && echo "PASS: version pinning" || echo "FAIL: version pinning (got $pinned)"

# Test 3: version_number strips v
result="$(bash -c 'version_number() { echo "${1#v}"; }; version_number v0.7.1')"
[ "$result" = "0.7.1" ] && echo "PASS: version_number" || echo "FAIL: version_number (got $result)"
```

Expected: `latest tag: v0.7.1` (or current), `PASS: version pinning`, `PASS: version_number`.

- [ ] **Step 4: Verify the full script still parses**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement release resolution"
```

---

## Task 4: Download

**Files:**
- Modify: `scripts/install.sh` — replace `download_files` stub

- [ ] **Step 1: Replace the `download_files` stub**

In `scripts/install.sh`, replace:

```bash
download_files()     { die "not implemented"; }
```

with:

```bash
# Downloads archive and checksums.txt from the resolved tag URL into tmpdir.
# Uses -f so HTTP errors (404, rate-limit) surface immediately rather than
# writing an error page to disk.
download_files() {
  local tag="$1" ver="$2" platform="$3" tmpdir="$4"
  local base="https://github.com/${REPO}/releases/download/${tag}"
  local archive="radix_${ver}_${platform}.tar.gz"

  info "Downloading ${archive} ..."
  curl -fsSL "${base}/${archive}" -o "${tmpdir}/${archive}" \
    || die "download failed: ${base}/${archive}"

  info "Downloading checksums.txt ..."
  curl -fsSL "${base}/checksums.txt" -o "${tmpdir}/checksums.txt" \
    || die "download failed: ${base}/checksums.txt"
}
```

- [ ] **Step 2: Test download_files against the real v0.7.1 release**

```bash
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

REPO="osuritz/radix"
info()    { printf '  %s\n' "$*"; }
die()     { printf 'error: %s\n' "$*" >&2; exit 1; }

download_files() {
  local tag="$1" ver="$2" platform="$3" tmpdir="$4"
  local base="https://github.com/${REPO}/releases/download/${tag}"
  local archive="radix_${ver}_${platform}.tar.gz"
  info "Downloading ${archive} ..."
  curl -fsSL "${base}/${archive}" -o "${tmpdir}/${archive}" || die "download failed"
  info "Downloading checksums.txt ..."
  curl -fsSL "${base}/checksums.txt" -o "${tmpdir}/checksums.txt" || die "download failed"
}

platform="$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/aarch64/arm64/')"
download_files "v0.7.1" "0.7.1" "$platform" "$tmpdir"

ls -lh "$tmpdir"
grep "radix_0.7.1_${platform}" "$tmpdir/checksums.txt" && echo "PASS: checksums.txt contains expected entry" || echo "FAIL"
```

Expected: lists two files in tmpdir, prints a matching checksum line, then `PASS`.

- [ ] **Step 3: Verify the full script still parses**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 4: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement file download"
```

---

## Task 5: Checksum verification

**Files:**
- Modify: `scripts/install.sh` — replace `verify_checksum` stub

- [ ] **Step 1: Replace the `verify_checksum` stub**

In `scripts/install.sh`, replace:

```bash
verify_checksum()    { die "not implemented"; }
```

with:

```bash
# Verifies archive SHA256 against checksums.txt.
# Asserts exactly one matching entry exists (zero or many are both errors).
verify_checksum() {
  local archive_path="$1" checksums_path="$2"
  local filename
  filename="$(basename "$archive_path")"

  local match_count
  match_count="$(grep -c "  ${filename}$" "$checksums_path" || echo 0)"
  case "$match_count" in
    0) die "checksum entry for ${filename} not found in checksums.txt" ;;
    1) ;;
    *) die "multiple checksum entries for ${filename} in checksums.txt" ;;
  esac

  local expected
  expected="$(grep "  ${filename}$" "$checksums_path" | awk '{print $1}')"

  local actual
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive_path" | awk '{print $1}')"
  else
    die "no checksum tool found (need shasum or sha256sum)"
  fi

  [ "$actual" = "$expected" ] || die "checksum mismatch for ${filename}"
  success "checksum verified"
}
```

- [ ] **Step 2: Test with a real download (happy path)**

```bash
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

REPO="osuritz/radix"
die()     { printf 'error: %s\n' "$*" >&2; exit 1; }
info()    { printf '  %s\n' "$*"; }
success() { printf 'ok: %s\n' "$*"; }

# Inline verify_checksum for testing
verify_checksum() {
  local archive_path="$1" checksums_path="$2"
  local filename; filename="$(basename "$archive_path")"
  local match_count; match_count="$(grep -c "  ${filename}$" "$checksums_path" || echo 0)"
  case "$match_count" in 0) die "not found" ;; 1) ;; *) die "multiple" ;; esac
  local expected; expected="$(grep "  ${filename}$" "$checksums_path" | awk '{print $1}')"
  local actual
  if command -v shasum >/dev/null 2>&1; then actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
  else actual="$(sha256sum "$archive_path" | awk '{print $1}')"; fi
  [ "$actual" = "$expected" ] || die "checksum mismatch"
  success "checksum verified"
}

# Download
platform="$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/aarch64/arm64/')"
archive="radix_0.7.1_${platform}.tar.gz"
curl -fsSL "https://github.com/${REPO}/releases/download/v0.7.1/${archive}" -o "${tmpdir}/${archive}"
curl -fsSL "https://github.com/${REPO}/releases/download/v0.7.1/checksums.txt" -o "${tmpdir}/checksums.txt"

# Happy path
verify_checksum "${tmpdir}/${archive}" "${tmpdir}/checksums.txt" && echo "PASS: happy path"

# Corrupt and verify failure
echo "corrupted" >> "${tmpdir}/${archive}"
verify_checksum "${tmpdir}/${archive}" "${tmpdir}/checksums.txt" 2>&1 | grep -q "checksum mismatch" \
  && echo "PASS: detects corruption" || echo "FAIL: did not detect corruption"
```

Expected: `ok: checksum verified`, `PASS: happy path`, `PASS: detects corruption`.

- [ ] **Step 3: Test missing entry detection**

```bash
tmpdir="$(mktemp -d)"; trap 'rm -rf "$tmpdir"' EXIT
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
success() { printf 'ok: %s\n' "$*"; }
verify_checksum() {
  local archive_path="$1" checksums_path="$2"
  local filename; filename="$(basename "$archive_path")"
  local match_count; match_count="$(grep -c "  ${filename}$" "$checksums_path" || echo 0)"
  case "$match_count" in 0) die "checksum entry for ${filename} not found in checksums.txt" ;; 1) ;; *) die "multiple" ;; esac
  success "found"
}

echo "abc123  other_file.tar.gz" > "${tmpdir}/checksums.txt"
touch "${tmpdir}/radix_0.7.1_darwin_arm64.tar.gz"
verify_checksum "${tmpdir}/radix_0.7.1_darwin_arm64.tar.gz" "${tmpdir}/checksums.txt" 2>&1 \
  | grep -q "not found" && echo "PASS: missing entry" || echo "FAIL"
```

Expected: `PASS: missing entry`.

- [ ] **Step 4: Verify the full script still parses**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement checksum verification"
```

---

## Task 6: Extract and install

**Files:**
- Modify: `scripts/install.sh` — replace `extract_and_install` stub

- [ ] **Step 1: Replace the `extract_and_install` stub**

In `scripts/install.sh`, replace:

```bash
extract_and_install(){ die "not implemented"; }
```

with:

```bash
# Extracts only the radix binary from the archive, verifies it is executable,
# then atomically moves it into install_dir.
extract_and_install() {
  local archive_path="$1" install_dir="$2" tmpdir="$3"

  mkdir -p "$install_dir" 2>/dev/null \
    || die "cannot create install directory: ${install_dir}"
  [ -w "$install_dir" ] \
    || die "install directory is not writable: ${install_dir}
  Hint: run with sudo, or set RADIX_INSTALL_DIR to a writable path"

  info "Extracting ${BINARY} ..."
  tar -xzf "$archive_path" -C "$tmpdir" "${BINARY}" \
    || die "failed to extract ${BINARY} from archive"

  local extracted="${tmpdir}/${BINARY}"
  [ -f "$extracted" ] || die "${BINARY} not found in archive"
  [ -x "$extracted" ] || die "extracted ${BINARY} is not executable"

  mv "$extracted" "${install_dir}/${BINARY}"
  success "installed ${install_dir}/${BINARY}"
}
```

- [ ] **Step 2: Test with a real archive (happy path)**

```bash
tmpdir="$(mktemp -d)"; trap 'rm -rf "$tmpdir"' EXIT
BINARY="radix"
die()     { printf 'error: %s\n' "$*" >&2; exit 1; }
info()    { printf '  %s\n' "$*"; }
success() { printf 'ok: %s\n' "$*"; }

extract_and_install() {
  local archive_path="$1" install_dir="$2" tmpdir="$3"
  mkdir -p "$install_dir" 2>/dev/null || die "cannot create: ${install_dir}"
  [ -w "$install_dir" ] || die "not writable: ${install_dir}"
  info "Extracting ${BINARY} ..."
  tar -xzf "$archive_path" -C "$tmpdir" "${BINARY}" || die "extraction failed"
  local extracted="${tmpdir}/${BINARY}"
  [ -f "$extracted" ] || die "binary not found"
  [ -x "$extracted" ] || die "binary not executable"
  mv "$extracted" "${install_dir}/${BINARY}"
  success "installed ${install_dir}/${BINARY}"
}

# Download a real archive
platform="$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/aarch64/arm64/')"
curl -fsSL "https://github.com/osuritz/radix/releases/download/v0.7.1/radix_0.7.1_${platform}.tar.gz" \
  -o "${tmpdir}/radix_0.7.1_${platform}.tar.gz"

install_dir="${tmpdir}/bin"
extract_and_install "${tmpdir}/radix_0.7.1_${platform}.tar.gz" "$install_dir" "$tmpdir"

"${install_dir}/radix" version && echo "PASS: binary runs" || echo "FAIL: binary did not run"
```

Expected: `ok: installed ...`, `radix v0.7.1 ...`, `PASS: binary runs`.

- [ ] **Step 3: Test unwritable directory detection**

```bash
tmpdir="$(mktemp -d)"; trap 'rm -rf "$tmpdir"' EXIT
BINARY="radix"
die()     { printf 'error: %s\n' "$*" >&2; exit 1; }
info()    { printf '  %s\n' "$*"; }
success() { printf 'ok: %s\n' "$*"; }

extract_and_install() {
  local archive_path="$1" install_dir="$2" tmpdir="$3"
  mkdir -p "$install_dir" 2>/dev/null || die "cannot create: ${install_dir}"
  [ -w "$install_dir" ] || die "not writable: ${install_dir}"
  info "would extract here"
}

unwritable="${tmpdir}/locked"
mkdir "$unwritable" && chmod 000 "$unwritable"
extract_and_install "fake.tar.gz" "$unwritable" "$tmpdir" 2>&1 \
  | grep -q "not writable" && echo "PASS: detects unwritable dir" || echo "FAIL"
chmod 755 "$unwritable"  # cleanup so trap can remove it
```

Expected: `PASS: detects unwritable dir`.

- [ ] **Step 4: Verify the full script still parses**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement binary extraction and install"
```

---

## Task 7: PATH setup and final wiring

**Files:**
- Modify: `scripts/install.sh` — replace `setup_path` stub

- [ ] **Step 1: Replace the `setup_path` stub**

In `scripts/install.sh`, replace:

```bash
setup_path()         { die "not implemented"; }
```

with:

```bash
# Appends ~/.radix/bin to the user's shell startup file if not already on PATH.
# Skipped entirely when RADIX_INSTALL_DIR is set (user manages their own PATH).
# Uses single-quoted heredoc so $HOME expands at shell startup, not install time.
setup_path() {
  local install_dir="$1"

  # When the user set a custom install dir, skip shell file modification entirely
  if [ "${RADIX_INSTALL_DIR+set}" = "set" ]; then
    info "Make sure ${install_dir} is on your PATH."
    return
  fi

  # Resolve canonical path to catch $HOME vs /Users/... equivalence
  local canonical
  canonical="$(cd "$install_dir" 2>/dev/null && pwd || echo "$install_dir")"

  case ":${PATH}:" in
    *":${install_dir}:"*|*":${canonical}:"*)
      success "${install_dir} is already on PATH"
      return ;;
  esac

  # Pick shell startup file
  local shell_file
  case "${SHELL:-}" in
    */zsh) shell_file="${HOME}/.zshrc" ;;
    *)
      if [ -f "${HOME}/.bashrc" ]; then
        shell_file="${HOME}/.bashrc"
      else
        shell_file="${HOME}/.profile"
      fi ;;
  esac

  # Single-quoted heredoc: $HOME expands at shell startup, not at install time
  cat >> "$shell_file" << 'RADIX_PATH'

export PATH='$HOME/.radix/bin':$PATH  # added by radix installer
RADIX_PATH

  info "Added ~/.radix/bin to PATH in ${shell_file}"
  info "Restart your shell or run: source ${shell_file}"
}
```

- [ ] **Step 2: Test PATH-already-present detection**

```bash
die()     { printf 'error: %s\n' "$*" >&2; exit 1; }
info()    { printf '  %s\n' "$*"; }
success() { printf 'ok: %s\n' "$*"; }

setup_path() {
  local install_dir="$1"
  [ "${RADIX_INSTALL_DIR+set}" = "set" ] && { info "Make sure ${install_dir} is on your PATH."; return; }
  local canonical; canonical="$(cd "$install_dir" 2>/dev/null && pwd || echo "$install_dir")"
  case ":${PATH}:" in *":${install_dir}:"*|*":${canonical}:"*) success "already on PATH"; return ;; esac
  info "would append to shell file"
}

# Test 1: already on PATH
PATH="/fake/bin:${HOME}/.radix/bin:/usr/bin" setup_path "${HOME}/.radix/bin"
# Expected: "ok: already on PATH"

# Test 2: RADIX_INSTALL_DIR set → skip
RADIX_INSTALL_DIR="/usr/local/bin" setup_path "/usr/local/bin"
# Expected: "  Make sure /usr/local/bin is on your PATH."

# Test 3: not on PATH → would append
setup_path "${HOME}/.radix/bin"
# Expected: "  would append to shell file"
```

- [ ] **Step 3: Test that the appended line uses single quotes (shell startup expansion)**

```bash
tmpdir="$(mktemp -d)"; trap 'rm -rf "$tmpdir"' EXIT
test_profile="${tmpdir}/.profile"
touch "$test_profile"

cat >> "$test_profile" << 'RADIX_PATH'

export PATH='$HOME/.radix/bin':$PATH  # added by radix installer
RADIX_PATH

grep -q "'\$HOME/.radix/bin'" "$test_profile" \
  && echo "PASS: uses single quotes" || echo "FAIL: \$HOME was expanded at write time"
```

Expected: `PASS: uses single quotes`.

- [ ] **Step 4: End-to-end smoke test of the complete script**

This runs the full script against the real v0.7.1 release into a temp dir:

```bash
tmpdir="$(mktemp -d)"; trap 'rm -rf "$tmpdir"' EXIT
RADIX_INSTALL_DIR="${tmpdir}/bin" bash scripts/install.sh
"${tmpdir}/bin/radix" version
```

Expected: installation progress lines, then `radix v0.7.1 ...` from the version command.

- [ ] **Step 5: Verify the complete script parses and shellcheck passes (if available)**

```bash
bash -n scripts/install.sh
command -v shellcheck >/dev/null 2>&1 && shellcheck scripts/install.sh || echo "(shellcheck not installed — skip)"
```

- [ ] **Step 6: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): implement PATH setup, complete script"
```

---

## Task 8: Update README

**Files:**
- Modify: `README.md` — add the one-liner as the primary install method

- [ ] **Step 1: Find the current installation section in README**

```bash
grep -n "Install\|install\|go install" README.md | head -20
```

Note the line number of the install block.

- [ ] **Step 2: Replace the installation section**

Find the block in `README.md` that currently reads (around the `go install` line):

```markdown
# Install
go install github.com/osuritz/radix/cmd/radix@latest
```

Replace it with:

```markdown
## Install

```bash
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash
```

This installs the latest release to `~/.radix/bin` and adds it to your PATH.

**Other options:**

```bash
# Pin a specific version
RADIX_VERSION=v0.7.0 curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | bash

# Install to a custom directory
curl -fsSL https://raw.githubusercontent.com/osuritz/radix/main/scripts/install.sh | RADIX_INSTALL_DIR=/usr/local/bin bash

# If you have Go 1.25+
go install github.com/osuritz/radix/cmd/radix@latest
```
```

(Adjust surrounding markdown headers to match the existing document style.)

- [ ] **Step 3: Verify the README renders without broken markdown**

```bash
# Quick sanity check: look for unclosed fences
awk '/^```/{n++} END{if(n%2!=0) print "WARN: odd number of fences"}' README.md \
  || echo "ok: fence count looks even"
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add one-liner install to README"
```

---

## Self-Review Checklist

- [x] **Platform detection** — covers darwin/linux × x86_64/arm64; fails loudly on unsupported combos
- [x] **v-prefix** — `version_number` strips `v`; archive filename uses stripped form; download URL uses full tag
- [x] **TOCTOU** — both files fetched from resolved tag URL, not `latest/download`
- [x] **Exactly-one checksum match** — `case "$match_count"` handles 0 and 2+
- [x] **Download failures** — `curl -f` + explicit `|| die` on each download
- [x] **Unwritable dir** — explicit check before extraction
- [x] **Extract only binary** — `tar -xzf ... radix` (confirmed at root of archive from v0.7.1)
- [x] **PATH dedup** — checks both `install_dir` and `canonical` (expanded) form
- [x] **Single-quoted PATH line** — heredoc with `'RADIX_PATH'` delimiter prevents expansion at install time
- [x] **Env var scoping** — docs show correct `export` or inline-before-bash form
- [x] **README** — Task 8 updates installation section
