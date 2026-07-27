# Project Delete (decouple via SET NULL) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add `DELETE /api/v1/projects/{id}` + `flow project rm <slug>` so a project can be deleted; its work-sessions and documents are **decoupled (project_id → NULL)**, not deleted (no data loss); its bindings cascade away.

**Architecture:** Hexagonal (domain→usecase→ports→adapters). Migration 0012 changes the `work_sessions.project_id` and `documents.project_id` foreign keys from the implicit `NO ACTION`/`RESTRICT` to `ON DELETE SET NULL`. A `DeleteProject` usecase calls `ProjectStore.Delete`; everything else follows the existing binding/project patterns.

**Tech Stack:** Go, pgx/pgxpool, goose migrations, net/http mux, cobra.

## Global Constraints

- Module `github.com/serverkraken/flow`. Hexagonal; one responsibility per file.
- Migrations need `-- +goose Up` / `-- +goose Down` annotations; the **table is `work_sessions`** (not `sessions`). FK names (verified against the dev DB): `work_sessions_project_id_fkey`, `documents_project_id_fkey`.
- Decouple semantics: deleting a project sets `work_sessions.project_id=NULL` and `documents.project_id=NULL` for its rows; `project_bindings` cascade-delete (already `ON DELETE CASCADE`). **No session/document row is ever deleted.**
- CLI verb is `rm` (like `flow dayoff rm`), with a confirmation prompt.
- Owner-scoped everywhere. `make ci` green (~80% gate) at the end.
- Project identified server-side by `{id}`; CLI resolves slug→id via the existing `resolveSlug`.

---

### Task 1: `usecase.DeleteProject` + `ports.ProjectStore.Delete` + fake

**Files:** Modify `internal/ports/ports.go` (add `Delete` to `ProjectStore`); Create `internal/usecase/delete_project.go`; Modify `internal/testutil/fakes.go` (`FakeProjectStore.Delete`); Test `internal/usecase/delete_project_test.go` + extend `internal/testutil/fakes_test.go`.

