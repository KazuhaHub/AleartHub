#!/usr/bin/env bash
# Fire a test alert through the running server (which signs + publishes).
# Usage: ./scripts/publish-test.sh [severity] [category]
set -euo pipefail
: "${ALERTHUB_ADMIN_TOKEN:=dev-admin-token}"
SEV="${1:-emergency}"
CAT="${2:-earthquake}"
curl -fsS -X POST http://localhost:8080/api/publish \
  -H "Authorization: Bearer ${ALERTHUB_ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"severity\":\"${SEV}\",\"category\":\"${CAT}\",\"title\":\"测试警报\",\"body\":\"这是一条本地测试。\",\"action\":\"\"}"
echo
