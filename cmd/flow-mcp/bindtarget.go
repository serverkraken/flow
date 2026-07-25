package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
)

// bindTargetArgs are the three addressing arguments shared by
// flow_bind_project, all four actions of flow_node_binding, and the optional
// bind_path of flow_create_node (Spec §4). One resolver for all of them is the
// whole point: a binding target must mean the same thing everywhere.
type bindTargetArgs struct {
	Path   string
	Remote string
	Kind   string
}

// bindEnv is the environment the resolver needs, injected so tests never run git
// and never touch the real home directory. Origin has gitremote.OriginSlug's
// signature; Home may be "" when the process has no resolvable home (then a ~
// path stays literal and fails loudly at the existence check).
type bindEnv struct {
	Cwd     string
	Home    string
	Machine clientmachine.Machine
	Origin  func(dir string) (slug string, ok bool, err error)
}

// bindTarget is a resolved binding address. Kind decides which apiclient call
// bind/unbind uses. RemoteSlug, MachineID and Path are ALL populated for a
// directory-derived target, because ResolveNode matches on all three tiers at
// once (internal/adapter/apiclient/projectbindings.go:39) — resolve would
// otherwise lose the path tier for a repo that has a git origin.
type bindTarget struct {
	Kind         string // "remote" | "path"
	RemoteSlug   string // normalized origin slug; "" when the directory has none
	MachineID    string
	MachineLabel string
	Path         string // absolute + cleaned; "" for a remote-only target
}

// liveBindEnv builds the production environment. A missing cwd is fatal (the
// same guard bindProject has had all along); a missing home or machine id is
// best-effort — the ~ expansion and the path-kind branch each fail loudly on
// their own when they actually need what is missing.
func liveBindEnv() (bindEnv, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return bindEnv{}, errGuard{fmt.Errorf("cannot determine the working directory: %v", err)}
	}
	home, _ := os.UserHomeDir()
	machine, _ := clientmachine.Load()
	return bindEnv{Cwd: cwd, Home: home, Machine: machine, Origin: gitremote.OriginSlug}, nil
}

