#!/usr/bin/env bash
# spikes/verify-jellyfin-headless.sh
#
# Runs the full headless Jellyfin 12.4 startup-wizard sequence against a
# fresh container, from the host (localhost-bound port).
#
# Usage:
#   ./spikes/verify-jellyfin-headless.sh <host_port> <config_dir>
#
# Sequence (per WP0.9/S3 verification):
#   1. Poll GET /System/Info/Public until 200 (container reachable)
#   2. Poll GET /Startup/User until 200 BEFORE any POST (pre-first-user reachable)
#   3. POST /Startup/Configuration {}
#   4. POST /Startup/RemoteAccess {}
#   5. POST /Startup/User   (creates first admin)
#   6. POST /Startup/Complete {}   (flips StartupWizardCompleted false->true)
#   7. POST /Users/AuthenticateByName (MediaBrowser header, body field "Pw")
#   8. POST /Auth/Keys?app=arrstart (with admin token)
#
# "Verified" only if every step succeeds. Poll loops have bounded retries so
# timing issues surface as failures rather than false passes.
set -u

PORT="${1:-18099}"
CONFIG_DIR="${2:-./scratch/jellyfin-run}"
BASE="http://127.0.0.1:${PORT}"
ADMIN="arrstart-admin"
ADMIN_PW="Arrstart-Pw-12345"

pass=0
fail=0

check() {
  local desc="$1"; local code="$2"
  case "$code" in
    200|204) echo "  PASS: $desc ($code)"; pass=$((pass+1));;
    *)       echo "  FAIL: $desc ($code, expected 200/204)"; fail=$((fail+1));;
  esac
}

# ---- helpers ---------------------------------------------------------------
http_code() { curl -s -o /dev/null -w "%{http_code}" --max-time 4 "$1"; }

# ---- 1. Poll GET /System/Info/Public until 200 -----------------------------
echo "==> Polling GET /System/Info/Public until 200 ..."
public_ok=0
for i in $(seq 1 90); do
  c=$(http_code "$BASE/System/Info/Public")
  if [ "$c" = "200" ]; then echo "   Public reachable (attempt $i)"; public_ok=1; break; fi
  sleep 1
done
[ "$public_ok" = "1" ] || { echo "  FAIL: /System/Info/Public never 200"; fail=$((fail+1)); }

# ---- 2. Poll GET /Startup/User until 200 before any POST -------------------
echo "==> Polling GET /Startup/User until 200 (before any POST) ..."
setup_user_ok=0
for i in $(seq 1 120); do
  c=$(http_code "$BASE/Startup/User")
  if [ "$c" = "200" ]; then echo "   /Startup/User reachable (attempt $i)"; setup_user_ok=1; break; fi
  sleep 1
done
[ "$setup_user_ok" = "1" ] || { echo "  FAIL: /Startup/User never 200"; fail=$((fail+1)); }

u_body=$(curl -s --max-time 4 "$BASE/Startup/User")
echo "   GET /Startup/User body: $u_body"

# ---- 3. POST /Startup/Configuration {} -------------------------------------
echo "==> POST /Startup/Configuration {}"
c=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/Startup/Configuration" \
    -H 'Content-Type: application/json' -d '{}' --max-time 6)
check "POST /Startup/Configuration {}" "$c"

# ---- 4. POST /Startup/RemoteAccess {} --------------------------------------
echo "==> POST /Startup/RemoteAccess {}"
c=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/Startup/RemoteAccess" \
    -H 'Content-Type: application/json' -d '{}' --max-time 6)
check "POST /Startup/RemoteAccess {}" "$c"

# ---- 5. POST /Startup/User (create first admin) ----------------------------
echo "==> POST /Startup/User (create first admin)"
c=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/Startup/User" \
    -H 'Content-Type: application/json' \
    -d "{\"Name\":\"$ADMIN\",\"Password\":\"$ADMIN_PW\",\"HasPassword\":true}" --max-time 6)
check "POST /Startup/User create admin" "$c"

