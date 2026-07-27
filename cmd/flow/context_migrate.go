package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

func contextMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate the document corpus into the B3d type system",
	}
	cmd.AddCommand(migrateDocTypesCmd(), migrateMemoriesCmd())
	return cmd
}

func migrateDocTypesCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "doctypes",
		Short: "Rewrite legacy `agent` docs to spec/plan with slim paths",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := c.RedesignDocTypes(cmd.Context(), dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scanned %d agent docs, converted %d%s\n",
				rep.Scanned, rep.Converted, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without mutating")
	return cmd
}

type manifestRow struct {
	File     string
	Scope    string // node slug or "global"
	Tags     []string
	Pin      bool
	Keep     bool
	Archived bool
}

type memoryDoc struct {
	Path     string
	Title    string
	Body     string
	Tags     []string
	Pinned   bool
	Archived bool
}

// parseManifest reads a TSV manifest: file<TAB>scope<TAB>tags<TAB>pin<TAB>keep.
// Blank lines, `#` comments, and a leading `file<TAB>...` header are ignored.
func parseManifest(r io.Reader) ([]manifestRow, error) {
	var rows []manifestRow
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			return nil, fmt.Errorf("manifest line needs 5 tab-separated columns: %q", line)
		}
		if f[0] == "file" { // header
			continue
		}
		var tags []string
		for _, t := range strings.Split(f[2], ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		row := manifestRow{
			File:  strings.TrimSpace(f[0]),
			Scope: strings.TrimSpace(f[1]),
			Tags:  tags,
			Pin:   strings.TrimSpace(f[3]) == "y",
			Keep:  strings.TrimSpace(f[4]) != "skip",
		}
		if len(f) >= 6 {
			row.Archived = strings.TrimSpace(f[5]) == "y"
		}
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

// deriveMemoryDoc turns a raw memory file body + its manifest row into the
// document to upsert: path = filename slug, title from frontmatter description
// (fallback name, fallback slug), body = content after the frontmatter, tags =
// manifest tags union frontmatter metadata.type.
func deriveMemoryDoc(body string, row manifestRow) memoryDoc {
	stem := strings.TrimSuffix(row.File, filepath.Ext(row.File))
	fm, start := domain.ParseFrontmatterMap(body)
	content := body
	if start > 0 {
		content = strings.TrimLeft(body[start:], "\n")
	}
	title := stem
	tags := append([]string{}, row.Tags...)
	if fm != nil {
		if d, ok := fm["description"].(string); ok && strings.TrimSpace(d) != "" {
			title = strings.TrimSpace(d)
		} else if n, ok := fm["name"].(string); ok && strings.TrimSpace(n) != "" {
			title = strings.TrimSpace(n)
		}
		if meta, ok := fm["metadata"].(map[string]any); ok {
			if mt, ok := meta["type"].(string); ok && mt != "" {
				tags = appendUnique(tags, mt)
			}
		}
	}
	pin := row.Pin && !row.Archived
	return memoryDoc{Path: stem, Title: title, Body: content, Tags: tags, Pinned: pin, Archived: row.Archived}
}

func appendUnique(ss []string, s string) []string {
	for _, x := range ss {
		if x == s {
			return ss
		}
	}
	return append(ss, s)
}

func migrateMemoriesCmd() *cobra.Command {
	var dir, manifest string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "memories",
		Short: "Import classified memory files into flow (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMigrateMemories(cmd.Context(), cmd.OutOrStdout(), dir, manifest, dryRun)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "memory directory (the source files)")
	cmd.Flags().StringVar(&manifest, "manifest", "", "reviewed TSV manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without writing")
	_ = cmd.MarkFlagRequired("dir")
	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}

func runMigrateMemories(ctx context.Context, out io.Writer, dir, manifest string, dryRun bool) error {
	mf, err := os.Open(manifest)
	if err != nil {
		return err
	}
	defer func() { _ = mf.Close() }()
	rows, err := parseManifest(mf)
	if err != nil {
		return err
	}

	c, err := clientFromStore(ctx)
	if err != nil {
		return err
	}
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return err
	}
	slugToID := map[string]string{}
	for _, n := range nodes {
		slugToID[n.Slug] = n.ID
	}

	var imported, skipped int
	for _, row := range rows {
		if !row.Keep || row.File == "MEMORY.md" {
			skipped++
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, row.File))
		if err != nil {
			return fmt.Errorf("read %s: %w", row.File, err)
		}
		doc := deriveMemoryDoc(string(raw), row)

		var nodeID *string
		if row.Scope != "global" {
			id, ok := slugToID[row.Scope]
			if !ok {
				return fmt.Errorf("%s: unknown scope slug %q (not a node)", row.File, row.Scope)
			}
			nodeID = &id
		}

		if dryRun {
			_, _ = fmt.Fprintf(out, "UPSERT %-45s -> %-30s tags=%v pin=%v archived=%v\n", doc.Path, row.Scope, doc.Tags, doc.Pinned, doc.Archived)
			imported++
			continue
		}
		if _, err := c.UpsertDocumentByPath(ctx, apiclient.UpsertByPathInput{
			Type: string(domain.DocMemory), NodeID: nodeID, Path: doc.Path,
			Title: doc.Title, Body: doc.Body, Tags: doc.Tags, Pinned: doc.Pinned,
			Archived: doc.Archived,
		}); err != nil {
			return fmt.Errorf("upsert %s: %w", doc.Path, err)
		}
		imported++
	}
	mode := ""
	if dryRun {
		mode = " (dry-run)"
	}
	_, _ = fmt.Fprintf(out, "\nimported %d, skipped %d%s\n", imported, skipped, mode)
	return nil
}
