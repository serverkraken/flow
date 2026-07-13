# flow-mcp `flow_upload_artifact` `path` Parameter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the MCP tool `flow_upload_artifact` a `path` parameter (mutually exclusive with `base64`) so agents upload files by reference instead of piping base64 through model output.

**Architecture:** The flow-mcp process runs locally and already has filesystem access. In `path`-mode it `os.ReadFile`s the file and uploads the bytes through the same `apiclient.UploadArtifact`/`UploadFreeArtifact` the tool already calls — exactly like the CLI's `flow artifact add`. The CLI's mime-guess logic moves into a shared `internal/artifactfile` package both binaries import. No `flow-server` change.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, `internal/adapter/apiclient`, stdlib `mime`/`os`/`path/filepath`/`encoding/base64`.

## Global Constraints

- No `flow-server` change; no goose migration; no `apiclient` change — this is `cmd/flow-mcp` + `cmd/flow` + a new `internal/artifactfile` only.
- No client-side size/MIME pre-check — the server stays the source of truth; surface its errors.
- Server-side upload REST field is `dataBase64`; the `apiclient` already base64-encodes the `[]byte` — the handler passes raw bytes, never base64 for `path`-mode.
- `make ci` is the gate (golangci-lint 0, verify-generate clean, coverage gate, build). **Never run `make fmt`.** pgstore Docker tests need Podman (`DOCKER_HOST`).
- Commit trailers on every commit:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01VcCHz2MiiWUAAsiSwehgsD
  ```
- Base64-mode behavior is unchanged and MUST stay backward-compatible (`name` + `mime` required, invalid base64 errors).

---

## Task 1: `internal/artifactfile.GuessMime` — extract + share the mime guess

**Files:**
- Create: `internal/artifactfile/artifactfile.go`
- Test: `internal/artifactfile/artifactfile_test.go`
- Modify: `cmd/flow/artifact.go` (delete `resolveArtifactMime`, call the shared helper)

**Interfaces:**
- Produces: `func GuessMime(path, override string) string` in package `artifactfile` — returns `override` when non-empty, else `mime.TypeByExtension(filepath.Ext(path))` with any `; charset=…` stripped, else `"application/octet-stream"`. Task 2 consumes this.

- [ ] **Step 1: Write the failing test**

Create `internal/artifactfile/artifactfile_test.go`:

```go
package artifactfile

import "testing"

