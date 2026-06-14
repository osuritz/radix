#!/usr/bin/env bash
#
# smoke.sh - End-to-end smoke test for radix.
#
# Builds the binary and exercises every command end-to-end against real running
# servers: version, validate, gencert, serve, echo, mock (built-ins + custom
# routes), and proxy. Each long-running server is started in the background on a
# fixed high port, polled for readiness, probed with curl, asserted, and then
# torn down. Prints a PASS/FAIL line per check and exits non-zero if any check
# fails.
#
# Usage:
#   bash scripts/smoke.sh        # or: make smoke
#
# Requirements: bash, curl, and a free port range starting at 18080 (ports
# 18080-18086 are used). Override the base port with PORT_BASE=NNNNN.

set -euo pipefail

# --- Configuration ----------------------------------------------------------

# Resolve repo root from this script's location so it works from any cwd.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

BIN="$ROOT_DIR/bin/radix"
HOST="127.0.0.1"
PORT_BASE="${PORT_BASE:-18080}"

# Fixed high ports (one per server so checks can overlap cleanly).
PORT_SERVE=$((PORT_BASE + 0))
PORT_ECHO=$((PORT_BASE + 1))
PORT_MOCK=$((PORT_BASE + 2))
PORT_MOCK_ROUTES=$((PORT_BASE + 3))
PORT_PROXY_BACKEND=$((PORT_BASE + 4))
PORT_PROXY=$((PORT_BASE + 5))

TMPDIR_SMOKE="$(mktemp -d "${TMPDIR:-/tmp}/radix-smoke.XXXXXX")"
PIDS=()

PASS_COUNT=0
FAIL_COUNT=0

# --- Output helpers ---------------------------------------------------------

if [ -t 1 ]; then
  C_GREEN=$'\033[0;32m'
  C_RED=$'\033[0;31m'
  C_DIM=$'\033[2m'
  C_RESET=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_DIM=""; C_RESET=""
fi

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '%sPASS%s %s\n' "$C_GREEN" "$C_RESET" "$1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '%sFAIL%s %s\n' "$C_RED" "$C_RESET" "$1"
  if [ -n "${2:-}" ]; then
    printf '     %s%s%s\n' "$C_DIM" "$2" "$C_RESET"
  fi
}

info() {
  printf '%s--- %s%s\n' "$C_DIM" "$1" "$C_RESET"
}

# assert_contains <description> <haystack> <needle>
# Uses a here-string (not a pipe) so grep -q exiting early cannot SIGPIPE a
# writer under `set -o pipefail` and produce a spurious FAIL.
assert_contains() {
  local desc="$1" haystack="$2" needle="$3"
  if grep -qF -- "$needle" <<<"$haystack"; then
    pass "$desc"
  else
    fail "$desc" "expected to find: $needle"
  fi
}

# assert_eq <description> <actual> <expected>
assert_eq() {
  local desc="$1" actual="$2" expected="$3"
  if [ "$actual" = "$expected" ]; then
    pass "$desc"
  else
    fail "$desc" "expected '$expected', got '$actual'"
  fi
}

# --- Process management -----------------------------------------------------

# start_bg <logfile> <command...> : start a background process, record its pid.
# Sets the global LAST_PID to the new child's pid (rather than echoing it, so the
# PIDS array used by cleanup is mutated in the main shell, not a subshell) and
# appends it to PIDS for teardown.
LAST_PID=""
start_bg() {
  local logfile="$1"; shift
  "$@" >"$logfile" 2>&1 &
  LAST_PID=$!
  PIDS+=("$LAST_PID")
}

# wait_ready <url> <pid> [expect_substr] : poll a URL until OUR server answers.
#
# Hardening over a naive "any HTTP response" probe:
#   - <pid> is the child we just started; if it has died we fail fast instead of
#     polling (and never risk probing some unrelated process on a colliding port).
#   - [expect_substr], when given, must appear in the response body before we
#     consider the server ready — this proves it's OUR server (e.g. /_health),
#     not whatever else might be listening on that port.
wait_ready() {
  local url="$1" pid="$2" expect="${3:-}"
  local i body
  for i in $(seq 1 100); do
    # Fail fast if the server we started has already exited.
    if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
      return 1
    fi
    if body="$(curl -sS --max-time 2 "$url" 2>/dev/null)"; then
      if [ -z "$expect" ] || grep -qF -- "$expect" <<<"$body"; then
        return 0
      fi
    fi
    sleep 0.1
  done
  return 1
}

