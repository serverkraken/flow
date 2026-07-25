package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/gitremote"
)

// noOrigin / withOrigin are injected git-origin lookups so the tests never run
// git and never depend on the checkout they happen to live in.
func noOrigin(string) (string, bool, error)   { return "", false, nil }
func withOrigin(string) (string, bool, error) { return "github.com/serverkraken/flow", true, nil }

// testBindEnv builds an env whose cwd and home are real temp directories, so the
// existence check runs against the real filesystem without touching $HOME.
func testBindEnv(t *testing.T, origin func(string) (string, bool, error)) bindEnv {
	t.Helper()
	return bindEnv{
		Cwd:     t.TempDir(),
		Home:    t.TempDir(),
		Machine: clientmachine.Machine{ID: "m1", Label: "notebook-a"},
		Origin:  origin,
	}
}

// TestLiveBindEnv exercises the production env builder itself (not the
// test double). It doesn't just check err == nil: it verifies the specific
// contract the doc comment on liveBindEnv promises — an absolute cwd, and an
// Origin func that is actually wired to gitremote.OriginSlug rather than nil
// or some other stub. Home/Machine are documented as best-effort, so the
// only thing asserted about them is that their absence never turns into an
// error.
func TestLiveBindEnv(t *testing.T) {
	env, err := liveBindEnv()
	if err != nil {
		t.Fatalf("liveBindEnv: %v", err)
	}

	wantCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if env.Cwd != wantCwd {
		t.Fatalf("Cwd = %q, want the process cwd %q", env.Cwd, wantCwd)
	}
	if !filepath.IsAbs(env.Cwd) {
		t.Fatalf("Cwd = %q, want an absolute path (resolveBindTarget joins relative paths against it)", env.Cwd)
	}

	if env.Origin == nil {
		t.Fatal("Origin is nil, want gitremote.OriginSlug wired in")
	}
	// Cross-check env.Origin against the real gitremote.OriginSlug on two
	// different directories rather than hardcoding an expected slug: a
	// wiring mistake (nil check aside, e.g. swapping in noOrigin-style stub,
	// or always-true/always-false) would disagree with the real function on
	// at least one of these, whatever this checkout's actual remote is.
	for _, dir := range []string{env.Cwd, t.TempDir()} {
		wantSlug, wantOK, wantErr := gitremote.OriginSlug(dir)
		gotSlug, gotOK, gotErr := env.Origin(dir)
		if gotSlug != wantSlug || gotOK != wantOK || (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("Origin(%q) = (%q, %v, %v), want gitremote.OriginSlug's own result (%q, %v, %v)",
				dir, gotSlug, gotOK, gotErr, wantSlug, wantOK, wantErr)
		}
	}
	// This package's checkout must actually have a resolvable origin, or the
	// loop above can't distinguish a correctly-wired Origin from a stub that
	// always reports "no origin" — both would agree with gitremote.OriginSlug
	// on a directory with no origin, but only the real thing agrees here too.
	if _, ok, _ := gitremote.OriginSlug(env.Cwd); !ok {
		t.Fatalf("gitremote.OriginSlug(%q) reported no origin; this test needs a checkout with a git origin to be meaningful", env.Cwd)
	}

	// Home and Machine are best-effort (doc comment on liveBindEnv): even
	// when the process has no resolvable home or machine id, liveBindEnv
	// must still return a nil error rather than propagating one.
	_ = env.Home
	_ = env.Machine
}

func TestResolveBindTarget_PathAndRemoteAreMutuallyExclusive(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd, Remote: "github.com/a/b"}, env)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("path+remote err = %v, want a mutually-exclusive guard", err)
	}
}

func TestResolveBindTarget_MissingDirectoryIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	missing := filepath.Join(env.Cwd, "definitely-not-here")
	_, err := resolveBindTarget(bindTargetArgs{Path: missing}, env)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing dir err = %v, want a 'does not exist' guard", err)
	}
	var g errGuard
	if !errors.As(err, &g) {
		t.Fatalf("missing dir err type = %T, want errGuard so the model sees it verbatim", err)
	}
}

func TestResolveBindTarget_FileInsteadOfDirectoryIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	file := filepath.Join(env.Cwd, "README.md")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveBindTarget(bindTargetArgs{Path: file}, env)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file path err = %v, want a 'not a directory' guard", err)
	}
}

func TestResolveBindTarget_OriginPresentYieldsRemoteKind(t *testing.T) {
	env := testBindEnv(t, withOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "remote" || got.RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("got %+v, want remote kind with the origin slug", got)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the resolved directory (resolve needs all three tiers)", got.Path)
	}
}

func TestResolveBindTarget_NoOriginYieldsPathKindWithMachine(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "path" || got.MachineID != "m1" || got.MachineLabel != "notebook-a" {
		t.Fatalf("got %+v, want path kind carrying the machine identity", got)
	}
}

func TestResolveBindTarget_KindRemoteWithoutOriginIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd, Kind: "remote"}, env)
	if err == nil || !strings.Contains(err.Error(), "git origin") {
		t.Fatalf("kind=remote without origin err = %v, want the decideBindKind guard", err)
	}
}

