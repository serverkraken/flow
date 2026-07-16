package artifactfile

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// GuessMime returns override when set, else a best-effort MIME type from the
// file extension. The server remains authoritative for allowed MIME types.
func GuessMime(path, override string) string {
	if override != "" {
		return override
	}
	if mimeType := mime.TypeByExtension(filepath.Ext(path)); mimeType != "" {
		if i := strings.Index(mimeType, ";"); i >= 0 {
			mimeType = strings.TrimSpace(mimeType[:i])
		}
		return mimeType
	}
	return "application/octet-stream"
}

// Read loads a regular file while enforcing the artifact size limit before
// and during the read. Rejecting non-regular files avoids blocking on devices
// and named pipes supplied through the MCP path argument.
func Read(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact path is not a regular file")
	}
	if info.Size() > domain.MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", domain.MaxArtifactBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, domain.MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > domain.MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", domain.MaxArtifactBytes)
	}
	return data, nil
}
