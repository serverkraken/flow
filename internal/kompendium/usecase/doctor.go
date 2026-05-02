package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Doctor checks the notebook for inconsistencies that aren't caught at
// write-time: invalid frontmatter, broken wikilinks, drift between path and
// frontmatter ID, plus a lightweight git-status overview.
type Doctor struct {
	Store ports.NoteStore
	Git   ports.NotebookInitializer
}

// NewDoctor wires the use case with its required ports.
func NewDoctor(store ports.NoteStore, git ports.NotebookInitializer) *Doctor {
	return &Doctor{Store: store, Git: git}
}

// DoctorReport bundles every check result for a single Doctor.Execute call.
type DoctorReport struct {
	NotebookRoot       string
	IsRepo             bool
	HasUncommitted     bool
	NoteCount          int
	InvalidFrontmatter []DoctorIssue
	BrokenLinks        []DoctorIssue
	InconsistentIDs    []DoctorIssue
	MergeMarkers       []DoctorIssue
}

// DoctorIssue identifies one problem in one note.
type DoctorIssue struct {
	NoteID domain.ID
	Detail string
}

// IsClean reports whether every check passed (no issues, no uncommitted
// changes). The notebook may still be a non-repo; that is reported but not
// considered "dirty" by itself — kompendium init is a one-shot fix.
func (r DoctorReport) IsClean() bool {
	return !r.HasUncommitted &&
		len(r.InvalidFrontmatter) == 0 &&
		len(r.BrokenLinks) == 0 &&
		len(r.InconsistentIDs) == 0 &&
		len(r.MergeMarkers) == 0
}

// Execute walks every note, validates its frontmatter, checks ID
// consistency between path and frontmatter, and verifies that every
// wikilink resolves to an existing note in the notebook. Git status is
// reported but does not affect IsClean's repo check.
func (u *Doctor) Execute(ctx context.Context) (DoctorReport, error) {
	root := u.Store.Root()
	report := DoctorReport{NotebookRoot: root}

	if err := u.fillGitStatus(ctx, root, &report); err != nil {
		return DoctorReport{}, err
	}

	entries, err := u.Store.List(ctx, ports.ListFilter{})
	if err != nil {
		return DoctorReport{}, fmt.Errorf("list: %w", err)
	}
	report.NoteCount = len(entries)

	idSet := make(map[domain.ID]struct{}, len(entries))
	for _, e := range entries {
		idSet[e.ID] = struct{}{}
	}

	for _, e := range entries {
		if err := u.checkEntry(ctx, e, idSet, &report); err != nil {
			return DoctorReport{}, err
		}
	}
	return report, nil
}

func (u *Doctor) fillGitStatus(ctx context.Context, root string, report *DoctorReport) error {
	isRepo, err := u.Git.IsRepo(ctx, root)
	if err != nil {
		return fmt.Errorf("is-repo: %w", err)
	}
	report.IsRepo = isRepo
	if !isRepo {
		return nil
	}
	dirty, err := u.Git.HasUncommittedChanges(ctx, root)
	if err != nil {
		return fmt.Errorf("has-changes: %w", err)
	}
	report.HasUncommitted = dirty
	return nil
}

func (u *Doctor) checkEntry(
	ctx context.Context,
	e ports.NoteEntry,
	idSet map[domain.ID]struct{},
	report *DoctorReport,
) error {
	if err := e.Meta.Validate(); err != nil {
		report.InvalidFrontmatter = append(report.InvalidFrontmatter, DoctorIssue{
			NoteID: e.ID,
			Detail: err.Error(),
		})
	}
	if e.Meta.ID != "" && e.Meta.ID != e.ID.String() {
		report.InconsistentIDs = append(report.InconsistentIDs, DoctorIssue{
			NoteID: e.ID,
			Detail: fmt.Sprintf("frontmatter id %q != path id %q", e.Meta.ID, e.ID),
		})
	}
	note, err := u.Store.Get(ctx, e.ID)
	if err != nil {
		if errors.Is(err, ports.ErrNoteNotFound) {
			return nil
		}
		return fmt.Errorf("get %q: %w", e.ID, err)
	}
	for _, link := range note.Links() {
		if _, ok := idSet[domain.ID(link.Target)]; ok {
			continue
		}
		report.BrokenLinks = append(report.BrokenLinks, DoctorIssue{
			NoteID: e.ID,
			Detail: fmt.Sprintf("[[%s]] does not resolve", link.Target),
		})
	}
	if line, ok := scanMergeMarkers(note.Body); ok {
		report.MergeMarkers = append(report.MergeMarkers, DoctorIssue{
			NoteID: e.ID,
			Detail: fmt.Sprintf("unresolved merge marker on line %d (run git mergetool or hand-edit)", line),
		})
	}
	return nil
}

// scanMergeMarkers reports the first line in body that starts with one
// of git's three conflict markers. After `kompendium import --bundle`
// runs into a real conflict, the affected note keeps `<<<<<<<`,
// `=======`, `>>>>>>>` blocks until the user resolves them — until
// they do, the file is technically a valid Markdown but visually
// broken, so doctor surfaces them up front.
func scanMergeMarkers(body []byte) (int, bool) {
	markers := [][]byte{
		[]byte("<<<<<<<"),
		[]byte("=======\n"),
		[]byte(">>>>>>>"),
	}
	line := 1
	for offset := 0; offset < len(body); {
		for _, m := range markers {
			if bytes.HasPrefix(body[offset:], m) {
				return line, true
			}
		}
		nl := bytes.IndexByte(body[offset:], '\n')
		if nl < 0 {
			break
		}
		offset += nl + 1
		line++
	}
	return 0, false
}
