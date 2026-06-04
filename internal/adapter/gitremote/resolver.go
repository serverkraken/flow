// Package gitremote wraps `git remote get-url origin` so the use-case
// CanonicalKey resolver can be exec-based without importing os/exec
// itself (depguard keeps that boundary clean).
package gitremote

import (
	"os/exec"
	"strings"
)

// Resolver implements usecase.RemoteResolver via the local git binary.
type Resolver struct{}

// New returns a Resolver.
func New() *Resolver { return &Resolver{} }

// RemoteURL runs `git -C pwd remote get-url origin` and returns the trimmed
// URL on success. Any error (no git in PATH, not a repo, no origin remote)
// surfaces as ok=false — the use-case layer treats that as "fall back to
// path:sha256 key", which is exactly the contract.
func (Resolver) RemoteURL(pwd string) (string, bool) {
	cmd := exec.Command("git", "-C", pwd, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(out))
	return s, s != ""
}
