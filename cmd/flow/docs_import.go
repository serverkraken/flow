package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"gopkg.in/yaml.v3"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify maps a vault path (the frontmatter id or relative path without .md)
// onto a flow-valid Path matching domain.SlugOK: per "/" segment lowercase,
// collapse every run of non-[a-z0-9] to a single "-", trim leading/trailing
// "-", and drop empty segments.
func slugify(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		seg = nonSlug.ReplaceAllString(strings.ToLower(seg), "-")
		seg = strings.Trim(seg, "-")
		if seg != "" {
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// vaultFrontmatter is the subset of a note's YAML frontmatter the importer reads.
type vaultFrontmatter struct {
	ID      string `yaml:"id"`
	Type    string `yaml:"type"`
	Date    string `yaml:"date"`
	Project string `yaml:"project"`
}

// parseVaultFrontmatter extracts the leading "---\n … \n---" YAML block.
// Returns the zero value when there is no frontmatter or it is malformed.
// The closing fence must be a line equal to "---" or "..." (line-exact, no suffix).
func parseVaultFrontmatter(body string) vaultFrontmatter {
	const open = "---\n"
	var fm vaultFrontmatter
	if !strings.HasPrefix(body, open) {
		return fm
	}
	rest := body[len(open):]

	end := -1
	for off := 0; off <= len(rest); {
		nl := strings.IndexByte(rest[off:], '\n')
		var line string
		if nl < 0 {
			line = rest[off:]
		} else {
			line = rest[off : off+nl]
		}
		if line == "---" || line == "..." {
			end = off
			break
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	if end < 0 {
		return fm
	}

	_ = yaml.Unmarshal([]byte(rest[:end]), &fm)
	return fm
}

// titleFromBody returns the first markdown H1 ("# …") after any frontmatter,
// or "" when there is none.
func titleFromBody(body string) string {
	_, start := domain.ParseFrontmatter(body)
	for _, ln := range strings.Split(body[start:], "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return ""
}

// importDate resolves a daily document's date: the frontmatter `date`
// (YYYY-MM-DD) first, else a YYYY-MM-DD filename, else nil.
func importDate(fm vaultFrontmatter, filename string) *time.Time {
	if fm.Date != "" {
		if t, err := time.Parse("2006-01-02", fm.Date); err == nil {
			return &t
		}
	}
	base := strings.TrimSuffix(filepath.Base(filename), ".md")
	if t, err := time.Parse("2006-01-02", base); err == nil {
		return &t
	}
	return nil
}
