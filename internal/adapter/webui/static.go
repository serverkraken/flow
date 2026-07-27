package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
	"sync"
)

//go:embed all:static
var staticFS embed.FS

var (
	assetVersionOnce sync.Once
	assetVersionVal  string
)

// AssetVersion returns a short, deterministic content-fingerprint over every
// file embedded under static/ — computed once (sync.Once) per process, never
// a timestamp, so it is reproducible across replicas of the same build
// (Lesesaal L4 Task 7, OE #11: ONE global hash for all assets, no per-file
// manifest — a deploy busts every /static/ URL together; simpler than a
// manifest, and every deploy already invalidates all assets at once anyway).
func AssetVersion() string {
	assetVersionOnce.Do(func() {
		assetVersionVal = computeAssetVersion()
	})
	return assetVersionVal
}

// computeAssetVersion walks the embedded static/ tree in a stable (lexical,
// fs.WalkDir-guaranteed) order and hashes each file's path + content, so the
// result only depends on the embedded bytes — never on wall-clock time (the
// embed.FS reports a zero modtime for every entry, which is exactly why
// http.FileServerFS could not set Last-Modified/ETag on its own).
func computeAssetVersion() string {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	h := sha256.New()
	var paths []string
	_ = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths) // fs.WalkDir is already lexical, but be explicit/robust
	for _, p := range paths {
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			continue // compile-time-embedded path just listed; cannot fail at runtime
		}
		h.Write([]byte(p))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// AssetURL builds a cache-busted /static/ URL for path (e.g. "app.css",
// "js/dialog.js") — the single source of truth every templ component uses
// instead of a bare "/static/..." string literal.
func AssetURL(path string) string {
	return "/static/" + path + "?v=" + AssetVersion()
}

// StaticHandler serves the embedded static assets (mount under /static/).
// Every response carries a long-lived, immutable Cache-Control — safe ONLY
// because every URL a page renders is bust with AssetURL's "?v=<hash>": a
// deploy that changes any asset changes the hash, which changes the URL, so
// the old cached (and now unreachable) response never resurfaces (Lesesaal
// L4 Task 7 — before this, embed.FS's zero-modtime meant no ETag/Last-
// Modified at all, and Cloudflare's 4h edge cache served stale CSS/JS against
// fresh HTML after every deploy, verified on PROD 2026-07-06).
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	fileServer := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}