func TestGuessMime(t *testing.T) {
	cases := []struct{ name, path, override, want string }{
		{"override wins over extension", "logo.png", "application/x-thing", "application/x-thing"},
		{"png from extension", "/a/b/logo.png", "", "image/png"},
		{"pdf from extension", "doc.pdf", "", "application/pdf"},
		{"charset parameter stripped", "style.css", "", "text/css"},
		{"unknown extension falls back", "data.zzz", "", "application/octet-stream"},
		{"no extension falls back", "README", "", "application/octet-stream"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GuessMime(c.path, c.override); got != c.want {
				t.Fatalf("GuessMime(%q, %q) = %q, want %q", c.path, c.override, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/artifactfile/ -run TestGuessMime -v`
Expected: FAIL — build error, package/`GuessMime` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/artifactfile/artifactfile.go`:

```go
// Package artifactfile holds file-side helpers shared by the flow CLI and the
// flow-mcp server for turning a filesystem path into an artifact upload.
package artifactfile

import (
	"mime"
	"path/filepath"
	"strings"
)

// GuessMime returns override when set, else a best-effort guess from path's
// extension (stripping any "; charset=…" parameter), else the catch-all
// application/octet-stream. No content sniffing — the server validates the
// final MIME type against the allowed set.
func GuessMime(path, override string) string {
	if override != "" {
		return override
	}
	if m := mime.TypeByExtension(filepath.Ext(path)); m != "" {
		if i := strings.Index(m, ";"); i >= 0 {
			m = strings.TrimSpace(m[:i])
		}
		return m
	}
	return "application/octet-stream"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/artifactfile/ -run TestGuessMime -v`
Expected: PASS (all six subtests).

- [ ] **Step 5: Refactor the CLI to use the shared helper**

In `cmd/flow/artifact.go`:

1. Delete the whole `resolveArtifactMime` function (the `// resolveArtifactMime returns override …` comment block through its closing brace).
2. In `runArtifactAdd`, change the mime line from:
   ```go
   mimeType := resolveArtifactMime(path, mimeFlag)
   ```
   to:
   ```go
   mimeType := artifactfile.GuessMime(path, mimeFlag)
   ```
3. Add the import `"github.com/serverkraken/flow/internal/artifactfile"` to the import block and remove the now-unused `"mime"` import (keep `"path/filepath"` — still used by `filepath.Base`).

- [ ] **Step 6: Run the CLI tests to verify the refactor is behavior-neutral**

Run: `go test ./cmd/flow/ -run Artifact -v`
Expected: PASS — existing artifact tests unchanged.

- [ ] **Step 7: Verify the tree builds and lints**

Run: `go build ./... && go vet ./internal/artifactfile/ ./cmd/flow/`
Expected: no output (success).

- [ ] **Step 8: Commit**

```bash
git add internal/artifactfile/ cmd/flow/artifact.go
git commit -m "refactor(artifact): extract GuessMime into internal/artifactfile

Shared by the flow CLI and (next) the flow-mcp upload tool. Behavior-neutral.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01VcCHz2MiiWUAAsiSwehgsD"
```

---

## Task 2: `flow_upload_artifact` gains `path`

**Files:**
- Modify: `cmd/flow-mcp/tools_artifacts.go` (the `uploadArtifactIn` struct + `uploadArtifact` handler)
- Modify: `cmd/flow-mcp/server.go` (the tool `Description`)
- Test: `cmd/flow-mcp/tools_artifacts_test.go` (add path-mode loopback tests)

**Interfaces:**
- Consumes: `artifactfile.GuessMime(path, override string) string` from Task 1.
- Consumes (existing, unchanged): `h.artifactNode`, `h.do`, `h.resultErr`, `c.UploadArtifact`, `c.UploadFreeArtifact`, the `errFreeNodeExclusive` const, and the test helpers `authedArtifactServer`/`unboundArtifactServer`/`callText`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/flow-mcp/tools_artifacts_test.go`:

```go
func TestLoopback_UploadArtifact_Path_Node(t *testing.T) {
	sess := authedArtifactServer(t)
	p := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(p, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{"path": p})
	if res.IsError {
		t.Fatalf("path upload errored: %s", out)
	}
	if !strings.Contains(out, "pic.png") || !strings.Contains(out, "image/png") {
		t.Fatalf("path upload result = %q, want basename name + guessed mime", out)
	}
}

func TestLoopback_UploadArtifact_Path_Free(t *testing.T) {
	sess := unboundArtifactServer(t)
	p := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.4 body"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{"path": p, "free": true})
	if res.IsError {
		t.Fatalf("free path upload errored: %s", out)
	}
	if !strings.Contains(out, "freeslug") || !strings.Contains(out, "doc.pdf") || !strings.Contains(out, "application/pdf") {
		t.Fatalf("free path upload result = %q", out)
	}
}

func TestLoopback_UploadArtifact_Path_Overrides(t *testing.T) {
	sess := authedArtifactServer(t)
	p := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"path": p, "name": "custom.bin", "mime": "application/x-custom",
	})
	if res.IsError {
		t.Fatalf("path upload with overrides errored: %s", out)
	}
	if !strings.Contains(out, "custom.bin") || !strings.Contains(out, "application/x-custom") {
		t.Fatalf("overrides not applied: %q", out)
	}
}

func TestLoopback_UploadArtifact_PathAndBase64(t *testing.T) {
	sess := authedArtifactServer(t)
	p := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"path": p, "base64": base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if !res.IsError || !strings.Contains(out, "either path or base64") {
		t.Fatalf("path+base64 = (IsError=%v, %q), want a mutual-exclusion error", res.IsError, out)
	}
}

func TestLoopback_UploadArtifact_NeitherPathNorBase64(t *testing.T) {
	sess := authedArtifactServer(t)
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{"name": "x", "mime": "image/png"})
	if !res.IsError || !strings.Contains(out, "either path or base64") {
		t.Fatalf("neither path nor base64 = (IsError=%v, %q), want a path-or-base64 error", res.IsError, out)
	}
}

func TestLoopback_UploadArtifact_Path_Unreadable(t *testing.T) {
	sess := authedArtifactServer(t)
	res, out := callText(t, sess, "flow_upload_artifact", map[string]any{
		"path": filepath.Join(t.TempDir(), "nope.png"),
	})
	if !res.IsError || !strings.Contains(out, "read ") {
		t.Fatalf("unreadable path = (IsError=%v, %q), want a read error", res.IsError, out)
	}
}
```

Add `"os"` and `"path/filepath"` to the test file's import block (it already imports `encoding/base64`, `strings`, `testing`, etc.).

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./cmd/flow-mcp/ -run 'TestLoopback_UploadArtifact_(Path|PathAndBase64|NeitherPathNorBase64)' -v`
Expected: FAIL — `path` is not yet a field, so path uploads fall through to the base64 branch and error with "name and mime are required" / decode failures rather than the asserted outcomes.

- [ ] **Step 3: Add the `Path` field and make base64-pairing fields optional**

In `cmd/flow-mcp/tools_artifacts.go`, replace the `uploadArtifactIn` struct with:

```go
type uploadArtifactIn struct {
	Node   string `json:"node,omitempty" jsonschema:"project slug, name, or id to attach the artifact to; omit to use the current directory's project"`
	Free   bool   `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
	Path   string `json:"path,omitempty" jsonschema:"absolute or relative filesystem path the local MCP process reads directly (relative resolves against the MCP process's working directory); preferred for files on disk. Mutually exclusive with base64."`
	Name   string `json:"name,omitempty" jsonschema:"the artifact's file name; optional with path (defaults to the file's basename), required with base64"`
	Mime   string `json:"mime,omitempty" jsonschema:"the artifact's MIME type, e.g. image/png; optional with path (guessed from the extension), required with base64"`
	Base64 string `json:"base64,omitempty" jsonschema:"the file contents, base64-encoded; use for small generated content. Mutually exclusive with path."`
}
```

- [ ] **Step 4: Rewrite the handler's validation + data resolution**

In `cmd/flow-mcp/tools_artifacts.go`, replace the body of `uploadArtifact` from its first line down to (but not including) the `var out string` line with:

```go
func (h *handlers) uploadArtifact(ctx context.Context, req *mcp.CallToolRequest, in uploadArtifactIn) (*mcp.CallToolResult, any, error) {
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	hasPath := strings.TrimSpace(in.Path) != ""
	hasB64 := strings.TrimSpace(in.Base64) != ""
	switch {
	case hasPath && hasB64:
		return errorResult("provide either path or base64, not both"), nil, nil
	case !hasPath && !hasB64:
		return errorResult("provide either path or base64"), nil, nil
	}

	var data []byte
	name, mimeType := in.Name, in.Mime
	if hasPath {
		b, readErr := os.ReadFile(in.Path)
		if readErr != nil {
			return errorResult(fmt.Sprintf("read %s: %v", in.Path, readErr)), nil, nil
		}
		data = b
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(in.Path)
		}
		mimeType = artifactfile.GuessMime(in.Path, in.Mime)
	} else {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(mimeType) == "" {
			return errorResult("name and mime are required"), nil, nil
		}
		b, decErr := base64.StdEncoding.DecodeString(in.Base64)
		if decErr != nil {
			return errorResult("base64: invalid encoding: " + decErr.Error()), nil, nil
		}
		data = b
	}
```

Then in the existing `h.do(...)` closure below, change the two upload calls to use the resolved `name`/`mimeType`/`data`:
- `c.UploadFreeArtifact(ctx, in.Name, in.Mime, data)` → `c.UploadFreeArtifact(ctx, name, mimeType, data)`
- `c.UploadArtifact(ctx, nodeID, in.Name, in.Mime, data)` → `c.UploadArtifact(ctx, nodeID, name, mimeType, data)`

(The `var out string`, the `h.do` call, the `if err != nil { return h.resultErr(err) … }`, and the final `return textResult(out)…` all stay as they are.)

- [ ] **Step 5: Fix the import block**

In `cmd/flow-mcp/tools_artifacts.go`, ensure the import block is:

```go
import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/artifactfile"
	"github.com/serverkraken/flow/internal/domain"
)
```

(Adds `os`, `path/filepath`, and the `artifactfile` package.)

- [ ] **Step 6: Update the tool description**

In `cmd/flow-mcp/server.go`, replace the `flow_upload_artifact` tool's `Description` with:

```go
		Description: "Upload an artifact (image or downloadable file) onto a node. Provide the file as `path` (a filesystem path the local MCP process reads directly — preferred for files on disk, no token overhead) or as `base64` (for small generated content); exactly one is required. With `path`, name and mime are optional (basename / guessed from the extension). Scoped to the current project by default; pass node to target another. Images render inline via ![[slug]] in Kompendium docs; other MIME types are download links.",