cleanup() {
  local pid
  for pid in "${PIDS[@]:-}"; do
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -rf "$TMPDIR_SMOKE"
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM

# --- Build ------------------------------------------------------------------

info "Building radix"
if make build >"$TMPDIR_SMOKE/build.log" 2>&1; then
  pass "make build"
else
  fail "make build" "see $TMPDIR_SMOKE/build.log"
  cat "$TMPDIR_SMOKE/build.log"
  exit 1
fi

# --- version ----------------------------------------------------------------

info "version"

version_out="$("$BIN" version 2>&1 || true)"
assert_contains "version prints 'radix'" "$version_out" "radix"

version_json="$("$BIN" version --json 2>&1 || true)"
assert_contains "version --json has version field" "$version_json" '"version"'

# --- validate ---------------------------------------------------------------

info "validate"

if "$BIN" validate examples/radix.example.yml >"$TMPDIR_SMOKE/validate.log" 2>&1; then
  pass "validate examples/radix.example.yml exits 0"
else
  fail "validate examples/radix.example.yml exits 0" "$(tail -n 3 "$TMPDIR_SMOKE/validate.log")"
fi

# --- gencert ----------------------------------------------------------------

info "gencert"

CERT_DIR="$TMPDIR_SMOKE/certs"
if "$BIN" gencert --host localhost --output "$CERT_DIR" >"$TMPDIR_SMOKE/gencert.log" 2>&1; then
  if [ -f "$CERT_DIR/cert.pem" ] && [ -f "$CERT_DIR/key.pem" ]; then
    pass "gencert produces cert.pem and key.pem"
  else
    fail "gencert produces cert.pem and key.pem" "missing files in $CERT_DIR"
  fi
else
  fail "gencert exits 0" "$(tail -n 3 "$TMPDIR_SMOKE/gencert.log")"
fi

# --- serve ------------------------------------------------------------------

info "serve"

SERVE_DIR="$TMPDIR_SMOKE/www"
mkdir -p "$SERVE_DIR"
echo "<html><body>radix smoke index</body></html>" >"$SERVE_DIR/index.html"

start_bg "$TMPDIR_SMOKE/serve.log" \
  "$BIN" serve "$SERVE_DIR" --host "$HOST" --port "$PORT_SERVE" --spa
serve_pid="$LAST_PID"
if wait_ready "http://$HOST:$PORT_SERVE/" "$serve_pid" "radix smoke index"; then
  code="$(curl -sS -o "$TMPDIR_SMOKE/serve-body" -w '%{http_code}' "http://$HOST:$PORT_SERVE/" || true)"
  assert_eq "serve returns 200" "$code" "200"
  assert_contains "serve returns index.html" "$(cat "$TMPDIR_SMOKE/serve-body")" "radix smoke index"

  # SPA fallback: an unknown deep path should still serve index.html with 200.
  spa_code="$(curl -sS -o "$TMPDIR_SMOKE/serve-spa-body" -w '%{http_code}' "http://$HOST:$PORT_SERVE/some/spa/route" || true)"
  assert_eq "serve --spa fallback returns 200" "$spa_code" "200"
  assert_contains "serve --spa falls back to index.html" "$(cat "$TMPDIR_SMOKE/serve-spa-body")" "radix smoke index"
else
  fail "serve becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/serve.log")"
fi

# --- echo -------------------------------------------------------------------

info "echo"

start_bg "$TMPDIR_SMOKE/echo.log" \
  "$BIN" echo --host "$HOST" --port "$PORT_ECHO"
echo_pid="$LAST_PID"
if wait_ready "http://$HOST:$PORT_ECHO/_health" "$echo_pid" '"status":"ok"'; then
  echo_code="$(curl -sS -o "$TMPDIR_SMOKE/echo-body" -w '%{http_code}' -X POST "http://$HOST:$PORT_ECHO/anything" \
    -H 'Content-Type: application/json' \
    -d '{"smoke":"echo-test"}' || true)"
  echo_body="$(cat "$TMPDIR_SMOKE/echo-body")"
  assert_eq "echo returns 200" "$echo_code" "200"
  assert_contains "echo reflects request method" "$echo_body" '"method": "POST"'
  assert_contains "echo reflects request body" "$echo_body" 'echo-test'
else
  fail "echo becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/echo.log")"
fi

# --- mock (built-ins) -------------------------------------------------------

info "mock (built-ins)"

start_bg "$TMPDIR_SMOKE/mock.log" \
  "$BIN" mock --host "$HOST" --port "$PORT_MOCK"
mock_pid="$LAST_PID"
if wait_ready "http://$HOST:$PORT_MOCK/_health" "$mock_pid" '"status":"ok"'; then
  get_code="$(curl -sS -o "$TMPDIR_SMOKE/mock-get-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK/get?foo=bar" || true)"
  assert_eq "mock /get returns 200" "$get_code" "200"
  assert_contains "mock /get reflects query args" "$(cat "$TMPDIR_SMOKE/mock-get-body")" '"foo"'

  status_code="$(curl -sS -o /dev/null -w '%{http_code}' "http://$HOST:$PORT_MOCK/status/418" || true)"
  assert_eq "mock /status/418 returns 418" "$status_code" "418"

  uuid_code="$(curl -sS -o "$TMPDIR_SMOKE/mock-uuid-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK/uuid" || true)"
  assert_eq "mock /uuid returns 200" "$uuid_code" "200"
  uuid_body="$(cat "$TMPDIR_SMOKE/mock-uuid-body")"
  # Assert the response is a JSON object with a v4-shaped uuid value.
  if grep -qiE '"uuid"[[:space:]]*:[[:space:]]*"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"' <<<"$uuid_body"; then
    pass "mock /uuid returns a v4 uuid"
  else
    fail "mock /uuid returns a v4 uuid" "got: $uuid_body"
  fi

  headers_code="$(curl -sS -o "$TMPDIR_SMOKE/mock-headers-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK/headers" || true)"
  assert_eq "mock /headers returns 200" "$headers_code" "200"
  assert_contains "mock /headers returns a headers object" "$(cat "$TMPDIR_SMOKE/mock-headers-body")" '"headers"'
