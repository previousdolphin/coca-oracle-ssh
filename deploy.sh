#!/usr/bin/env bash
# Deploy to Fly with the FULL (Art) Bible embedded, WITHOUT committing the book
# to this public repo. The real book lives in bible.full.txt (gitignored); the
# committed bible.txt is a short public-safe placeholder. This swaps the full
# book in for the build, deploys, then restores the placeholder.
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f bible.full.txt ]]; then
  echo "ERROR: bible.full.txt (the full book) is missing — cannot deploy the full Oracle." >&2
  exit 1
fi

restore() { git checkout -- bible.txt 2>/dev/null || true; }
trap restore EXIT

cp bible.full.txt bible.txt
echo "→ embedded full Bible ($(wc -c < bible.txt) bytes); deploying…"
fly deploy --app coca-oracle-ssh --ha=false --wait-timeout 300 "$@"
echo "→ deployed; restoring placeholder bible.txt"