```

- [ ] **Step 7: Run the new tests to verify they pass**

Run: `go test ./cmd/flow-mcp/ -run 'TestLoopback_UploadArtifact' -v`
Expected: PASS — the new path tests plus the existing `_InvalidBase64` / `_MissingFields` base64 tests all green.

- [ ] **Step 8: Run the full flow-mcp package to verify no regressions**

Run: `go test ./cmd/flow-mcp/ -race`
Expected: PASS (base64 upload/list/delete, free, owner-scope-404, mutual-exclusion tests all still pass).

- [ ] **Step 9: Commit**

```bash
git add cmd/flow-mcp/tools_artifacts.go cmd/flow-mcp/server.go cmd/flow-mcp/tools_artifacts_test.go
git commit -m "feat(flow-mcp): flow_upload_artifact accepts a path parameter

path (mutually exclusive with base64) lets agents upload files by reference
instead of piping base64 through model output. name/mime optional in path-mode.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01VcCHz2MiiWUAAsiSwehgsD"
```

---

## Final Verification

- [ ] **Step 1: `make ci` green**

Run: `make ci` (with `DOCKER_HOST` pointed at Podman so pgstore testcontainers run).
Expected: golangci-lint 0 issues, verify-generate clean, coverage at/above gate, build succeeds.

- [ ] **Step 2: Build the binary**

Run: `go build -o bin/flow-mcp ./cmd/flow-mcp`
Expected: builds, no output.

- [ ] **Step 3: Schema smoke (automated equivalent of main-wiring check)**

No new tool/route is added, so the wiring check is that the schema change surfaces:
the `TestLoopback_UploadArtifact_Path_*` tests exercise a real `path` argument end to
end through the MCP `CommandTransport` — if `path` were absent from the tool's
inputSchema, the SDK would drop the arg and the path uploads would fail. Their passing
in Task 2 IS the smoke. Confirm they ran: `go test ./cmd/flow-mcp/ -run 'TestLoopback_UploadArtifact_Path' -v`.

- [ ] **Step 4: Live PROD gate (manual — needs Soenne)**

Against `.mcp.json` → `flow.thebackend.org`: rebuild `bin/flow-mcp` at the `.mcp.json`
path, `flow login` if the token is stale, `/mcp` reconnect to load the new binary, then:
1. `flow_upload_artifact` with `path` to a real file on a bound node → artifact lands; name = basename, mime guessed correctly.
2. Same with `free: true` → lands in the free library.
3. `path` + `base64` together → error; neither → error.
4. Clean up the test artifacts (`flow_delete_artifact`).

- [ ] **Step 5: Finish the branch**

Merge `fr-artifact-path` → `rebuild` (per superpowers:finishing-a-development-branch), then mark the FR `feature-requests/upload-artifact-path-param` addressed in flow.
