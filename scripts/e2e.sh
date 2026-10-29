#!/usr/bin/env bash
# E2E: bring up the stack, check the probes, then sign up, log in, submit an identity that a reviewer approves in the
# back office API (as in production: no registry, ADR 0009), and top up through the real API,
# workers and the Daraja mock (which approves every prompt after two seconds and calls the api back); then pay into the
# paybill through the mock and wait for the verified credit.
# Then (phases 6-7): a play, the wallet and account reads, a withdrawal followed until paid, an attestation challenge,
# and the back office with real staff tokens from the local staff realm. The clients' own journeys run in their test
# suites (Flutter widget/integration tests, back office component tests).
set -euo pipefail
docker compose up -d --build --wait
trap 'docker compose down -v' EXIT
curl -fsS localhost:8080/healthz >/dev/null
curl -fsS localhost:8080/readyz >/dev/null
for p in 8090 8093; do curl -fsS "localhost:$p/healthz" >/dev/null; done

post() { # post <path> <idempotency-key> <json> [token]
  curl -sS -o /tmp/e2e.body -w '%{http_code}' -X POST "localhost:8080$1" -H "Idempotency-Key: $2" \
    -H 'Content-Type: application/json' ${4:+-H "Authorization: Bearer $4"} -d "$3"
}
dev='"device":{"fingerprint":"e2e-device","platform":"android"}'
# Back-office sessions: the local staff realm stands in for Google/Apple sign-in; ada (the bootstrap admin) invites the
# test staff first (ADR 0018).
scripts/seed-staff.sh >/dev/null
staff_token() { scripts/seed-staff.sh token "$1"; }
creds="{\"msisdn\":\"0712345678\",\"email\":\"e2e@example.com\",\"password\":\"correct horse battery\",$dev}"

[ "$(post /v1/auth/signup e2e-1 "$creds")" = 201 ] || { echo "signup failed"; cat /tmp/e2e.body; exit 1; }
[ "$(post /v1/auth/login e2e-2 "$creds")" = 200 ] || { echo "login failed"; cat /tmp/e2e.body; exit 1; }
token=$(jq -r .access_token /tmp/e2e.body)
bad="{\"msisdn\":\"0712345678\",\"password\":\"not the password\",$dev}"
[ "$(post /v1/auth/login e2e-2b "$bad")" = 401 ] || { echo "a wrong password was not refused"; cat /tmp/e2e.body; exit 1; }
[ "$(post /v1/kyc e2e-3 '{"national_id":"12345678","full_name":"Jane Wanjiru Kamau","date_of_birth":"1990-05-17"}' "$token")" = 202 ] \
  || { echo "kyc submit failed"; cat /tmp/e2e.body; exit 1; }
status=$(curl -fsS localhost:8080/v1/kyc -H "Authorization: Bearer $token" | jq -r .status)
[ "$status" = manual_review ] || { echo "the identity does not wait for a reviewer (status: $status)"; exit 1; }
# the reviewer is told it waits (counts only), opens it and approves it
kim=$(staff_token kim)
[ "$(curl -fsS localhost:8080/v1/staff/kyc/queue -H "Authorization: Bearer $kim" | jq -r .waiting)" = 1 ] || { echo "the review queue does not show it"; exit 1; }
player=$(curl -fsS localhost:8080/v1/staff/kyc/pending -H "Authorization: Bearer $kim" | jq -r '.submissions[] | select(.msisdn == "254712345678") | .user_id')
[ "$(post "/v1/staff/kyc/$player/approve" e2e-3b '{"reason":"e2e: details match the ID"}' "$kim")" = 200 ] || { echo "approval failed"; cat /tmp/e2e.body; exit 1; }
[ "$(curl -fsS localhost:8080/v1/kyc -H "Authorization: Bearer $token" | jq -r .status)" = verified ] || { echo "KYC not verified after approval"; exit 1; }

[ "$(post /v1/payments/topups e2e-4 '{"amount_kes":100}' "$token")" = 202 ] || { echo "top-up failed"; cat /tmp/e2e.body; exit 1; }
payment=$(jq -r .id /tmp/e2e.body)
for _ in $(seq 1 60); do
  status=$(curl -fsS "localhost:8080/v1/payments/$payment" -H "Authorization: Bearer $token" | jq -r .status)
  [ "$status" = credited ] && break
  sleep 0.5
done
[ "$status" = credited ] || { echo "top-up never credited (status: $status)"; docker compose logs api daraja-mock | tail -30; exit 1; }

# Paybill: register the hooks with the mock, pay into 600992 with the player's phone (proved by the top-up above) as
# the account number, and wait until the confirmation is verified with Transaction Status and credited.
hook="http://api:8080/hooks/c2b/$(printf %s local-dev-only-callback-token-000000 | sha256sum | cut -d' ' -f1)"
mock() { curl -fsS -X POST "localhost:8090$1" -H 'Authorization: Bearer mock-access-token' -H 'Content-Type: application/json' -d "$2" >/dev/null; }
mock /mpesa/c2b/v1/registerurl "{\"ShortCode\":\"600992\",\"ResponseType\":\"Cancelled\",\"ValidationURL\":\"$hook/validate\",\"ConfirmationURL\":\"$hook/confirm\"}"
mock /mpesa/c2b/v1/simulate '{"ShortCode":"600992","CommandID":"CustomerPayBillOnline","Amount":200,"Msisdn":"254708374149","BillRefNumber":"0712345678"}'
for _ in $(seq 1 60); do
  status=$(docker compose exec -T postgres psql -U lottery -d lottery -tAc "SELECT status FROM payments.c2b_confirmations LIMIT 1")
  [ "$status" = credited ] && break
  sleep 0.5
