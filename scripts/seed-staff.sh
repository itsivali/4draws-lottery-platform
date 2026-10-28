#!/usr/bin/env bash
# Seeds the local back office's staff (ADR 0019): signs in as ada, the first admin (STAFF_BOOTSTRAP_ADMIN in compose),
# through the local staff realm, and invites the realm's test staff with their role (admin or staff) and the local
# password local-dev-staff-2026 (temporary: the back office asks for a new one at the first sign-in). Safe to run
# again: members that exist are left as they are. Local only: the realm and these passwords exist only in compose.
#
#   scripts/seed-staff.sh                 seed
#   scripts/seed-staff.sh token <user>    print a back-office session token for a local staff member (for curl/e2e)
set -euo pipefail
API=${API:-http://localhost:8080}
REALM=http://localhost:8081/realms/lottery-staff

# session <user>: the local realm's ID token for user (password grant, local-only client), exchanged for a session
session() {
  local id
  id=$(curl -fsS -d client_id=lottery-e2e -d grant_type=password -d scope=openid -d "username=$1" -d password=local-dev-staff \
    "$REALM/protocol/openid-connect/token" | jq -r .id_token)
  curl -fsS -X POST "$API/v1/staff/session" -H 'Content-Type: application/json' -H "Idempotency-Key: seed-$1-$(date +%s%N)" \
    -d "{\"provider\":\"dev\",\"id_token\":\"$id\",\"nonce\":\"\"}" | jq -r .token
}

if [ "${1:-}" = token ]; then
  session "${2:?usage: $0 token <user>}"
  exit 0
fi

admin=$(session ada)
invite() { # invite <user> <name> <admin|staff>
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$API/v1/staff/members" -H "Authorization: Bearer $admin" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: seed-invite-$1" \
    -d "{\"email\":\"$1@staff.local\",\"name\":\"$2\",\"roles\":[\"$3\"],\"password\":\"local-dev-staff-2026\"}")
  case "$code" in 201|409) ;; *) echo "inviting $1 failed ($code)" >&2; exit 1 ;; esac
}
invite alice Alice admin
invite bob Bob staff
invite kim Kim staff
invite paula Paula staff
invite otis Otis staff
invite tara Tara staff
invite tom Tom staff
invite sam Sam staff
echo "Staff seeded: ada and alice (admin), bob, kim, paula, otis, tara, tom, sam (staff); password local-dev-staff-2026 (temporary)."
