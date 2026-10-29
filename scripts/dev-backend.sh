#!/usr/bin/env bash
# Runs the backend locally: the api (with its workers), Postgres, Valkey, Keycloak (staff sign-in) and the M-Pesa, KYC
# drand mocks, all in Docker. The M-Pesa mock approves every prompt, so the apps can be used end to end without real
# money. Identity submissions wait for a reviewer, as in production: approve them in the back office (staff kim).
#
#   scripts/dev-backend.sh            start (or update) the stack and wait until it is ready
#   scripts/dev-backend.sh --seed     also create a ready-to-play test player (see below)
#   scripts/dev-backend.sh --sandbox  top-ups through Safaricom's Daraja sandbox: a real STK prompt reaches the phone
#                                     number of the account (needs .env.sandbox, see .env.sandbox.example, and
#                                     cloudflared for the callback tunnel). Withdrawals still need sandbox B2C settings.
#   scripts/dev-backend.sh --logs     follow the api's logs
#   scripts/dev-backend.sh --down     stop it and delete its data
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

API=http://localhost:8080
SEED_MSISDN=0712345678
SEED_PASSWORD="correct horse battery"
SEED_EMAIL=player@example.com

TUNNEL_PID="${XDG_RUNTIME_DIR:-/tmp}/lottery-tunnel.pid"
stop_tunnel() { if [ -f "$TUNNEL_PID" ]; then kill "$(cat "$TUNNEL_PID")" 2>/dev/null || true; rm -f "$TUNNEL_PID"; fi; }

case "${1:-}" in
  --down) stop_tunnel; docker compose down -v; exit 0 ;;
  --logs) exec docker compose logs -f api ;;
  ""|--seed|--sandbox) ;;
  *) echo "usage: $0 [--seed|--sandbox|--logs|--down]" >&2; exit 2 ;;
esac

command -v docker >/dev/null || { echo "Docker is required" >&2; exit 1; }
compose=(docker compose)
if [ "${1:-}" = --sandbox ]; then
  [ -f .env.sandbox ] || { echo "Copy .env.sandbox.example to .env.sandbox and fill in your Daraja sandbox app's credentials" >&2; exit 1; }
  command -v cloudflared >/dev/null || { echo "Daraja must reach this machine: install cloudflared (sudo pacman -S cloudflared)" >&2; exit 1; }
  set -a; . ./.env.sandbox; set +a
  # A quick tunnel gives the api a public https address for Daraja's callbacks (it exposes the local api, whose keys are
  # throwaway). It keeps running after this script; --down stops it.
  stop_tunnel
  log="${XDG_RUNTIME_DIR:-/tmp}/lottery-tunnel.log"
  setsid nohup cloudflared tunnel --no-autoupdate --url "http://localhost:8080" >"$log" 2>&1 &
  echo $! >"$TUNNEL_PID"
  PUBLIC_BASE_URL=
  for _ in $(seq 1 30); do
    PUBLIC_BASE_URL=$(grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' "$log" | head -1 || true)
    [ -n "$PUBLIC_BASE_URL" ] && break
    sleep 1
  done
  [ -n "$PUBLIC_BASE_URL" ] || { echo "The tunnel did not start; see $log" >&2; stop_tunnel; exit 1; }
  DARAJA_CALLBACK_TOKEN=$(openssl rand -hex 24) # a fresh secret callback path for each run
  export PUBLIC_BASE_URL DARAJA_CALLBACK_TOKEN
  compose+=(-f docker-compose.yml -f docker-compose.sandbox.yml)
  echo "Daraja sandbox: callbacks reach $PUBLIC_BASE_URL"
fi
echo "Starting the backend (the first build takes a few minutes)..."
"${compose[@]}" up -d --build --wait --remove-orphans
curl -fsS "$API/readyz" >/dev/null
command -v jq >/dev/null && scripts/seed-staff.sh || echo "Install jq to seed the back office's staff (scripts/seed-staff.sh)" >&2

seed() {
  command -v jq >/dev/null || { echo "--seed needs jq" >&2; exit 1; }
  key() { echo "seed-$(date +%s%N)-$1"; }
  post() { # post <path> <json> [token]
    curl -fsS -X POST "$API$1" -H "Idempotency-Key: $(key "$1")" -H 'Content-Type: application/json' \
      ${3:+-H "Authorization: Bearer $3"} -d "$2"
  }
  creds="{\"msisdn\":\"$SEED_MSISDN\",\"email\":\"$SEED_EMAIL\",\"first_name\":\"Test\",\"second_name\":\"Player\",\"password\":\"$SEED_PASSWORD\",\"device\":{\"platform\":\"web\"}}"
  token=$(post /v1/auth/signup "$creds" 2>/dev/null | jq -r .access_token) || true
  if [ -z "$token" ] || [ "$token" = null ]; then
    token=$(post /v1/auth/login "$creds" | jq -r .access_token) # already seeded
  fi
  kyc=$(curl -fsS "$API/v1/kyc" -H "Authorization: Bearer $token" | jq -r .status)
  if [ "$kyc" != verified ]; then
    [ "$kyc" = manual_review ] \
      || post /v1/kyc '{"national_id":"12345678","full_name":"Test Player","date_of_birth":"1990-05-17"}' "$token" >/dev/null
    # approved the way a reviewer would, through the back office API as kim (the local identity reviewer)
    kim=$(scripts/seed-staff.sh token kim)
    player=$(curl -fsS "$API/v1/staff/kyc/pending" -H "Authorization: Bearer $kim" \
      | jq -r --arg m "254${SEED_MSISDN#0}" '.submissions[] | select(.msisdn == $m) | .user_id')
    post "/v1/staff/kyc/$player/approve" '{"reason":"local seed"}' "$kim" >/dev/null
  fi
  balance=$(curl -fsS "$API/v1/wallet" -H "Authorization: Bearer $token" | jq -r .balance_kes)
  if [ "$balance" -lt 100 ]; then
    payment=$(post /v1/payments/topups '{"amount_kes":1000}' "$token" | jq -r .id)
    for _ in $(seq 1 60); do
      [ "$(curl -fsS "$API/v1/payments/$payment" -H "Authorization: Bearer $token" | jq -r .status)" = credited ] && break
      sleep 1
    done
    balance=$(curl -fsS "$API/v1/wallet" -H "Authorization: Bearer $token" | jq -r .balance_kes)
  fi
  echo "Test player: $SEED_MSISDN / $SEED_PASSWORD (ID verified, phone proved, balance KES $balance)"
}
[ "${1:-}" = --seed ] && seed
if [ "${1:-}" = --sandbox ]; then
  cat <<EOF

Top-ups now send a real M-Pesa prompt (Daraja sandbox) to the phone number of the player's account: sign up in the app
with your own Safaricom number, have the ID approved in the back office (kim), then top up (KES 10 is the minimum).
EOF
fi

cat <<EOF

Backend ready:
  API          $API          (health: $API/readyz)
  Keycloak     http://localhost:8081   (admin/admin; staff realm "lottery-staff", staff password local-dev-staff)
               Back office staff sign in through it (ada is the admin); identity submissions wait for kim to review.
  Postgres     localhost:${POSTGRES_PORT:-5433}  (lottery/lottery)

Next: scripts/dev-frontend.sh (web app, back office) or scripts/dev-mobile.sh (Android emulator).
Stop with: scripts/dev-backend.sh --down
EOF
