#!/usr/bin/env bash

set -euo pipefail

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required environment variable: ${name}" >&2
    exit 1
  fi
}

request_status() {
  local method="$1"
  local url="$2"
  local auth_header="${3:-}"
  local tmp
  tmp="$(mktemp)"

  local args=(
    -sS
    -X "$method"
    -o "$tmp"
    -w "%{http_code}"
  )
  if [[ -n "$auth_header" ]]; then
    args+=(-H "Authorization: ${auth_header}")
  fi

  local status
  status="$(curl "${args[@]}" "$url")"
  local body
  body="$(cat "$tmp")"
  rm -f "$tmp"

  printf '%s\n%s' "$status" "$body"
}

assert_status() {
  local label="$1"
  local expected="$2"
  local actual="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "[FAIL] ${label}: expected ${expected}, got ${actual}" >&2
    exit 1
  fi
  echo "[OK] ${label}: ${actual}"
}

join_url() {
  local base="${1%/}"
  local path="$2"
  if [[ "$path" != /* ]]; then
    path="/${path}"
  fi
  printf '%s%s' "$base" "$path"
}

require_env PUBLIC_BASE_URL
require_env PUBLIC_ROUTE
require_env PROTECTED_ROUTE
require_env PROTECTED_BEARER_TOKEN

PUBLIC_EXPECTED_STATUS="${PUBLIC_EXPECTED_STATUS:-200}"
PROTECTED_NO_AUTH_EXPECTED_STATUS="${PROTECTED_NO_AUTH_EXPECTED_STATUS:-401}"
PROTECTED_WITH_AUTH_EXPECTED_STATUS="${PROTECTED_WITH_AUTH_EXPECTED_STATUS:-200}"
HEALTH_PATH="${HEALTH_PATH:-/health}"
HEALTH_EXPECTED_STATUS="${HEALTH_EXPECTED_STATUS:-200}"
MANAGEMENT_AUTH_ENABLED="${MANAGEMENT_AUTH_ENABLED:-false}"
MANAGEMENT_READY_PATH="${MANAGEMENT_READY_PATH:-/ready}"
MANAGEMENT_READY_EXPECTED_STATUS="${MANAGEMENT_READY_EXPECTED_STATUS:-200}"

public_url="$(join_url "$PUBLIC_BASE_URL" "$PUBLIC_ROUTE")"
protected_url="$(join_url "$PUBLIC_BASE_URL" "$PROTECTED_ROUTE")"

echo "Running targeted smoke test against ${PUBLIC_BASE_URL}"

mapfile -t response < <(request_status GET "$public_url")
assert_status "public route" "$PUBLIC_EXPECTED_STATUS" "${response[0]}"

mapfile -t response < <(request_status GET "$protected_url")
assert_status "protected route without token" "$PROTECTED_NO_AUTH_EXPECTED_STATUS" "${response[0]}"

mapfile -t response < <(request_status GET "$protected_url" "Bearer ${PROTECTED_BEARER_TOKEN}")
assert_status "protected route with token" "$PROTECTED_WITH_AUTH_EXPECTED_STATUS" "${response[0]}"

management_base="${MANAGEMENT_BASE_URL:-$PUBLIC_BASE_URL}"
health_url="$(join_url "$management_base" "$HEALTH_PATH")"
mapfile -t response < <(request_status GET "$health_url")
assert_status "management health" "$HEALTH_EXPECTED_STATUS" "${response[0]}"

if [[ "$MANAGEMENT_AUTH_ENABLED" == "true" ]]; then
  require_env MANAGEMENT_ROUTE
  require_env MANAGEMENT_BEARER_TOKEN

  management_url="$(join_url "$management_base" "$MANAGEMENT_ROUTE")"
  mapfile -t response < <(request_status GET "$management_url" "Bearer ${MANAGEMENT_BEARER_TOKEN}")
  assert_status "management protected route" "${MANAGEMENT_READY_EXPECTED_STATUS}" "${response[0]}"
fi

echo "Smoke test completed successfully"
