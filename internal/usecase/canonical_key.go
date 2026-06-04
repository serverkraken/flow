package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// RemoteResolver returns the git remote URL for the working directory.
// Production wires it to an os/exec-based adapter (`internal/adapter/gitremote`);
// tests inject a fake. The use-case layer doesn't import os/exec directly so
// `depguard` keeps the boundary clean.
type RemoteResolver interface {
	// RemoteURL returns (rawURL, true) when the directory has an origin
	// remote, ("", false) otherwise. Errors are silenced — "no remote" is
	// the only outcome that matters at this layer.
	RemoteURL(pwd string) (string, bool)
}

// CanonicalKey returns the stable identifier for the repository at pwd.
// Shape per the M2 spec:
//
//	git:<host>/<owner>/<repo>  — extracted from `git remote get-url origin`
//	                              and normalised (lowercase, no .git suffix,
//	                              git@/https://ssh:// stripped)
//	path:<sha256-hex>           — sha256 of the absolute path, when no remote
//
// Two devices that clone the same upstream produce the same git: key even
// when the local checkout path differs. Path-keyed repos are only addressable
// from the device they live on; that's documented in the spec.
func CanonicalKey(pwd string, resolver RemoteResolver) (string, error) {
	if resolver != nil {
		if url, ok := resolver.RemoteURL(pwd); ok && strings.TrimSpace(url) != "" {
			return normalizeGitURL(url), nil
		}
	}
	abs, err := filepath.Abs(pwd)
	if err != nil {
		return "", fmt.Errorf("canonical-key: abs %q: %w", pwd, err)
	}
	h := sha256.Sum256([]byte(abs))
	return "path:" + hex.EncodeToString(h[:]), nil
}

// normalizeGitURL collapses the common origin-URL shapes onto one canonical
// "git:<host>/<owner>/<repo>" form. Drops the .git suffix and the scheme.
func normalizeGitURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".git")
	if strings.HasPrefix(s, "git@") {
		// git@github.com:foo/bar → github.com/foo/bar
		rest := strings.TrimPrefix(s, "git@")
		s = strings.Replace(rest, ":", "/", 1)
	}
	s = strings.TrimPrefix(s, "ssh://git@")
	s = strings.TrimPrefix(s, "ssh://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return "git:" + strings.ToLower(s)
}
