package components

import "sync"

// Package components cannot import webui (the existing import direction is
// webui → components, never reversed — AGENTS.md), yet base.templ/
// appshell.templ/dialog.templ/palette.templ render /static/ URLs and need
// the SAME content-fingerprint webui.AssetVersion() computes from the
// embedded static/ tree (Lesesaal L4 Task 7). SetAssetVersion is the one
// crossing point: httpserver (which already imports both packages) calls it
// once, in Routes(), with webui.AssetVersion() — after that every
// components-side AssetURL call returns the fingerprinted URL without ever
// reaching back into webui.
var (
	assetVersionMu  sync.RWMutex
	assetVersionVal string
)

// SetAssetVersion stores the process-wide asset fingerprint. Called once
// (httpserver.Server.Routes) with webui.AssetVersion() — never per-request,
// since the value is a build artifact, not request-scoped data. Tests that
// need a known value call this directly and should t.Cleanup back to "".
func SetAssetVersion(v string) {
	assetVersionMu.Lock()
	defer assetVersionMu.Unlock()
	assetVersionVal = v
}

// AssetVersion returns the fingerprint set by SetAssetVersion, or "" if it
// was never called (e.g. a component test rendering base.templ directly
// without going through httpserver.Routes()).
func AssetVersion() string {
	assetVersionMu.RLock()
	defer assetVersionMu.RUnlock()
	return assetVersionVal
}

// AssetURL builds a /static/ URL for path, cache-busted with "?v=<hash>"
// when a version is known. Falls back to the bare path (no query string) so
// tests that never call SetAssetVersion still get a valid, servable URL —
// just without the cache-busting this task exists to add.
func AssetURL(path string) string {
	v := AssetVersion()
	if v == "" {
		return "/static/" + path
	}
	return "/static/" + path + "?v=" + v
}
