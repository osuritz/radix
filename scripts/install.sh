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
