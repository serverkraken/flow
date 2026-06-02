package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"strings"
)

// RepoCanonicalKeyFromRemote normalises a `git remote get-url origin`
// output into the canonical `git:<host>/<owner>/<repo>` form.
//
// Accepts both SSH (`git@github.com:owner/repo.git`) and HTTPS forms; the
// `.git` suffix is stripped and the host is lowercased.
//
// Returns an empty string for inputs we can't parse — caller should fall
// back to RepoCanonicalKeyFromPath.
func RepoCanonicalKeyFromRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	if strings.HasPrefix(remote, "git@") {
		rest := strings.TrimPrefix(remote, "git@")
		host, path, ok := strings.Cut(rest, ":")
		if !ok {
			return ""
		}
		path = strings.TrimSuffix(path, ".git")
		return "git:" + strings.ToLower(host) + "/" + path
	}

	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
		return "git:" + strings.ToLower(u.Host) + "/" + path
	}

	return ""
}

// RepoCanonicalKeyFromPath returns `path:<sha256-hex>` of the absolute
// path. Used when a directory has no git remote — only the same absolute
// path on the same device matches.
func RepoCanonicalKeyFromPath(absPath string) string {
	clean := filepath.Clean(absPath)
	sum := sha256.Sum256([]byte(clean))
	return "path:" + hex.EncodeToString(sum[:])
}
