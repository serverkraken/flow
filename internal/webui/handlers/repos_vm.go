package handlers

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/format"
	repostmpl "github.com/serverkraken/flow/internal/webui/templates/repos"
)

// buildReposIndexVM assembles the /repos list view-model. For every
// repo we look up its RepoNote (if any) so the row can carry the
// "note ✓" pill and the "geändert" relative-time. A missing note is
// the expected shape — handled via flowports.ErrRepoNoteNotFound. Any
// other lookup error degrades the single row's relative-time field to
// "—" rather than failing the whole render.
func buildReposIndexVM(d ReposDeps, userID string, repos []domain.Repo) repostmpl.IndexVM {
	vm := repostmpl.IndexVM{
		HasRepos: len(repos) > 0,
		Rows:     make([]repostmpl.IndexRow, 0, len(repos)),
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}

	notesCount := 0
	for _, repo := range repos {
		hasNote := false
		modified := "—"
		if d.RepoNotes != nil {
			note, err := d.RepoNotes.GetByRepo(userID, repo.ID)
			switch {
			case err == nil:
				hasNote = true
				notesCount++
				modified = format.HumanRelativeTime(note.UpdatedAt, now)
			case errors.Is(err, flowports.ErrRepoNoteNotFound):
				// no-op — leave hasNote=false / modified="—".
			default:
				// Defensive: degrade silently. The list row still renders.
				_ = err
			}
		}
		vm.Rows = append(vm.Rows, repostmpl.IndexRow{
			DisplayName: repostmpl.DisplayNameOr(repo.DisplayName, repo.CanonicalKey),
			Subtitle:    subtitleFor(repo, hasNote),
			HasNote:     hasNote,
			MetaLeft:    repoMetaLeft(repo),
			MetaRight:   modified,
			Href:        repostmpl.NoteHref(repo.CanonicalKey),
		})
	}

	vm.TotalLabel = repostmpl.FormatRepoTotal(len(repos), notesCount)
	return vm
}

// subtitleFor returns the second line of a repo row. The canonical key
// already encodes the remote (`git:host/o/r` / `path:<sha>`); we
// re-stylise the `git:` form back to the SSH-style `git@host:o/r`
// rendering the mockup uses so the row reads like a remote URL. Path
// keys are rendered as "(lokal)".
func subtitleFor(repo domain.Repo, hasNote bool) string {
	base := remoteDisplay(repo.CanonicalKey)
	if hasNote {
		return base + " · note ✓"
	}
	return base
}

// remoteDisplay turns a CanonicalKey into the visible remote form. We
// keep this here (not in the templ helper) because it touches domain
// shape; the templ-side helper is pure formatting.
func remoteDisplay(canonicalKey string) string {
	if strings.HasPrefix(canonicalKey, "git:") {
		rest := strings.TrimPrefix(canonicalKey, "git:")
		host, path, ok := strings.Cut(rest, "/")
		if !ok {
			return canonicalKey
		}
		return "git@" + host + ":" + path
	}
	if strings.HasPrefix(canonicalKey, "path:") {
		return "(lokal)"
	}
	return canonicalKey
}

// repoMetaLeft returns the bottom-left meta cell on the index row.
// We use the version readout so the list parities the TUI's repo
// screen, which also highlights the sync-version.
func repoMetaLeft(repo domain.Repo) string {
	if repo.Version <= 0 {
		return "noch nicht synced"
	}
	return "version " + strconv.FormatInt(repo.Version, 10)
}

// buildReposNoteVM resolves a Repo + (optional) RepoNote into the
// view-model. When the note isn't present the placeholder branch is
// chosen by hasNote=false; HTML stays empty.
func buildReposNoteVM(d ReposDeps, repo domain.Repo, note domain.RepoNote, hasNote bool) repostmpl.NoteVM {
	vm := repostmpl.NoteVM{
		DisplayName:   repostmpl.DisplayNameOr(repo.DisplayName, repo.CanonicalKey),
		CanonicalKey:  repo.CanonicalKey,
		RemoteURL:     remoteOrFallback(repo.CanonicalKey),
		ShortHash:     repostmpl.ShortHash(repo.ID),
		DevicesLabel:  devicesPlaceholder,
		ModifiedLabel: "—",
		HasNote:       hasNote,
	}
	if hasNote {
		now := time.Now()
		if d.Clock != nil {
			now = d.Clock.Now()
		}
		if rel := format.HumanRelativeTime(note.UpdatedAt, now); rel != "" {
			vm.ModifiedLabel = rel
		}
		if d.Markdown != nil && note.Content != "" {
			html, err := d.Markdown.Render([]byte(note.Content))
			if err != nil {
				// Degrade to empty body — the meta strip still renders.
				html = ""
			}
			vm.HTML = html
		}
	}
	return vm
}

// remoteOrFallback returns the monospace subtitle under the heading.
// Path-keyed repos render "(kein remote)" so the field never reads as
// an empty mono line.
func remoteOrFallback(canonicalKey string) string {
	if strings.HasPrefix(canonicalKey, "path:") {
		return "(kein remote)"
	}
	return remoteDisplay(canonicalKey)
}
