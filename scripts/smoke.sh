#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"

echo "health:"
curl -fsS "$BASE/healthz"; echo

a=$(curl -fsS -X POST "$BASE/accounts" -d '{"initial_balance":1000}' | sed -E 's/.*"id":"([^"]+)".*/\1/')
b=$(curl -fsS -X POST "$BASE/accounts" -d '{"initial_balance":0}'    | sed -E 's/.*"id":"([^"]+)".*/\1/')
echo "created A=$a B=$b"

echo "deposit 500 -> A:"
curl -fsS -X POST "$BASE/accounts/$a/deposits" -d '{"amount":500}'; echo

echo "transfer 600 A -> B:"
curl -fsS -X POST "$BASE/transfers" -d "{\"from_account_id\":\"$a\",\"to_account_id\":\"$b\",\"amount\":600}"; echo

echo "B balance (expect 600):"
curl -fsS "$BASE/accounts/$b"; echo

echo "overdraft attempt (expect 422):"
curl -s -o /dev/null -w "%{http_code}\n" -X POST "$BASE/transfers" \
  -d "{\"from_account_id\":\"$b\",\"to_account_id\":\"$a\",\"amount\":999999}"
