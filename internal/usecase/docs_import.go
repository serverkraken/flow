package usecase

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
)

// DocsImport mirrors a local Markdown tree into the documents-API
// (Spec §11.3): recursive, only *.md, idempotent — re-run overwrites with
// If-Match discipline (Get → Version → Put).
type DocsImport struct {
	Docs   ports.DocumentStore
	UserID string
}

// DocsImportResult counts the outcome of a DocsImport.Run call.
type DocsImportResult struct {
	Created, Updated, Unchanged, Skipped int
}

// Run walks dir recursively, importing every *.md file into the DocumentStore.
// Hidden directories (dot-prefixed) are skipped. report is called once per
// processed path; pass nil to suppress per-file output.
func (u *DocsImport) Run(dir string, report func(path string)) (DocsImportResult, error) {
	var res DocsImportResult
	root, err := filepath.Abs(dir)
	if err != nil {
		return res, err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			res.Skipped++
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		docPath := filepath.ToSlash(rel)
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		cur, err := u.Docs.Get(u.UserID, docPath)
		switch {
		case errors.Is(err, ports.ErrDocumentNotFound):
			if _, err := u.Docs.Put(u.UserID, docPath, string(body), "", 0); err != nil {
				return fmt.Errorf("create %s: %w", docPath, err)
			}
			res.Created++
		case err != nil:
			return fmt.Errorf("get %s: %w", docPath, err)
		case cur.Body == string(body):
			res.Unchanged++
		default:
			if _, err := u.Docs.Put(u.UserID, docPath, string(body), "", cur.Version); err != nil {
				return fmt.Errorf("update %s: %w", docPath, err)
			}
			res.Updated++
		}
		if report != nil {
			report(docPath)
		}
		return nil
	})
	return res, err
}
