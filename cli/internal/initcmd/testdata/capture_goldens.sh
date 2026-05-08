#!/bin/sh
# Captures bash init.sh .env output for parity testing.
# Run once to regenerate goldens. CI does NOT run this.
#
# Goldens lock the byte-exact .env produced by the legacy init.sh so that
# Go renderEnv can be tested for parity. lets-7vtaw Phase 4b: init.sh stays
# in plugin source through this phase; lets-8ilsl will eventually delete it,
# at which point these goldens become the source of truth.
set -e

REPO=$(git rev-parse --show-toplevel)
INIT_SH="${REPO}/plugins/lets/scripts/lets/init.sh"
OUT_DIR="${REPO}/cli/internal/initcmd/testdata"

if [ ! -x "$INIT_SH" ] && [ ! -f "$INIT_SH" ]; then
  echo "ERROR: init.sh not found at $INIT_SH" >&2
  exit 1
fi

capture() {
  name=$1; shift
  tmp=$(mktemp -d)
  cd "$tmp"
  git init -q
  bash "$INIT_SH" --quiet --skip-beads "$@" > /dev/null 2>&1 || true
  if [ ! -f .lets/.env ]; then
    echo "ERROR: .lets/.env not produced by init.sh for golden '$name' (args: $*)" >&2
    rm -rf "$tmp"
    exit 1
  fi
  cp .lets/.env "${OUT_DIR}/golden_env_${name}.txt"
  rm -rf "$tmp"
}

capture default --language English --merge-branch main
capture ukrainian --language Ukrainian --merge-branch main
capture github --language English --merge-branch main --github

echo "Goldens captured in ${OUT_DIR}/"
ls -la "${OUT_DIR}/"golden_env_*.txt
