# Litestream Restore Runbook

When to use: the SQLite `server.db` on the PersistentVolumeClaim is corrupted,
deleted, or lost (PVC failure, accidental delete, bad migration). This runbook
restores the database from the Litestream S3 replica into the same PVC and
brings `flow-server` back up.

The chart's Litestream sidecar replicates continuously (`syncInterval: 1s`
by default — see `deploy/helm/flow-server/values.yaml`), so the recovery
point objective is "the last second or two before the failure".

## Prerequisites

- `kubectl` configured for the cluster + namespace (`flow` in the examples below).
- `litestream` CLI locally (`brew install benbjohnson/litestream/litestream`)
  OR a Pod image of `litestream/litestream:0.3.13` you can spawn.
- The replica URL — copy it out of the Helm release:
  ```bash
  helm get values flow-server -n flow -o yaml | grep replicaURL
  ```
  Matches `litestream.replicaURL` in `deploy/helm/flow-server/values.yaml`.
- The S3 credentials. They live in the chart's Secret:
  ```bash
  kubectl get secret flow-server-secret -n flow \
    -o jsonpath='{.data.LITESTREAM_ACCESS_KEY_ID}' | base64 -d
  kubectl get secret flow-server-secret -n flow \
    -o jsonpath='{.data.LITESTREAM_SECRET_ACCESS_KEY}' | base64 -d
  ```

Names below assume the default release name `flow-server` and namespace `flow`.
Adjust if your install differs (see `flow-server.fullname` in
`deploy/helm/flow-server/templates/_helpers.tpl`).

## Procedure

### 1. Inspect the replica

Confirm the replica has recent data BEFORE you touch the prod PVC. If the
replica is stale or empty, abort and triage instead — restoring an old
snapshot loses work.

```bash
export LITESTREAM_ACCESS_KEY_ID=...
export LITESTREAM_SECRET_ACCESS_KEY=...

litestream snapshots <replicaURL>
litestream wal-segments <replicaURL> | tail
```

Verify the newest snapshot/WAL timestamp is within minutes of "now". If it
is hours old, Litestream replication stopped at some point — find the
sidecar logs and fix the upstream issue before restoring.

### 2. Scale flow-server to 0

The PVC must not be open by any other process while we replace the file.
The Deployment uses `strategy: Recreate` (see deployment.yaml), so scaling
to 0 fully releases the volume.

```bash
kubectl scale deployment/flow-server --replicas=0 -n flow
kubectl wait --for=delete pod \
  -l app.kubernetes.io/name=flow-server \
  -n flow --timeout=60s
```

### 3. Spawn a restore Pod with the PVC mounted

```bash
kubectl apply -n flow -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: flow-restore
  labels:
    app.kubernetes.io/name: flow-restore
spec:
  restartPolicy: Never
  containers:
    - name: litestream
      image: litestream/litestream:0.3.13
      command: ["sleep", "3600"]
      envFrom:
        - secretRef:
            name: flow-server-secret
      volumeMounts:
        - name: flow-data
          mountPath: /var/lib/flow
  volumes:
    - name: flow-data
      persistentVolumeClaim:
        claimName: flow-server-data
EOF

kubectl wait --for=condition=Ready pod/flow-restore -n flow --timeout=60s
```

The same Secret that the sidecar uses gets mounted here, so the env vars
Litestream needs (`LITESTREAM_ACCESS_KEY_ID`,
`LITESTREAM_SECRET_ACCESS_KEY`) are already available.

### 4. Move the bad file aside

Do NOT delete. Keep `server.db.bad` around for postmortem analysis (and
as a fallback if the restore turns out worse than the broken state).

```bash
kubectl exec -n flow flow-restore -- \
  mv /var/lib/flow/server.db /var/lib/flow/server.db.bad
kubectl exec -n flow flow-restore -- \
  mv /var/lib/flow/server.db-wal /var/lib/flow/server.db-wal.bad 2>/dev/null || true
kubectl exec -n flow flow-restore -- \
  mv /var/lib/flow/server.db-shm /var/lib/flow/server.db-shm.bad 2>/dev/null || true
```

