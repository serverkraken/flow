#!/usr/bin/env bash
# Replace the local dev database from a read-only production snapshot and map
# one Authentik subject to the static Dex identity. Production is never mutated.
set -Eeuo pipefail

cd "$(dirname "$0")/.."

PROD_CONTEXT="${FLOW_PROD_KUBE_CONTEXT:-admin@study-en75-mybackbone-cc}"
PROD_NAMESPACE="${FLOW_PROD_NAMESPACE:-flow}"
PROD_CLUSTER="${FLOW_PROD_DB_CLUSTER:-flow-db}"
PROD_DATABASE="${FLOW_PROD_DATABASE:-flow}"
PROD_DB_USER="${FLOW_PROD_DB_USER:-postgres}"
PROD_OIDC_SUB="${FLOW_PROD_OIDC_SUB:-msoent}"
if [[ ! "$PROD_OIDC_SUB" =~ ^[A-Za-z0-9._:@/-]+$ ]]; then
	echo "dev-refresh-prod: production OIDC subject contains unsupported characters" >&2
	exit 1
fi

LOCAL_HOST="${FLOW_DEV_DB_HOST:-localhost}"
LOCAL_PORT="${FLOW_DEV_DB_PORT:-5432}"
LOCAL_DATABASE="${FLOW_DEV_DATABASE:-flow}"
LOCAL_DB_USER="${FLOW_DEV_DB_USER:-flow}"
LOCAL_DB_PASSWORD="${FLOW_DEV_DB_PASSWORD:-flow}"
LOCAL_SERVER_PORT="${FLOW_DEV_SERVER_PORT:-8080}"
LOCAL_DSN="postgres://${LOCAL_DB_USER}:${LOCAL_DB_PASSWORD}@${LOCAL_HOST}:${LOCAL_PORT}/${LOCAL_DATABASE}?sslmode=disable"

for command in mise podman pg_restore psql lsof go; do
	command -v "$command" >/dev/null || { echo "dev-refresh-prod: missing command: $command" >&2; exit 1; }
done

if lsof -nP -iTCP:"$LOCAL_SERVER_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
	echo "dev-refresh-prod: local flow-server is listening on :$LOCAL_SERVER_PORT; stop make dev-run first" >&2
	exit 1
fi

if ! podman compose -f deploy/dev/compose.yml exec -T db pg_isready -U "$LOCAL_DB_USER" -d "$LOCAL_DATABASE" >/dev/null 2>&1; then
	echo "dev-refresh-prod: local dev Postgres is not ready; run make dev-up first" >&2
	exit 1
fi

set -a
. deploy/dev/flow.env
set +a
DATABASE_URL="$LOCAL_DSN"
export DATABASE_URL
DEX_SUB="${FLOW_ALLOWED_SUBS%%,*}"
if [[ -z "$DEX_SUB" || "$DEX_SUB" == *" "* ]]; then
	echo "dev-refresh-prod: deploy/dev/flow.env has no single usable Dex subject" >&2
	exit 1
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/flow-refresh-prod.XXXXXX")"
chmod 700 "$tmpdir"
prod_dump="$tmpdir/prod.dump"
local_backup="$tmpdir/local-before-refresh.dump"
local_mutated=0
success=0

reset_local_database() {
	podman compose -f deploy/dev/compose.yml exec -T db \
		dropdb --force --if-exists -U "$LOCAL_DB_USER" "$LOCAL_DATABASE"
	podman compose -f deploy/dev/compose.yml exec -T db \
		createdb -U "$LOCAL_DB_USER" -O "$LOCAL_DB_USER" "$LOCAL_DATABASE"
}

cleanup() {
	status=$?
	remove_tmp=1
	if [[ "$status" -ne 0 && "$local_mutated" -eq 1 && -s "$local_backup" ]]; then
		echo "dev-refresh-prod: refresh failed; restoring previous local database" >&2
		set +e
		reset_local_database && podman compose -f deploy/dev/compose.yml exec -T db \
			pg_restore --exit-on-error --no-owner --no-privileges \
			-U "$LOCAL_DB_USER" -d "$LOCAL_DATABASE" <"$local_backup"
		if [[ $? -ne 0 ]]; then
			remove_tmp=0
			echo "dev-refresh-prod: automatic rollback failed; secure backup retained at $local_backup" >&2
		fi
		set -e
	fi
	if [[ "$remove_tmp" -eq 1 ]]; then
		rm -rf "$tmpdir"
	fi
	if [[ "$success" -eq 1 ]]; then
		echo "dev-refresh-prod: temporary production dump and local backup removed"
	fi
	exit "$status"
}
trap cleanup EXIT

