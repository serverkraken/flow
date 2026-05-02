package legacysource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

var (
	dailyDateRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	remoteRE    = regexp.MustCompile(`(?m)^Remote:\s*(.+?)\s*$`)
)

// Source implements ports.LegacySource.
type Source struct{}

// New returns an empty Source.
func New() Source { return Source{} }

// ListDailyNotes returns every YYYY-MM-DD.md file directly under sourceDir.
// A non-existent sourceDir yields no entries and no error — kompendium
// import-legacy is a one-shot best-effort migration.
func (Source) ListDailyNotes(_ context.Context, sourceDir string) ([]ports.LegacyDaily, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %q: %w", sourceDir, err)
	}

	out := make([]ports.LegacyDaily, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := dailyDateRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		full := filepath.Join(sourceDir, e.Name())
		body, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", full, err)
		}
		out = append(out, ports.LegacyDaily{Path: full, Date: m[1], Body: body})
	}
	return out, nil
}

// ListProjectNotes returns every *.md file under sourceDir, extracting the
// `Remote: <url>` line from the legacy boilerplate header. Files whose
// header carries `(kein Remote)` (the old project-notes.sh placeholder)
// surface with URL == "" so the use case can skip them with a clear reason.
func (Source) ListProjectNotes(_ context.Context, sourceDir string) ([]ports.LegacyProject, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %q: %w", sourceDir, err)
	}

	out := make([]ports.LegacyProject, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		full := filepath.Join(sourceDir, e.Name())
		body, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", full, err)
		}
		out = append(out, ports.LegacyProject{
			Path: full,
			URL:  extractRemote(body),
			Body: body,
		})
	}
	return out, nil
}

func extractRemote(body []byte) string {
	m := remoteRE.FindSubmatch(body)
	if m == nil {
		return ""
	}
	url := strings.TrimSpace(string(m[1]))
	if url == "(kein Remote)" {
		return ""
	}
	return url
}

var _ ports.LegacySource = Source{}
