package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// ErrDoctorIssues is returned by `kompendium doctor --exit-non-zero-on-issues`
// when the report is not clean. CLI shells / CI pipelines test on this so a
// dirty notebook surfaces as a non-zero exit code.
var ErrDoctorIssues = errors.New("doctor reported issues")

func newDoctorCmd(deps Deps) *cobra.Command {
	var (
		asJSON      bool
		exitNonZero bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check notebook health (git status, frontmatter, wikilinks, merge markers)",
		Long: "Walk every note in the notebook and report inconsistencies: invalid frontmatter, " +
			"broken wikilinks, drift between path and frontmatter id, unresolved merge markers " +
			"left over from `import --bundle`, plus the notebook's git status. " +
			"Use --exit-non-zero-on-issues to make CI fail the run on a dirty notebook.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := deps.Doctor.Execute(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				if err := printDoctorJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else if err := printDoctorText(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			if exitNonZero && !report.IsClean() {
				return ErrDoctorIssues
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of human-readable text")
	cmd.Flags().BoolVar(&exitNonZero, "exit-non-zero-on-issues", false,
		"return a non-zero exit code when the report is not clean (CI mode)")
	return cmd
}

func printDoctorText(w io.Writer, r usecase.DoctorReport) error {
	if _, err := fmt.Fprintf(w, "Notebook: %s\nNotes:    %d\n", r.NotebookRoot, r.NoteCount); err != nil {
		return err
	}
	gitLine := "Git:      not a repository (run `kompendium init`)"
	switch {
	case r.IsRepo && r.HasUncommitted:
		gitLine = "Git:      repo (uncommitted changes — run `kompendium snapshot`)"
	case r.IsRepo:
		gitLine = "Git:      repo (clean)"
	}
	if _, err := fmt.Fprintln(w, gitLine); err != nil {
		return err
	}

	if err := writeIssues(w, "Invalid frontmatter", r.InvalidFrontmatter); err != nil {
		return err
	}
	if err := writeIssues(w, "Broken wikilinks", r.BrokenLinks); err != nil {
		return err
	}
	if err := writeIssues(w, "Inconsistent IDs", r.InconsistentIDs); err != nil {
		return err
	}
	if err := writeIssues(w, "Merge markers", r.MergeMarkers); err != nil {
		return err
	}

	if r.IsClean() {
		_, err := fmt.Fprintln(w, "All checks passed.")
		return err
	}
	return nil
}

func writeIssues(w io.Writer, header string, issues []usecase.DoctorIssue) error {
	if len(issues) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s:\n", header); err != nil {
		return err
	}
	for _, i := range issues {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", i.NoteID, i.Detail); err != nil {
			return err
		}
	}
	return nil
}

// doctorReportDTO mirrors the wire shape of DoctorReport. The CLI owns
// this type so a refactor of the use-case struct (renaming a field,
// adding a new check category) doesn't silently break external
// consumers of `kompendium doctor --json`. Other JSON outputs in this
// package use the same DTO discipline — see output.go.
type doctorReportDTO struct {
	NotebookRoot       string             `json:"notebook_root"`
	IsRepo             bool               `json:"is_repo"`
	HasUncommitted     bool               `json:"has_uncommitted"`
	NoteCount          int                `json:"note_count"`
	InvalidFrontmatter []doctorIssueDTO   `json:"invalid_frontmatter"`
	BrokenLinks        []doctorIssueDTO   `json:"broken_links"`
	InconsistentIDs    []doctorIssueDTO   `json:"inconsistent_ids"`
	MergeMarkers       []doctorIssueDTO   `json:"merge_markers"`
}

type doctorIssueDTO struct {
	NoteID string `json:"note_id"`
	Detail string `json:"detail"`
}

func toDoctorReportDTO(r usecase.DoctorReport) doctorReportDTO {
	conv := func(in []usecase.DoctorIssue) []doctorIssueDTO {
		out := make([]doctorIssueDTO, len(in))
		for i, v := range in {
			out[i] = doctorIssueDTO{NoteID: v.NoteID.String(), Detail: v.Detail}
		}
		return out
	}
	return doctorReportDTO{
		NotebookRoot:       r.NotebookRoot,
		IsRepo:             r.IsRepo,
		HasUncommitted:     r.HasUncommitted,
		NoteCount:          r.NoteCount,
		InvalidFrontmatter: conv(r.InvalidFrontmatter),
		BrokenLinks:        conv(r.BrokenLinks),
		InconsistentIDs:    conv(r.InconsistentIDs),
		MergeMarkers:       conv(r.MergeMarkers),
	}
}

func printDoctorJSON(w io.Writer, r usecase.DoctorReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(toDoctorReportDTO(r))
}