func TestResolveBindTarget_RelativePathResolvesAgainstProcessCwd(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	sub := filepath.Join(env.Cwd, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveBindTarget(bindTargetArgs{Path: "nested"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(sub) {
		t.Fatalf("Path = %q, want %q (relative resolves against the MCP process cwd)", got.Path, sub)
	}
}

func TestResolveBindTarget_TildeExpandsAgainstHome(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	sub := filepath.Join(env.Home, "SourceCode")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveBindTarget(bindTargetArgs{Path: "~/SourceCode"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(sub) {
		t.Fatalf("Path = %q, want %q (a JSON tool argument never passes through a shell)", got.Path, sub)
	}
}

func TestResolveBindTarget_OmittedPathUsesProcessCwd(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the process cwd %q", got.Path, env.Cwd)
	}
}

func TestResolveBindTarget_RemoteNeedsNoLocalDirectory(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Remote: "git@github.com:serverkraken/flow.git"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "remote" || got.RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("got %+v, want a normalized remote binding without any local checkout", got)
	}
	if got.Path != "" {
		t.Fatalf("Path = %q, want empty for a remote-only target", got.Path)
	}
}

func TestResolveBindTarget_UnparseableRemoteIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Remote: "not a url at all"}, env)
	if err == nil {
		t.Fatal("unparseable remote: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "clone URL") {
		t.Fatalf("error = %v, want it to name the accepted forms", err)
	}
}

// TestResolveBindTarget_BlankArgumentsAreErrorsNotSilentCwdFallbacks: a
// present-but-whitespace argument is a mistake. Trimming it to "" and falling
// back to the process cwd would bind a directory the caller never named.
func TestResolveBindTarget_BlankArgumentsAreErrorsNotSilentCwdFallbacks(t *testing.T) {
	env := testBindEnv(t, noOrigin)

	if _, err := resolveBindTarget(bindTargetArgs{Remote: "   "}, env); err == nil ||
		!strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("blank remote err = %v, want a 'must not be blank' guard", err)
	}
	if _, err := resolveBindTarget(bindTargetArgs{Path: "  "}, env); err == nil ||
		!strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("blank path err = %v, want a 'must not be blank' guard", err)
	}
	// Genuinely omitted still means "the process cwd".
	got, err := resolveBindTarget(bindTargetArgs{}, env)
	if err != nil {
		t.Fatalf("omitted arguments must still resolve to the cwd: %v", err)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the cwd", got.Path)
	}
}

// TestResolveBindTarget_GitExecFailure separates "no origin here" (ok=false,
// err=nil) from "git could not run" (err!=nil, gitremote.go:31). Auto-detect
// degrades to a path binding like bindProject always has; an explicit
// kind="remote" gets the true reason instead of "needs a git origin".
func TestResolveBindTarget_GitExecFailure(t *testing.T) {
	broken := func(string) (string, bool, error) {
		return "", false, errors.New(`exec: "git": executable file not found in $PATH`)
	}
	env := testBindEnv(t, broken)

	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("auto-detect must degrade to a path binding when git cannot run, got %v", err)
	}
	if got.Kind != "path" {
		t.Fatalf("Kind = %q, want path", got.Kind)
	}

	_, err = resolveBindTarget(bindTargetArgs{Path: env.Cwd, Kind: "remote"}, env)
	if err == nil {
		t.Fatal(`kind="remote" with a broken git: want an error`)
	}
	if !strings.Contains(err.Error(), "cannot run git") {
		t.Fatalf("error = %v, want the real reason, not the misleading 'needs a git origin'", err)
	}
}

func TestResolveBindTarget_PathKindWithoutMachineIDIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	env.Machine = clientmachine.Machine{}
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err == nil || !strings.Contains(err.Error(), "machine id") {
		t.Fatalf("missing machine id err = %v, want the machine-id guard", err)
	}
}

func TestResolveBindTarget_KindPathWithRemoteArgIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Remote: "github.com/a/b", Kind: "path"}, env)
	if err == nil || !strings.Contains(err.Error(), "local directory") {
		t.Fatalf("kind=path with remote err = %v, want a 'needs a local directory' guard", err)
	}
}

func TestExpandHomePath(t *testing.T) {
	cases := []struct {
		name, in, home, want string
	}{
		{"no tilde", "/abs/path", "/home/x", "/abs/path"},
		{"bare tilde", "~", "/home/x", "/home/x"},
		{"tilde slash", "~/SourceCode/flow", "/home/x", "/home/x/SourceCode/flow"},
		{"no home available", "~/x", "", "~/x"},
		{"bare user form is not expanded", "~other/x", "/home/x", "~other/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := expandHomePath(c.in, c.home); got != c.want {
				t.Fatalf("expandHomePath(%q, %q) = %q, want %q", c.in, c.home, got, c.want)
			}
		})
	}
}

func TestBindTargetLabelAndBindingFields(t *testing.T) {
	remote := bindTarget{Kind: "remote", RemoteSlug: "github.com/a/b"}
	if got := bindTargetLabel(remote); got != "remote github.com/a/b" {
		t.Errorf("bindTargetLabel(remote) = %q", got)
	}
	rf := bindingFieldsFor(remote)
	if rf.Kind != "remote" || rf.RemoteSlug != "github.com/a/b" || rf.Path != "" {
		t.Errorf("bindingFieldsFor(remote) = %+v", rf)
	}

	path := bindTarget{Kind: "path", MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/flow"}
	if got := bindTargetLabel(path); got != "path /work/flow" {
		t.Errorf("bindTargetLabel(path) = %q", got)
	}
	pf := bindingFieldsFor(path)
	if pf.Kind != "path" || pf.MachineID != "m1" || pf.MachineLabel != "notebook-a" || pf.Path != "/work/flow" {
		t.Errorf("bindingFieldsFor(path) = %+v", pf)
	}
	if pf.RemoteSlug != "" {
		t.Errorf("bindingFieldsFor(path).RemoteSlug = %q, want empty", pf.RemoteSlug)
	}
}
