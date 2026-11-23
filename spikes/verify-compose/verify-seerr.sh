#!/usr/bin/env bash
# Throwaway verification harness for spikes/verify-compose (WP0.6 Spike S4).
# Reports exact HTTP status code and first 500 chars of body for each call.
set -u
BASE="http://127.0.0.1:18051"

call() {
  local method="$1" path="$2" body="${3:-}"
  local tmp code
  tmp="$(mktemp)"
  if [ -n "$body" ]; then
    code="$(curl -s -S -o "$tmp" -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Accept: application/json" -H "Content-Type: application/json" --data "$body" 2>&1)"
  else
    code="$(curl -s -S -o "$tmp" -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Accept: application/json" 2>&1)"
  fi
  echo "### $method $path"
  echo "HTTP $code"
  head -c 500 "$tmp" 2>/dev/null
  echo
  rm -f "$tmp"
}
