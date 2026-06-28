#!/usr/bin/env bash
set -euo pipefail
BASE="${FLOW_SERVER_URL:-https://localhost:8080}"
TOK="$(make -s dev-token)"
curl() { command curl -ks -H "Authorization: Bearer $TOK" "$@"; }

echo "== GET /context (unresolved → 200, unresolved=true) =="
curl "$BASE/api/v1/context?remote=does-not-exist" | tee /dev/stderr | grep -q '"unresolved":true'

echo "== PUT /context/active (bound repo via ?node override) =="
# replace <slug> with a real bound engagement/repo slug from `flow node list`
curl -X PUT "$BASE/api/v1/context/active" -H 'Content-Type: application/json' \
  -d '{"node":"<slug>","title":"AC","body":"smoke where-I-was","tags":["smoke"]}' | grep -q '"id"'

echo "== GET /context?node=<slug> shows the activeContext =="
curl "$BASE/api/v1/context?node=<slug>" | grep -q 'smoke where-I-was'

echo "== POST /documents/{id}/pin (use an id from the response above) =="
# curl -X POST "$BASE/api/v1/documents/<id>/pin" -d '{"pinned":true}' -w '%{http_code}\n'

echo "smoke OK"