primary="$(mise exec -- kubectl --context "$PROD_CONTEXT" -n "$PROD_NAMESPACE" \
	get clusters.postgresql.cnpg.io "$PROD_CLUSTER" -o jsonpath='{.status.currentPrimary}')"
if [[ -z "$primary" ]]; then
	echo "dev-refresh-prod: CloudNativePG cluster has no current primary" >&2
	exit 1
fi

prod_user_count="$(mise exec -- kubectl --context "$PROD_CONTEXT" -n "$PROD_NAMESPACE" \
	exec "$primary" -c postgres -- psql -U postgres -d "$PROD_DATABASE" -Atv ON_ERROR_STOP=1 \
	-c "BEGIN READ ONLY; SELECT count(*) FROM users WHERE oidc_sub = '$PROD_OIDC_SUB'; COMMIT;" | sed -n '2p')"
if [[ "$prod_user_count" != "1" ]]; then
	echo "dev-refresh-prod: expected exactly one production user for subject '$PROD_OIDC_SUB', got '$prod_user_count'" >&2
	exit 1
fi

echo "dev-refresh-prod: backing up current local database"
podman compose -f deploy/dev/compose.yml exec -T db \
	pg_dump --format=custom --no-owner --no-privileges \
	-U "$LOCAL_DB_USER" -d "$LOCAL_DATABASE" >"$local_backup"
chmod 600 "$local_backup"
pg_restore --list "$local_backup" >/dev/null

echo "dev-refresh-prod: dumping production from $PROD_CONTEXT/$PROD_NAMESPACE/$primary (read-only)"
mise exec -- kubectl --context "$PROD_CONTEXT" -n "$PROD_NAMESPACE" \
	exec "$primary" -c postgres -- pg_dump -U "$PROD_DB_USER" -d "$PROD_DATABASE" \
	--format=custom --no-owner --no-privileges >"$prod_dump"
chmod 600 "$prod_dump"
pg_restore --list "$prod_dump" >/dev/null

echo "dev-refresh-prod: replacing local database"
local_mutated=1
reset_local_database
podman compose -f deploy/dev/compose.yml exec -T db \
	pg_restore --exit-on-error --no-owner --no-privileges \
	-U "$LOCAL_DB_USER" -d "$LOCAL_DATABASE" <"$prod_dump"

mapped="$(PGPASSWORD="$LOCAL_DB_PASSWORD" psql "$LOCAL_DSN" -Atv ON_ERROR_STOP=1 \
	-v prod_sub="$PROD_OIDC_SUB" -v dex_sub="$DEX_SUB" <<'SQL'
BEGIN;
UPDATE users SET oidc_sub = :'dex_sub' WHERE oidc_sub = :'prod_sub';
SELECT count(*) FROM users WHERE oidc_sub = :'dex_sub';
COMMIT;
SQL
)"
mapped="$(printf '%s\n' "$mapped" | sed -n '3p')"
if [[ "$mapped" != "1" ]]; then
	echo "dev-refresh-prod: Dex mapping validation failed (matches=$mapped)" >&2
	exit 1
fi

echo "dev-refresh-prod: applying current checkout migrations"
FLOW_MIGRATE_ONLY=1 go run ./cmd/flow-server

validation="$(PGPASSWORD="$LOCAL_DB_PASSWORD" psql "$LOCAL_DSN" -Atv ON_ERROR_STOP=1 <<'SQL'
BEGIN READ ONLY;
SELECT max(version_id) FROM goose_db_version WHERE is_applied;
SELECT count(*) FROM users;
SELECT count(*) FROM nodes;
SELECT count(*) FROM documents;
SELECT count(*) FROM work_sessions;
SELECT count(*) FROM work_sessions a JOIN work_sessions b
  ON a.owner_id=b.owner_id AND a.id<b.id
 AND tstzrange(a.start_at,COALESCE(a.stop_at,'infinity'::timestamptz),'[)')
     && tstzrange(b.start_at,COALESCE(b.stop_at,'infinity'::timestamptz),'[)');
COMMIT;
SQL
)"
printf '%s\n' "$validation" | sed -n '2,7p' | {
	read -r schema_version
	read -r users
	read -r nodes
	read -r documents
	read -r sessions
	read -r overlaps
	if [[ "$overlaps" != "0" ]]; then
		echo "dev-refresh-prod: overlap validation failed ($overlaps pairs)" >&2
		exit 1
	fi
	printf 'dev-refresh-prod: ready (schema=%s users=%s nodes=%s documents=%s sessions=%s overlaps=%s)\n' \
		"$schema_version" "$users" "$nodes" "$documents" "$sessions" "$overlaps"
}

success=1
