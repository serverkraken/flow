package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
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

// projectResolver find-or-creates flow projects for vault `project:` paths,
// caching results for the run. Existing projects are matched by Name or Slug
// (the vault path, or its slugified form); unknown paths are created with the
// full path as the project name (the only field apiclient.CreateNode sets,
// and a stable idempotency key across re-runs).
type projectResolver struct {
	client   *apiclient.Client
	cache    map[string]string // vault project path → flow project id
	existing map[string]string // Name/Slug → flow project id (lazy-loaded)
	dryRun   bool
	created  int
}

func newProjectResolver(c *apiclient.Client, dryRun bool) *projectResolver {
	return &projectResolver{client: c, cache: map[string]string{}, dryRun: dryRun}
}

func (pr *projectResolver) load(ctx context.Context) error {
	if pr.existing != nil {
		return nil
	}
	pr.existing = map[string]string{}
	list, err := pr.client.ListNodes(ctx)
	if err != nil {
		return err
	}
	for _, p := range list {
		if p.Name != "" {
			pr.existing[p.Name] = p.ID
		}
		if p.Slug != "" {
			pr.existing[p.Slug] = p.ID
		}
	}
	return nil
}

func (pr *projectResolver) resolve(ctx context.Context, projectPath string) (*string, error) {
	if projectPath == "" {
		return nil, nil
	}
	if id, ok := pr.cache[projectPath]; ok {
		return &id, nil
	}
	if err := pr.load(ctx); err != nil {
		return nil, err
	}
	if id, ok := pr.existing[projectPath]; ok {
		pr.cache[projectPath] = id
		return &id, nil
	}
	if id, ok := pr.existing[slugify(projectPath)]; ok {
		pr.cache[projectPath] = id
		return &id, nil
	}
	if pr.dryRun {
		pr.created++
		id := "(dry-run)"
		pr.cache[projectPath] = id
		return &id, nil
	}
	p, err := pr.client.CreateNode(ctx, projectPath)
	if err != nil {
		return nil, err
	}
	pr.created++
	pr.cache[projectPath] = p.ID
	pr.existing[p.Name] = p.ID
	return &p.ID, nil
}

type importStats struct {
	imported, skipped, updated, failed, projectsCreated int
	failures                                            []string // "path: reason"
}

// runImport walks dir for *.md, derives each note's flow identity, resolves its
// project, and imports it (skip-existing, or --update). Errors are isolated
// per file; the walk continues.
func runImport(ctx context.Context, c *apiclient.Client, dir string, dryRun, update bool) (importStats, error) {
	var st importStats

	// Pre-list existing paths once for idempotency (+ id for --update).
	docs, err := c.ListDocuments(ctx)
	if err != nil {
		return st, fmt.Errorf("list documents: %w", err)
	}
	existingID := make(map[string]string, len(docs))
	for _, d := range docs {
		existingID[d.Path] = d.ID
	}

	pr := newProjectResolver(c, dryRun)

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			st.failed++
			st.failures = append(st.failures, rel+": read: "+rerr.Error())
			return nil
		}
		body := string(raw)
		fm := parseVaultFrontmatter(body)

		rawID := fm.ID
		if rawID == "" {
			rawID = strings.TrimSuffix(rel, ".md")
		}
		path := slugify(rawID)
		if path == "" {
			st.failed++
			st.failures = append(st.failures, rel+": empty path after slugify")
			return nil
		}
		typ := fm.Type
		if typ == "" {
			typ = "free"
		}
		title := titleFromBody(body)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(rel), ".md")
		}
		var date *time.Time
		if typ == "daily" {
			date = importDate(fm, rel)
			if date != nil {
				title = date.Format("2006-01-02")
			}
		}
		nodeID, perr := pr.resolve(ctx, fm.Project)
		if perr != nil {
			st.failed++
			st.failures = append(st.failures, rel+": project: "+perr.Error())
			return nil
		}

		if id, exists := existingID[path]; exists {
			if !update {
				st.skipped++
				return nil
			}
			if dryRun {
				st.updated++
				return nil
			}
			if _, uerr := c.UpdateDocument(ctx, id, apiclient.UpdateDocumentInput{Title: title, Body: body}); uerr != nil {
				st.failed++
				st.failures = append(st.failures, rel+": update: "+uerr.Error())
				return nil
			}
			st.updated++
			return nil
		}

		if dryRun {
			st.imported++
			existingID[path] = "(dry-run)"
			return nil
		}
		if _, ierr := c.ImportDocument(ctx, apiclient.ImportDocumentInput{
			Type: typ, Path: path, Title: title, Body: body, Date: date, NodeID: nodeID,
		}); ierr != nil {
			if apiclient.IsConflict(ierr) { // race backstop
				st.skipped++
				return nil
			}
			st.failed++
			st.failures = append(st.failures, rel+": import: "+ierr.Error())
			return nil
		}
		st.imported++
		existingID[path] = "(new)"
		return nil
	})
	st.projectsCreated = pr.created
	return st, walkErr
}

func docsImportCmd() *cobra.Command {
	var dryRun, update bool
	cmd := &cobra.Command{
		Use:   "import <dir>",
		Short: "Import a markdown vault into the compendium",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			st, err := runImport(cmd.Context(), c, args[0], dryRun, update)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"importiert %d · aktualisiert %d · übersprungen %d · Projekte angelegt %d · Fehler %d%s\n",
				st.imported, st.updated, st.skipped, st.projectsCreated, st.failed, mode)
			for _, f := range st.failures {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+f)
			}
			if st.failed > 0 {
				return fmt.Errorf("%d Datei(en) fehlgeschlagen", st.failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse und plane den Import, ohne zu schreiben")
	cmd.Flags().BoolVar(&update, "update", false, "vorhandene Dokumente (per Pfad) überschreiben statt überspringen")
	return cmd
}
