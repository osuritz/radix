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

# ── helpers ──────────────────────────────────────────────────────────────────

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
# Fetches the latest release tag from the GitHub API, or uses $RADIX_VERSION if set.
resolve_version() {
  local tag="${RADIX_VERSION:-}"
  if [ -z "$tag" ]; then
    tag="$(curl -fsSL --max-time 10 "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep -m 1 '"tag_name"' \
      | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
    [ -n "$tag" ] || die "failed to fetch latest release tag from GitHub"
  fi
  echo "$tag" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+' \
    || die "invalid release tag format: ${tag} (expected vX.Y.Z)"
  echo "$tag"
}

# GoReleaser strips the leading 'v' from .Version in archive filenames.
# e.g. tag v0.7.1 → filename prefix 0.7.1
version_number() { echo "${1#v}"; }
# Downloads archive and checksums.txt from the resolved tag URL into tmpdir.
# Uses -f so HTTP errors (404, 403, rate-limit) surface immediately rather than
# writing an error page to disk.
download_files() {
  local tag="$1" ver="$2" platform="$3" tmpdir="$4"
  local base="https://github.com/${REPO}/releases/download/${tag}"
  local archive="radix_${ver}_${platform}.tar.gz"

  info "Downloading ${archive} ..."
  curl -fsSL --max-time 60 "${base}/${archive}" -o "${tmpdir}/${archive}" \
    || die "download failed: ${base}/${archive}"

  info "Downloading checksums.txt ..."
  curl -fsSL --max-time 30 "${base}/checksums.txt" -o "${tmpdir}/checksums.txt" \
    || die "download failed: ${base}/checksums.txt"
}
# Verifies archive SHA256 against checksums.txt.
# Asserts exactly one matching entry exists (zero or many are both errors).
verify_checksum() {
  local archive_path="$1" checksums_path="$2"
  local filename
  filename="$(basename "$archive_path")"

  local match_count
  match_count=$(grep -cF "  ${filename}" "$checksums_path") || match_count=0
  case "$match_count" in
    0) die "checksum entry for ${filename} not found in checksums.txt" ;;
    1) ;;
    *) die "multiple checksum entries for ${filename} in checksums.txt" ;;
  esac

  local expected
  expected="$(grep -F "  ${filename}" "$checksums_path" | awk '{print $1}')"

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
# Extracts only the radix binary from the archive, verifies it is executable,
# then atomically moves it into install_dir.
extract_and_install() {
  local archive_path="$1" install_dir="$2" tmpdir="$3"

  mkdir -p "$install_dir" 2>/dev/null \
    || die "cannot create install directory: ${install_dir}"
  [ -w "$install_dir" ] \
    || die "install directory is not writable: ${install_dir} (hint: set RADIX_INSTALL_DIR to a writable path)"

  info "Extracting ${BINARY} ..."
  tar -xzf "$archive_path" -C "$tmpdir" "${BINARY}" \
    || die "failed to extract ${BINARY} from archive"

  local extracted="${tmpdir}/${BINARY}"
  [ -f "$extracted" ] || die "${BINARY} not found in archive"
  [ -x "$extracted" ] || die "extracted ${BINARY} is not executable"

  mv "$extracted" "${install_dir}/${BINARY}" \
    || die "failed to install ${BINARY} to ${install_dir}"
  success "installed ${install_dir}/${BINARY}"
}
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

  # Escaped heredoc: $HOME and $PATH expand when the user's shell sources this file
  cat >> "$shell_file" << RADIX_PATH

export PATH="\$HOME/.radix/bin:\$PATH"  # added by radix installer
RADIX_PATH

  info "Added ~/.radix/bin to PATH in ${shell_file}"
  info "Restart your shell or run: source ${shell_file}"
}

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
