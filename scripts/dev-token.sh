#!/usr/bin/env bash
# Mint a Dex id_token for the dev user via the OAuth2 password grant, for the
# TUI/CLI FLOW_TOKEN bearer (proper device-flow login lands in M1b). Prints the
# id_token to stdout; decoded claims go to stderr for inspection.
#   export FLOW_TOKEN=$(scripts/dev-token.sh)
set -euo pipefail
ISSUER="${ISSUER:-http://localhost:5556/dex}"
CLIENT_ID="${CLIENT_ID:-flow-dev}"
CLIENT_SECRET="${CLIENT_SECRET:-flow-dev-secret}"
USERNAME="${USERNAME:-msoent@dev.local}"
PASSWORD="${PASSWORD:-password}"

resp=$(curl -fsS -u "$CLIENT_ID:$CLIENT_SECRET" \
  -d grant_type=password \
  -d "username=$USERNAME" \
  -d "password=$PASSWORD" \
  --data-urlencode "scope=openid profile email" \
  "$ISSUER/token")

idt=$(printf '%s' "$resp" | sed -n 's/.*"id_token":"\([^"]*\)".*/\1/p')
[ -n "$idt" ] || { echo "dev-token: no id_token in response: $resp" >&2; exit 1; }

# decode the JWT payload (base64url) to stderr — handy to confirm sub/username/aud
payload=$(printf '%s' "$idt" | cut -d. -f2 | tr '_-' '/+')
case $(( ${#payload} % 4 )) in 2) payload="$payload==";; 3) payload="$payload=";; esac
claims=$(printf '%s' "$payload" | { base64 -d 2>/dev/null || base64 -D 2>/dev/null; } || true)
[ -n "$claims" ] && printf 'claims: %s\n' "$claims" >&2

printf '%s\n' "$idt"
