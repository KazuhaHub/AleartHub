#!/usr/bin/env bash
# Boots a throwaway AlertHub server (isolated keys + db), runs the cross-language
# signing conformance check, then tears it down.
set -euo pipefail
cd "$(dirname "$0")/.."

KEYDIR="$(mktemp -d)"
DBDIR="$(mktemp -d)"
export ALERTHUB_KEY_DIR="$KEYDIR"
export ALERTHUB_DB_PATH="$DBDIR/t.db"
export ALERTHUB_ADMIN_TOKEN="dev-admin-token"

BIN="$(mktemp -d)/alerthub"
go build -o "$BIN" ./server
"$BIN" >/tmp/alerthub-conformance.log 2>&1 &
PID=$!
cleanup() { kill "$PID" 2>/dev/null || true; rm -rf "$KEYDIR" "$DBDIR"; }
trap cleanup EXIT

# wait for the server to answer
for _ in $(seq 1 60); do
  curl -fsS http://localhost:8080/pubkey >/dev/null 2>&1 && break
  sleep 0.2
done

node scripts/conformance.mjs
