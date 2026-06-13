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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
	flowports "github.com/serverkraken/flow/internal/ports"
)

type entriesLoadedMsg struct {
	entries []ports.NoteEntry
	err     error
}

// changedMsg is emitted by listenForChanged when the httpapi.Status.Changed()
// channel signals that server-side data changed (SSE event or poll cycle).
// The Update handler reloads the corpus and re-arms the listener.
type changedMsg struct{}

// listenForChanged returns a tea.Cmd that blocks until one signal arrives on ch,
// then emits a changedMsg. Returns nil when ch is nil so no goroutine is
// leaked when Changed is not wired. The goroutine exits cleanly when the
// channel is closed (ok==false → bubbletea discards nil). The changedMsg
// handler must re-arm the listener so subsequent signals are also caught.
func listenForChanged(ch <-chan struct{}) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-ch
		if !ok {
			return nil
		}
		return changedMsg{}
	}
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

// editorReadyMsg lands when the tempfile has been written and the editor
// cmd is ready to launch. The Update reducer handles it by launching
// tea.ExecProcess and storing the tempfile context.
type editorReadyMsg struct {
	id        domain.ID
	tmpPath   string
	rawBefore []byte
	cmd       *exec.Cmd
}

// editorDoneMsg lands when prepareEditCmd fails before the editor could
// be launched (e.g. tempfile write error). The Update reducer uses id
// for display context. The tempfile path is not embedded here — the
// model tracks it via pendingEditTmp (set from editorReadyMsg.tmpPath).
type editorDoneMsg struct {
	id  domain.ID
	err error
}

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

// saveFinishedMsg lands after saveTempEditCmd completes (Put to Store).
type saveFinishedMsg struct{ err error }

// saveTempEditCmd reads the tempfile, parses the content, calls Store.Put
// if the content changed, then removes the tempfile. On version conflict
// the error message includes the tempfile path so the user can recover.
func saveTempEditCmd(store ports.NoteStore, id domain.ID, tmpPath string, rawBefore []byte) tea.Cmd {
	return func() tea.Msg {
		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath) // best-effort; no-op if already gone
			return saveFinishedMsg{err: fmt.Errorf("read tempfile %s: %w", tmpPath, err)}
		}
		if string(edited) == string(rawBefore) {
			_ = os.Remove(tmpPath)
			return saveFinishedMsg{}
		}
		fm, body, err := domain.ParseFrontmatter(edited)
		if err != nil {
			return saveFinishedMsg{err: fmt.Errorf("frontmatter kaputt — Bearbeitung liegt in %s: %w", tmpPath, err)}
		}
		note, err := domain.NewNote(id, fm, body)
		if err != nil {
			return saveFinishedMsg{err: fmt.Errorf("note invalid — Bearbeitung liegt in %s: %w", tmpPath, err)}
		}
		if err := store.Put(context.Background(), note); err != nil {
			if errors.Is(err, flowports.ErrDocumentVersionConflict) {
				// Tempfile bleibt erhalten — User kann daraus manuell zusammenführen.
				return saveFinishedMsg{err: fmt.Errorf("parallel geändert — Bearbeitung liegt in %s; neu laden und zusammenführen: %w", tmpPath, err)}
			}
			_ = os.Remove(tmpPath)
			return saveFinishedMsg{err: fmt.Errorf("speichern fehlgeschlagen: %w", err)}
		}
		_ = os.Remove(tmpPath)
		return saveFinishedMsg{}
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
