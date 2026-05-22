package browse

// Browse async commands + Msg-Types — loadEntriesCmd, loadBodiesCmd,
// deleteCmd kicken die jeweiligen Use-Cases auf der bubbletea-Cmd-Queue
// an; ihre Result-Msgs (entriesLoadedMsg, bodiesLoadedMsg,
// deleteFinishedMsg, editFinishedMsg) landen im Update-Reducer.
// runViaExecCapture umschließt tea.ExecProcess mit Stderr-Capture,
// damit Cobra-Errors aus dem Subprozess als Msg-Text durchkommen.
// Split aus model.go (Skill §No-Monoliths).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

type entriesLoadedMsg struct {
	entries []ports.NoteEntry
	err     error
}

func loadEntriesCmd(u *usecase.ListNotes, currentRepo domain.CanonicalURL) tea.Cmd {
	return func() tea.Msg {
		entries, err := u.Execute(context.Background(), usecase.ListNotesInput{
			CurrentRepo: currentRepo,
		})
		return entriesLoadedMsg{entries: entries, err: err}
	}
}

// editFinishedMsg lands when tea.ExecProcess returns from the editor.
type editFinishedMsg struct{ err error }

// bodiesLoadedMsg lands once the background goroutine has read every
// note's body so search can match against content, not only frontmatter.
type bodiesLoadedMsg struct{ bodies map[domain.ID][]byte }

// bodyExcerptLimit caps how much of each note body the loader keeps in
// memory. The map exists to back the row excerpt + the body-search
// substring match — both only need the start of the file. Holding full
// bodies for every note OOM-killed kompendium on real notebooks with
// large Markdown files; a few KB per entry is plenty for the use cases
// here. The preview pane reloads the full body on demand from the
// store, so opening a long note still renders end-to-end.
const bodyExcerptLimit = 8 * 1024

// loadBodiesBudget bounds the total time the background goroutine
// spends reading note bodies. bubbletea's tea.Cmd contract has no
// cancel signal — without a budget, a Program-Quit mid-load left the
// goroutine running until all reads finished (potentially seconds on
// notebooks with thousands of notes). 30s is generous enough for
// real notebooks on slow disks; if it expires the loader returns what
// it has and search continues against the partial map.
const loadBodiesBudget = 30 * time.Second

// perBodyBudget caps a single store.Get call so one stuck file
// (locked, NFS stall) can't burn the whole budget on its own.
const perBodyBudget = 2 * time.Second

func loadBodiesCmd(store ports.NoteStore, entries []ports.NoteEntry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadBodiesBudget)
		defer cancel()
		bodies := make(map[domain.ID][]byte, len(entries))
		for _, e := range entries {
			if ctx.Err() != nil {
				// Total budget exhausted — return partial map.
				break
			}
			noteCtx, noteCancel := context.WithTimeout(ctx, perBodyBudget)
			note, err := store.Get(noteCtx, e.ID)
			noteCancel()
			if err != nil {
				continue
			}
			body := note.Body
			if len(body) > bodyExcerptLimit {
				clipped := make([]byte, bodyExcerptLimit)
				copy(clipped, body[:bodyExcerptLimit])
				body = clipped
			}
			bodies[e.ID] = body
		}
		return bodiesLoadedMsg{bodies: bodies}
	}
}

// deleteFinishedMsg lands once the delete use case returns.
type deleteFinishedMsg struct{ err error }

func deleteCmd(u *usecase.DeleteNote, id domain.ID) tea.Cmd {
	return func() tea.Msg {
		return deleteFinishedMsg{err: u.Execute(context.Background(), id)}
	}
}

// runViaExecCapture wraps tea.ExecProcess with a stderr-capturing
// MultiWriter so that when the spawned process exits non-zero, the
// editFinishedMsg carries cobra's actual "Error: ..." line in
// addition to the bare exit code. Without this the alt-screen redraw
// after tea.ExecProcess wipes whatever the subprocess printed to
// stderr, leaving browse with only `*exec.ExitError`'s short
// "exit status N" — no actionable signal for the user.
//
// Stdout is left untouched (nvim and CreateX printCreateOutput need
// it to take over the TTY) and stderr keeps streaming to the user's
// terminal too — the captured copy is purely additive.
func runViaExecCapture(cmd *exec.Cmd) tea.Cmd {
	var errBuf bytes.Buffer
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			if captured := strings.TrimSpace(errBuf.String()); captured != "" {
				err = fmt.Errorf("%w — %s", err, captured)
			}
		}
		return editFinishedMsg{err: err}
	})
}
