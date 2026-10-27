#!/usr/bin/env bash
# Runs the web frontends against the local backend (scripts/dev-backend.sh): the player web app (Flutter) and,
# optionally, the staff back office (React). Stop with Ctrl-C.
#
#   scripts/dev-frontend.sh               player web app, release build, on http://localhost:8088
#   scripts/dev-frontend.sh --hot         player web app in debug mode with hot reload (slower to load)
#   scripts/dev-frontend.sh --backoffice  the back office on http://localhost:5173 instead
#   scripts/dev-frontend.sh --all         both
#
# Both origins are already allowed by the local api's CORS_ALLOWED_ORIGINS (docker-compose.yml).
set -euo pipefail
root=$(git rev-parse --show-toplevel)
API=http://localhost:8080
WEB_PORT=8088

mode=${1:-web}
case "$mode" in
  web|--hot|--backoffice|--all) ;;
  *) echo "usage: $0 [--hot|--backoffice|--all]" >&2; exit 2 ;;
esac

curl -fsS "$API/readyz" >/dev/null 2>&1 || { echo "The backend is not running: start it with scripts/dev-backend.sh" >&2; exit 1; }
command -v mise >/dev/null || { echo "mise is required (https://mise.jdx.dev): then run mise install" >&2; exit 1; }

pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; }
trap cleanup EXIT INT TERM

player_web() {
  cd "$root/apps/mobile"
  mise exec -- flutter pub get >/dev/null
  if [ "$mode" = --hot ]; then
    echo "Player web app (debug, hot reload: press r in this terminal): http://localhost:$WEB_PORT"
    mise exec -- flutter run -d web-server --web-hostname localhost --web-port "$WEB_PORT" \
      --dart-define=API_BASE_URL="$API"
  else
    echo "Building the player web app (release; a few minutes the first time)..."
    mise exec -- flutter build web --release --dart-define=API_BASE_URL="$API" >/dev/null
    echo "Player web app: http://localhost:$WEB_PORT"
    python3 -m http.server "$WEB_PORT" --bind 127.0.0.1 --directory build/web >/dev/null 2>&1
  fi
}

back_office() {
  cd "$root/apps/backoffice"
  # a copy from before staff signed in with Google/Apple (ADR 0018) points at settings that no longer exist
  grep -qs VITE_DEV_OIDC_AUTHORITY .env.local || cp .env.example .env.local
  [ -d node_modules ] || mise exec -- npm ci
  echo "Back office: http://localhost:5173 (local staff realm: ada (admin), alice, bob, kim, paula, otis, tara, tom, sam; password local-dev-staff)"
  mise exec -- npx vite --port 5173 --strictPort
}

case "$mode" in
  web|--hot) player_web ;;
  --backoffice) back_office ;;
  --all)
    back_office & pids+=($!)
    player_web
    ;;
esac
