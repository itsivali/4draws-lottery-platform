#!/usr/bin/env bash
# No checked-in deployment or workflow may switch the service to real money (ADR 0027): OPERATING_MODE=live is set by
# hand on the production service, after licensing, with two admins approving a live request, never by a file in this
# repository. Fails listing every place that sets it. Usage: scripts/check-operating-mode.sh [dir] (default: the repo).
set -euo pipefail
root="${1:-$(git rev-parse --show-toplevel)}"
cd "$root"
files=()
while IFS= read -r f; do files+=("$f"); done < <(
  # directories that do not exist are skipped (find fails on them; that must not end the list)
  for d in .github/workflows deploy infra; do
    [ -d "$d" ] && find "$d" -type f \( -name '*.yml' -o -name '*.yaml' -o -name '*.env' -o -name '*.sh' \)
  done
  find . -maxdepth 1 -type f \( -name 'render.yaml' -o -name 'docker-compose*.yml' -o -name '.env.*example' \)
)
bad=0
for f in "${files[@]}"; do
  # KEY: live, KEY=live, and Render's "- key: OPERATING_MODE" followed by "value: live"
  if grep -nEi 'OPERATING_MODE["'\'']?[[:space:]]*[:=][[:space:]]*["'\'']?live\b' "$f"; then
    echo "  ^ $f sets OPERATING_MODE=live" >&2; bad=1
  fi
  if awk 'tolower($0) ~ /key:[[:space:]]*operating_mode[[:space:]]*$/ { k = NR; next } k && NR == k + 1 && tolower($0) ~ /value:[[:space:]]*["'\'']?live/ { print FILENAME ":" NR ": " $0; found = 1 } END { exit !found }' "$f"; then
    echo "  ^ $f sets OPERATING_MODE=live" >&2; bad=1
  fi
done
if [ "$bad" -ne 0 ]; then
  echo "OPERATING_MODE=live must never be set by a file in the repository (ADR 0027; docs/runbooks/operating-mode.md)." >&2
  exit 1
fi
echo "operating mode: no checked-in file switches to live (${#files[@]} files checked)"