else
  fail "mock becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/mock.log")"
fi

# --- mock (custom routes) ---------------------------------------------------

info "mock (custom routes)"

start_bg "$TMPDIR_SMOKE/mock-routes.log" \
  "$BIN" mock --routes examples/mock-routes.yml --host "$HOST" --port "$PORT_MOCK_ROUTES"
mock_routes_pid="$LAST_PID"
if wait_ready "http://$HOST:$PORT_MOCK_ROUTES/_health" "$mock_routes_pid" '"status":"ok"'; then
  health_code="$(curl -sS -o "$TMPDIR_SMOKE/route-health-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK_ROUTES/api/health" || true)"
  assert_eq "custom route /api/health returns 200" "$health_code" "200"
  assert_contains "custom route /api/health returns templated body" "$(cat "$TMPDIR_SMOKE/route-health-body")" '"status":"ok"'

  user_code="$(curl -sS -o "$TMPDIR_SMOKE/route-user-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK_ROUTES/api/users/123" || true)"
  assert_eq "custom route /api/users/:id returns 200" "$user_code" "200"
  assert_contains "custom route /api/users/:id templates the path param" "$(cat "$TMPDIR_SMOKE/route-user-body")" '"id": "123"'

  # Built-in endpoints remain reachable alongside custom routes.
  builtin_code="$(curl -sS -o "$TMPDIR_SMOKE/route-uuid-body" -w '%{http_code}' "http://$HOST:$PORT_MOCK_ROUTES/uuid" || true)"
  assert_eq "built-in /uuid returns 200 with custom routes" "$builtin_code" "200"
  assert_contains "built-in /uuid still reachable with custom routes" "$(cat "$TMPDIR_SMOKE/route-uuid-body")" '"uuid"'
else
  fail "mock --routes becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/mock-routes.log")"
fi

# --- proxy ------------------------------------------------------------------

info "proxy"

# Use a mock server as the backend, then proxy to it.
start_bg "$TMPDIR_SMOKE/proxy-backend.log" \
  "$BIN" mock --host "$HOST" --port "$PORT_PROXY_BACKEND"
proxy_backend_pid="$LAST_PID"
if wait_ready "http://$HOST:$PORT_PROXY_BACKEND/_health" "$proxy_backend_pid" '"status":"ok"'; then
  start_bg "$TMPDIR_SMOKE/proxy.log" \
    "$BIN" proxy "http://$HOST:$PORT_PROXY_BACKEND" --host "$HOST" --port "$PORT_PROXY"
  proxy_pid="$LAST_PID"
  # The proxy forwards /_health to the backend, which returns {"status":"ok"}.
  if wait_ready "http://$HOST:$PORT_PROXY/_health" "$proxy_pid" '"status":"ok"'; then
    uuid_code="$(curl -sS -o "$TMPDIR_SMOKE/proxy-uuid-body" -w '%{http_code}' "http://$HOST:$PORT_PROXY/uuid" || true)"
    assert_eq "proxy /uuid returns 200" "$uuid_code" "200"
    assert_contains "proxy forwards to backend (/uuid)" "$(cat "$TMPDIR_SMOKE/proxy-uuid-body")" '"uuid"'

    get_code="$(curl -sS -o "$TMPDIR_SMOKE/proxy-get-body" -w '%{http_code}' "http://$HOST:$PORT_PROXY/get?via=proxy" || true)"
    assert_eq "proxy /get returns 200" "$get_code" "200"
    assert_contains "proxy forwards query args to backend" "$(cat "$TMPDIR_SMOKE/proxy-get-body")" '"via"'
  else
    fail "proxy becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/proxy.log")"
  fi
else
  fail "proxy backend becomes ready" "$(tail -n 5 "$TMPDIR_SMOKE/proxy-backend.log")"
fi

# --- Summary ----------------------------------------------------------------

echo
printf '%s================ smoke summary ================%s\n' "$C_DIM" "$C_RESET"
printf 'checks passed: %s%d%s\n' "$C_GREEN" "$PASS_COUNT" "$C_RESET"
if [ "$FAIL_COUNT" -gt 0 ]; then
  printf 'checks failed: %s%d%s\n' "$C_RED" "$FAIL_COUNT" "$C_RESET"
  echo "SMOKE TEST FAILED"
  exit 1
fi
echo "ALL SMOKE CHECKS PASSED"
