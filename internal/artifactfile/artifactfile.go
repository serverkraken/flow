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