### 5. Restore from replica

```bash
kubectl exec -n flow flow-restore -- litestream restore \
  -o /var/lib/flow/server.db \
  '<replicaURL>'
```

If you need a specific point-in-time recovery (e.g. roll back past a bad
migration), add `-timestamp 2026-06-07T12:00:00Z`.

### 6. Sanity-check the restored file

```bash
kubectl exec -n flow flow-restore -- ls -lh /var/lib/flow/

# Schema check — the binary `sqlite3` is NOT in the litestream image, so
# pull a tiny alpine sidecar to inspect it. Alternative: scp the file out.
kubectl run flow-restore-sqlite -n flow --rm -it --restart=Never \
  --image alpine:3.19 \
  --overrides='{"spec":{"volumes":[{"name":"flow-data","persistentVolumeClaim":{"claimName":"flow-server-data"}}],"containers":[{"name":"sqlite","image":"alpine:3.19","stdin":true,"tty":true,"volumeMounts":[{"name":"flow-data","mountPath":"/var/lib/flow"}]}]}}' \
  -- sh -c "apk add --no-cache sqlite && sqlite3 /var/lib/flow/server.db \
  'SELECT name FROM sqlite_master WHERE type=\"table\" ORDER BY name;'"
```

Expect at minimum: `active_sessions`, `projects`, `repo_notes`, `repos`,
`sessions`, `users`, plus any `*_v1` tables added by later migrations.

Also spot-check row counts against what you remember the live system having:

```sql
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM sessions;
SELECT MAX(created_at) FROM sessions;
```

The `MAX(created_at)` is the truest measure of how fresh the replica was.

### 7. Cleanup and scale back up

```bash
kubectl delete pod flow-restore -n flow
kubectl scale deployment/flow-server --replicas=1 -n flow
kubectl wait --for=condition=Available deployment/flow-server \
  -n flow --timeout=120s
```

### 8. Smoke check

```bash
# HTTP healthcheck (uses the Ingress so this also proves DNS + TLS came back).
curl -fsS https://flow.example.com/healthz

# Re-sync a real client and verify it pulls.
flow sync
flow projects list

# WebUI walk-through:
# - Login through Authentik
# - Open /worktime → recent sessions match what you remember
# - Open /repos    → notes are there
```

If the smoke passes and the row counts look sane, the incident is closed.
Keep `server.db.bad` on the PVC for a week or two before deleting — it's
the only artifact of the failure state.

## What if restore fails

- **`unable to determine snapshot`** — replica URL is wrong, OR
  Litestream never wrote anything (chart's `litestream.enabled` was
  `false` at deploy time). Check `helm get values flow-server`.
- **`AccessDenied` from S3** — the secret rotated under you. Re-create
  the `flow-server-secret` (or `helm upgrade` with new values) and
  redo step 3.
- **Restored file is much smaller than expected** — the replica was
  truncated (S3 lifecycle policy aged old segments out). Set the
  bucket's retention to at least 30 days for safety; meanwhile, accept
  the older state OR fall back to `server.db.bad`.
- **Schema check shows missing tables** — the restore landed an
  outdated snapshot from before a migration. The flow-server pod will
  re-run migrations on startup, so this is usually self-healing — but
  verify by checking `flow-server` logs after scale-up.

As a last resort:

```bash
kubectl exec -n flow flow-restore -- \
  mv /var/lib/flow/server.db.bad /var/lib/flow/server.db
```

…and scale up. The corrupted file may still be readable enough for
`flow sync` to drain in-flight rows out of the cache DBs on each client,
and you can rebuild from there.

## Drill cadence

- **Local stack:** run `scripts/litestream-restore-drill.sh` (or
  `make drill-restore`) against the docker-compose stack before each
  Phase-2 milestone, and any time `litestream.yml` or the Helm chart's
  Litestream wiring changes.
- **Production:** walk through this runbook against a staging cluster
  at least once per quarter, and immediately after any S3 provider
  change. Track the date of the last successful drill in the project
  log.