// expandHomePath replaces a leading "~" (bare, or followed by "/") with home.
// A CLI invocation gets the tilde expanded by the shell before the program sees
// it; an MCP tool call hands over a raw JSON string with no shell involved, and
// a model writes "~/SourceCode/foo" (Spec §4). The "~user" form is deliberately
// NOT expanded — it needs a passwd lookup and no model writes it; it stays
// literal and the existence check rejects it loudly.
func expandHomePath(path, home string) string {
	if home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolveBindTarget turns path | remote | (neither) into a concrete binding
// target. A directory MUST exist, because gitremote.OriginSlug runs
// `git -C <dir>` and treats every non-zero exit as "no origin" without an error
// (internal/gitremote/gitremote.go:22-33) — a typo in the path would otherwise
// silently become a path binding that never resolves.
func resolveBindTarget(in bindTargetArgs, env bindEnv) (bindTarget, error) {
	path := strings.TrimSpace(in.Path)
	remote := strings.TrimSpace(in.Remote)
	// A present-but-blank argument is a mistake, not an omission. Trimming it to
	// "" and silently falling back to the cwd would bind the wrong directory.
	if path == "" && in.Path != "" {
		return bindTarget{}, errGuard{errors.New(`"path" must not be blank: pass a directory, or omit it to use the flow-mcp process's working directory`)}
	}
	if remote == "" && in.Remote != "" {
		return bindTarget{}, errGuard{errors.New(`"remote" must not be blank: pass a clone URL or a host/path slug, or omit it`)}
	}
	if path != "" && remote != "" {
		return bindTarget{}, errGuard{errors.New(`"path" and "remote" are mutually exclusive: pass a directory in "path", a git remote in "remote", or neither to use the flow-mcp process's working directory`)}
	}

	if remote != "" {
		slug, ok := domain.NormalizeRemoteSlug(remote)
		if !ok {
			// domain.NormalizeRemoteSlug only accepts a clone URL (scheme or
			// scp-form). A caller that already has a normalized "host/path"
			// slug in hand — e.g. read off an existing node and passed straight
			// back — has no clone URL to give; accept that bare form here too.
			slug, ok = bareRemoteSlug(remote)
		}
		if !ok {
			return bindTarget{}, errGuard{fmt.Errorf("cannot read a git remote slug from %q; pass a clone URL (git@host:owner/repo.git or https://host/owner/repo) or a host/path slug like github.com/serverkraken/flow", remote)}
		}
		kind, err := decideBindKind(in.Kind, true)
		if err != nil {
			return bindTarget{}, err
		}
		if kind == "path" {
			return bindTarget{}, errGuard{errors.New(`kind "path" needs a local directory: pass it in "path" instead of "remote"`)}
		}
		return bindTarget{Kind: "remote", RemoteSlug: slug}, nil
	}

	dir := env.Cwd
	if path != "" {
		dir = expandHomePath(path, env.Home)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(env.Cwd, dir)
		}
	}
	dir = filepath.Clean(dir)
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return bindTarget{}, errGuard{fmt.Errorf("path %s does not exist or is not readable: %v", dir, statErr)}
	}
	if !info.IsDir() {
		return bindTarget{}, errGuard{fmt.Errorf("path %s is not a directory; a binding addresses a directory, not a file", dir)}
	}

	originSlug, originOK, originErr := env.Origin(dir)
	// originErr means git itself could not be executed (gitremote.go:31) — NOT
	// "no origin here", which the helper reports as ok=false, err=nil. Auto-detect
	// keeps degrading to a path binding as bindProject always has, but an EXPLICIT
	// kind="remote" must not be answered with the misleading "needs a git origin
	// in this directory" when the truth is that git never ran.
	if originErr != nil && strings.TrimSpace(in.Kind) == "remote" {
		return bindTarget{}, errGuard{fmt.Errorf("cannot run git in %s to read its origin: %v", dir, originErr)}
	}
	kind, err := decideBindKind(in.Kind, originOK)
	if err != nil {
		return bindTarget{}, err
	}
	if kind == "path" && env.Machine.ID == "" {
		return bindTarget{}, errGuard{errors.New("cannot determine this device's machine id for a path binding")}
	}
	return bindTarget{
		Kind: kind, RemoteSlug: originSlug,
		MachineID: env.Machine.ID, MachineLabel: env.Machine.Label, Path: dir,
	}, nil
}

// bareRemoteSlug accepts a remote argument that is already in the normalized
// "host/path" form domain.NormalizeRemoteSlug produces, rather than a clone
// URL. It rejects anything containing whitespace or lacking a "/" so a
// genuinely bogus argument like "not a url at all" still fails loudly instead
// of being silently accepted as a one-segment slug.
func bareRemoteSlug(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, " \t\n") || !strings.Contains(s, "/") {
		return "", false
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if s == "" {
		return "", false
	}
	return strings.ToLower(s), true
}

// bindTargetLabel names a resolved target for a result message.
func bindTargetLabel(tgt bindTarget) string {
	if tgt.Kind == "remote" {
		return "remote " + tgt.RemoteSlug
	}
	return "path " + tgt.Path
}

// bindingFieldsFor maps a resolved target onto the wire shape CreateBoundNode
// wants (internal/adapter/apiclient/projectbindings.go:12).
func bindingFieldsFor(tgt bindTarget) apiclient.BindingFields {
	if tgt.Kind == "remote" {
		return apiclient.BindingFields{Kind: "remote", RemoteSlug: tgt.RemoteSlug}
	}
	return apiclient.BindingFields{
		Kind: "path", MachineID: tgt.MachineID, MachineLabel: tgt.MachineLabel, Path: tgt.Path,
	}
}