done
[ "$status" = credited ] || { echo "paybill payment never credited (status: $status)"; docker compose logs api daraja-mock | tail -30; exit 1; }

# A play: the server spins and charges; the day and the wallet agree.
[ "$(post /v1/plays e2e-5 '' "$token")" = 201 ] || { echo "play failed"; cat /tmp/e2e.body; exit 1; }
slot=$(jq -r .play.spin.slot /tmp/e2e.body); balance=$(jq -r .balance_kes /tmp/e2e.body)
[ "$(curl -fsS localhost:8080/v1/plays/today -H "Authorization: Bearer $token" | jq -r .plays)" = 1 ] || { echo "today does not show the play"; exit 1; }
[ "$(curl -fsS localhost:8080/v1/wallet -H "Authorization: Bearer $token" | jq -r .balance_kes)" = "$balance" ] || { echo "wallet disagrees with the play"; exit 1; }
[ "$(curl -fsS localhost:8080/v1/account -H "Authorization: Bearer $token" | jq -r .phone_verified)" = true ] || { echo "phone not proved by the top-up"; exit 1; }
echo "played: slot $slot, balance $balance"

# A withdrawal: debited at once, paid only when M-Pesa's answer is confirmed with Transaction Status (the payout looks
# again a minute after asking, so this takes one to two minutes).
[ "$(post /v1/payouts/withdrawals e2e-6 '{"amount_kes":50}' "$token")" = 201 ] || { echo "withdrawal failed"; cat /tmp/e2e.body; exit 1; }
payout=$(jq -r .id /tmp/e2e.body)
for _ in $(seq 1 360); do
  status=$(curl -fsS "localhost:8080/v1/payouts/$payout" -H "Authorization: Bearer $token" | jq -r .status)
  [ "$status" = paid ] && break
  sleep 0.5
done
[ "$status" = paid ] || { echo "withdrawal never paid (status: $status)"; docker compose logs api daraja-mock | tail -30; exit 1; }

# Device attestation: a single-use challenge is issued.
[ "$(post /v1/auth/attestation/challenge e2e-7 '')" = 201 ] || { echo "attestation challenge failed"; cat /tmp/e2e.body; exit 1; }

# The back office: roles are enforced.
paula=$(staff_token paula)
[ "$(curl -fsS localhost:8080/v1/staff/me -H "Authorization: Bearer $paula" | jq -r '.roles | index("staff") != null')" = true ] \
  || { echo "staff roles not seen"; exit 1; }
[ "$(post /v1/staff/payments/reconcile e2e-8 '' "$paula")" = 200 ] || { echo "staff reconciliation failed"; cat /tmp/e2e.body; exit 1; }
# the financial dashboard sees this journey's money: the STK top-up and the paybill payment
alice=$(staff_token alice)
[ "$(curl -fsS localhost:8080/v1/staff/finance -H "Authorization: Bearer $alice" | jq -r '.totals.deposits_kes >= 300')" = true ] \
  || { echo "finance does not show the deposits"; exit 1; }
[ "$(curl -sS -o /dev/null -w '%{http_code}' localhost:8080/v1/staff/members -H "Authorization: Bearer $paula")" = 403 ] \
  || { echo "a staff member could open the admin-only Staff page"; exit 1; }
# email and password sign-in (ADR 0019): the seeded temporary password opens a session that may only change it
pw_session=$(curl -fsS -X POST localhost:8080/v1/staff/session -H 'Content-Type: application/json' -H "Idempotency-Key: e2e-pw-$(date +%s%N)" \
  -d '{"provider":"password","email":"otis@staff.local","password":"local-dev-staff-2026"}')
[ "$(echo "$pw_session" | jq -r .member.must_change_password)" = true ] || { echo "password sign-in did not open a temporary session"; exit 1; }
otis=$(echo "$pw_session" | jq -r .token)
[ "$(curl -sS -o /dev/null -w '%{http_code}' localhost:8080/v1/staff/payouts -H "Authorization: Bearer $otis")" = 403 ] \
  || { echo "a temporary password opened more than the password change"; exit 1; }
[ "$(curl -sS -o /dev/null -w '%{http_code}' -X PUT localhost:8080/v1/staff/password -H "Authorization: Bearer $otis" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-pw-change-$(date +%s%N)" -d '{"current_password":"local-dev-staff-2026","new_password":"a long phrase for the payout queue"}')" = 204 ] \
  || { echo "the password change failed"; exit 1; }
[ "$(curl -sS -o /dev/null -w '%{http_code}' localhost:8080/v1/staff/payouts -H "Authorization: Bearer $otis")" = 200 ] \
  || { echo "the queue is still closed after the password change"; exit 1; }
[ "$(curl -sS -o /dev/null -w '%{http_code}' localhost:8080/v1/staff/me -H "Authorization: Bearer $token")" = 401 ] \
  || { echo "a player token reached the back office"; exit 1; }
echo "e2e smoke ok"
