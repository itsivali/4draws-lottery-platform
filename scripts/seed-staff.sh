#!/usr/bin/env bash
# Seeds the local back office's staff (ADR 0018): signs in as ada, the first admin (STAFF_BOOTSTRAP_ADMIN in compose),
# through the local staff realm, and invites the realm's test staff with their roles. Safe to run again: members that
# exist are left as they are. Local only: the realm and its password exist only in compose.
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
invite() { # invite <user> <name> <role,role>
  local roles code
  roles=$(jq -cn --arg r "$3" '$r | split(",")')
  code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$API/v1/staff/members" -H "Authorization: Bearer $admin" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: seed-invite-$1" \
    -d "{\"email\":\"$1@staff.local\",\"name\":\"$2\",\"roles\":$roles}")
  case "$code" in 201|409) ;; *) echo "inviting $1 failed ($code)" >&2; exit 1 ;; esac
}
invite alice Alice draw_official,auditor,finance
invite bob Bob draw_official
invite kim Kim kyc_reviewer
invite paula Paula payments_operator,alerts_operator
invite otis Otis payout_operator
invite tara Tara tax_officer
invite tom Tom tax_officer
invite sam Sam support
echo "Staff seeded: ada (admin), alice (draws, auditor, finance), bob, kim (identity review), paula, otis, tara, tom, sam."
