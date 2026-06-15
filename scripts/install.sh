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