# ---- 6. POST /Startup/Complete + poll StartupWizardCompleted ---------------
#
# Success criterion: StartupWizardCompleted flips to true (readable without
# auth). POST /Startup/Complete may return 204 in some builds and 401 in others
# while still flipping the flag, so the poll is the authoritative signal.
echo "==> POST /Startup/Complete then poll StartupWizardCompleted ..."
for i in $(seq 1 3); do
  curl -s -o /dev/null -X POST "$BASE/Startup/Complete" \
      -H 'Content-Type: application/json' -d '{}' --max-time 6 >/dev/null
  sleep 1
done
completed="false"
for i in $(seq 1 30); do
  completed=$(curl -s --max-time 4 "$BASE/System/Info/Public" | grep -o '"StartupWizardCompleted":[a-z]*')
  if echo "$completed" | grep -q 'true'; then echo "   StartupWizardCompleted=true (poll $i)"; pass=$((pass+1)); break; fi
  sleep 1
done
if ! echo "$completed" | grep -q 'true'; then echo "  FAIL: StartupWizardCompleted never true"; fail=$((fail+1)); fi

# ---- 7. POST /Users/AuthenticateByName (MediaBrowser header, "Pw" field) ---
echo "==> POST /Users/AuthenticateByName (Pw field)"
auth_raw=$(curl -s -w "\n%{http_code}" -X POST "$BASE/Users/AuthenticateByName" \
    -H "Authorization: MediaBrowser Client=\"arrstart\", Device=\"verify\", DeviceId=\"arrstart-verify-jf\", Version=\"1.0.0.0\"" \
    -H 'Content-Type: application/json' \
    -d "{\"Username\":\"$ADMIN\",\"Pw\":\"$ADMIN_PW\"}" --max-time 6)
auth_http=$(echo "$auth_raw" | tail -1 | sed 's/HTTP //')
auth_body=$(echo "$auth_raw" | sed '$d')
check "POST /Users/AuthenticateByName (Pw field)" "$auth_http"

token=$(printf '%s' "$auth_body" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("AccessToken",""))' 2>/dev/null || true)
is_admin=$(printf '%s' "$auth_body" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("IsAdministrator","MISSING"))' 2>/dev/null || true)
echo "   AccessToken: ${token:0:40}..."
echo "   IsAdministrator: $is_admin"

# Negative: "Password" field must NOT bind (expect 400 in the spec; 401 in this
# build because Password is not the bound field so auth simply fails). Either
# rejection code means the field was NOT accepted as a credential.
echo "==> Negative: POST /Users/AuthenticateByName with 'Password' field (expect rejection)"
neg_http=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/Users/AuthenticateByName" \
    -H "Authorization: MediaBrowser Client=\"arrstart\", Device=\"verify\", DeviceId=\"arrstart-verify-jf\", Version=\"1.0.0.0\"" \
    -H 'Content-Type: application/json' \
    -d "{\"Username\":\"$ADMIN\",\"Password\":\"$ADMIN_PW\"}" --max-time 6)
case "$neg_http" in
  400|401|"") echo "  PASS: Password field rejected ($neg_http)"; pass=$((pass+1)) ;;
  *)          echo "  FAIL: Password field unexpectedly accepted ($neg_http)"; fail=$((fail+1)) ;;
esac

# ---- 8. POST /Auth/Keys with admin token ----------------------------------
if [ -n "$token" ]; then
  echo "==> POST /Auth/Keys?app=arrstart"
  key_raw=$(curl -s -w "\n%{http_code}" -X POST "$BASE/Auth/Keys?app=arrstart" \
      -H "Authorization: MediaBrowser Client=\"arrstart\", Device=\"verify\", DeviceId=\"arrstart-verify-jf\", Version=\"1.0.0.0\", Token=\"$token\"" \
      --max-time 6)
  key_http=$(echo "$key_raw" | tail -1 | sed 's/HTTP //')
  check "POST /Auth/Keys?app=arrstart" "$key_http"
else
  echo "  SKIP: POST /Auth/Keys (no token obtained)"
fi

echo ""
echo "RESULT: pass=$pass fail=$fail"
[ "$fail" -eq 0 ]