**Interfaces (Produces):**
- `ProjectStore` gains `Delete(ctx context.Context, ownerID, id string) error` (returns `ports.ErrProjectNotFound` when the row doesn't exist/owner mismatch).
- `type DeleteProject struct{ Projects ports.ProjectStore }`; `func (uc DeleteProject) Execute(ctx, ownerID, id string) error`.

- [ ] **Step 1: Failing test** — `FakeProjectStore.Delete` removes the project (owner-scoped); deleting a missing id → `ErrProjectNotFound`; `DeleteProject.Execute` delegates.

```go
func TestDeleteProject(t *testing.T) {
	ps := testutil.NewFakeProjectStore()
	p, _ := ps.Create(context.Background(), domain.Project{ID: "p1", OwnerID: "u", Slug: "x"})
	uc := usecase.DeleteProject{Projects: ps}
	if err := uc.Execute(context.Background(), "u", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Get(context.Background(), "u", p.ID); err == nil {
		t.Fatal("project should be gone")
	}
	if err := uc.Execute(context.Background(), "u", "missing"); err == nil {
		t.Fatal("deleting a missing project should error")
	}
}
```

- [ ] **Step 2:** Run → FAIL (`Delete` undefined). `go test ./internal/usecase/ ./internal/testutil/ -run Delete -v`.
- [ ] **Step 3:** Add `Delete` to the `ProjectStore` interface; implement `FakeProjectStore.Delete` (owner-scoped map delete, `ErrProjectNotFound` if absent — mirror `FakeProjectStore` style); write `delete_project.go` (`return uc.Projects.Delete(ctx, ownerID, id)`).
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5: Commit** `feat(usecase): DeleteProject + ProjectStore.Delete + fake`.

---

### Task 2: migration 0012 (SET NULL FKs) + `pgstore.ProjectStore.Delete`

**Files:** Create `internal/adapter/pgstore/migrations/0012_project_ondelete_setnull.sql`; Modify `internal/adapter/pgstore/projects.go` (add `Delete`); Test `internal/adapter/pgstore/projects_test.go` (extend, Docker).

**Interfaces:** Produces `pgstore.ProjectStore.Delete` implementing the port.

- [ ] **Step 1: Migration** (exact FK names verified against the dev DB):

```sql
-- +goose Up
ALTER TABLE work_sessions DROP CONSTRAINT work_sessions_project_id_fkey;
ALTER TABLE work_sessions ADD CONSTRAINT work_sessions_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE documents DROP CONSTRAINT documents_project_id_fkey;
ALTER TABLE documents ADD CONSTRAINT documents_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE work_sessions DROP CONSTRAINT work_sessions_project_id_fkey;
ALTER TABLE work_sessions ADD CONSTRAINT work_sessions_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id);
ALTER TABLE documents DROP CONSTRAINT documents_project_id_fkey;
ALTER TABLE documents ADD CONSTRAINT documents_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id);
```

- [ ] **Step 2: Failing Docker test** (mirror the existing `projects_test.go` harness with `startPG(t)`): create a project; create a `work_sessions` row AND a `documents` row referencing it (+ a `project_bindings` remote row); `ProjectStore.Delete` the project; assert the project is gone, the session and document rows still EXIST with `project_id IS NULL`, and the binding row is gone (cascade). Also assert deleting a missing id → `ErrProjectNotFound`.

- [ ] **Step 3:** Implement `pgstore.ProjectStore.Delete` (mirror `projects.go`: `DELETE FROM projects WHERE owner_id=$1 AND id=$2`; `RowsAffected()==0 → ports.ErrProjectNotFound`).
- [ ] **Step 4:** Run `go test ./internal/adapter/pgstore/ -run 'Project' -v` (Docker) → PASS, incl. the new SET-NULL/cascade assertions.
- [ ] **Step 5: Commit** `feat(pgstore): ProjectStore.Delete + migration 0012 (project FKs ON DELETE SET NULL)`.

---

### Task 3: REST `DELETE /projects/{id}` + apiclient

**Files:** Modify `internal/adapter/httpserver/server.go` (add `DeleteProject` Server field + route) + the project handler file (add `handleDeleteProject`); Modify `internal/adapter/apiclient/client.go` (or a project client file) — add `DeleteProject`; Test the handler (httptest) + apiclient.

**Interfaces:**
- Consumes `usecase.DeleteProject` as a `Server` field.
- Produces route `DELETE /api/v1/projects/{id}` → 204; `ErrProjectNotFound` → 404. And `apiclient.DeleteProject(ctx, id string) error`.

- [ ] **Step 1: Failing httptest** — `DELETE /api/v1/projects/{id}` for an existing project → 204 and the project is gone from the fake store; unknown id → 404. (Mirror the existing project/handler test harness.)
- [ ] **Step 2:** Run → FAIL (route 404 because unregistered).
- [ ] **Step 3:** Add `handleDeleteProject` (`u,_ := userFrom(ctx)`; `s.DeleteProject.Execute(ctx, u.ID, r.PathValue("id"))`; map `ErrProjectNotFound`→404, success→`w.WriteHeader(204)`). Register `mux.Handle("DELETE /api/v1/projects/{id}", s.auth(http.HandlerFunc(s.handleDeleteProject)))` beside the other project routes. Add the `DeleteProject usecase.DeleteProject` field to the `Server` struct. Add `apiclient.DeleteProject` using `c.do(ctx, http.MethodDelete, "/api/v1/projects/"+id, nil, nil)`.
- [ ] **Step 4:** Run the handler + apiclient tests → PASS; `go test ./internal/adapter/httpserver/ ./internal/adapter/apiclient/`.
- [ ] **Step 5: Commit** `feat(httpserver,apiclient): DELETE project endpoint`.

---

### Task 4: CLI `flow project rm <slug>`

**Files:** Modify `cmd/flow/project.go` (register `projectRmCmd`); add the command (in `project.go` or a small file); Test `cmd/flow/project_test.go` (or the projectbind test file).

**Interfaces:** `flow project rm <slug>` resolves slug→id (`resolveSlug`), prompts a confirmation (since it decouples sessions/docs), then `apiclient.DeleteProject`.

- [ ] **Step 1: Failing test** — a `runProjectRm(ctx, c, slug)` helper (httptest-backed apiclient) deletes the resolved project; unknown slug → `no project with slug …`. Keep the confirmation in the cobra `RunE` (interactive), not in the testable helper.

```go
func TestRunProjectRm(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/projects":
			_ = json.NewEncoder(w).Encode([]domain.Project{{ID: "p1", Slug: "x"}})
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/projects/p1":
			deleted = r.URL.Path
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	c := apiclient.New(srv.URL, "tkn")
	if err := runProjectRm(context.Background(), c, "x"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/api/v1/projects/p1" {
		t.Fatalf("deleted = %q", deleted)
	}
}
```

- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement `runProjectRm` (resolveSlug→id, `c.DeleteProject`) + `projectRmCmd` (`Use: "rm <slug>"`, `Args: cobra.ExactArgs(1)`, a confirm prompt in `RunE` — e.g. read stdin "delete project <slug>? its sessions/documents will be kept but un-assigned [y/N]"; on non-`y` abort with a message; honor a `--yes`/`-y` flag to skip the prompt for scripting). Register in `projectCmd()`.
- [ ] **Step 4:** Run `go test ./cmd/flow/ -run ProjectRm -v` → PASS.
- [ ] **Step 5: Commit** `feat(cli): flow project rm <slug> (delete project, keep sessions/docs un-assigned)`.

---

### Task 5: wire into composition root + live gate

**Files:** Modify `cmd/flow-server/main.go`.

- [ ] **Step 1: Wire** `DeleteProject: usecase.DeleteProject{Projects: projectStore}` into the `Server{…}` literal (beside `CreateProject`/`SetProjectRate`).
- [ ] **Step 2:** `go build ./...`; **full `make ci`** green (lint + templ-in-sync + tests + coverage gate). Confirm no binding/project `Server` field is left nil.
- [ ] **Step 3: Commit** `feat(flow-server): wire DeleteProject into composition root`.
- [ ] **Step 4: Live done-gate (controller):** against a temp dev server (auto-migrates 0012): create a project + a worktime session on it + a document on it + a binding; `flow project rm <slug> --yes`; assert the project is gone, `flow session list`/the session still exists with no project, the document still exists (project-less), `flow project bindings` no longer lists its binding. (The controller runs this; the implementer stops at Step 3.)

---

## Self-review (done)
- Spec coverage: delete endpoint (T3) + CLI rm (T4) + decouple-SET-NULL (T2 migration) + no-data-loss (T2 Docker test asserts rows survive with NULL) + bindings cascade (existing + T2 assertion) + wiring (T5).
- No placeholders. Types consistent: `DeleteProject.Execute(ctx, ownerID, id)` used identically across usecase/handler/wiring; `ProjectStore.Delete(ctx, ownerID, id)` across port/fake/pgstore.
- Out of scope (separate follow-up): the "(no project)" isolation filter for the docs list (the docs list has no project filter today; orphans remain visible in the full list + re-assignable via existing edit).
