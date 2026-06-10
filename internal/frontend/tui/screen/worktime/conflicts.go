package worktime

// conflicts.go — sync-conflict channel listener and resolution routing for the
// worktime root model.
//
// Design notes:
//
//   - listenForConflicts blocks on <-ch inside a goroutine that bubbletea
//     spawns for each Cmd invocation. Returning a conflictReceivedMsg re-arms
//     the listener so the next conflict is awaited. This is the idiomatic
//     bubbletea pattern for long-lived async channels.
//
//   - When Deps.Sync is nil (M2 mode) the session-conflict [s]/[l] keys emit a
//     toast "(Sync-Aktion in Task 33 verdrahtet)" and close the overlay rather
//     than attempting an actual AcceptServerVersion / OverwriteServerVersion
//     call. The overlay is still fully exercisable in tests by wiring Sync: nil.
//
//   - Type-cast failures (ConflictMsg.Local / .Server carry wrong type) render
//     a generic "Sync-Konflikt — Details fehlen" overlay with only [esc], which
//     is better UX than silently dropping the conflict.

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/conflict_overlay"
	"github.com/serverkraken/flow/internal/ports"
)

// conflictReceivedMsg wraps a ports.ConflictMsg so the bubbletea update loop
// can handle it without touching the channel directly.
type conflictReceivedMsg ports.ConflictMsg

// conflictResolveServerMsg is emitted by the sessions-conflict overlay when
// the user presses [s] (accept server version).
type conflictResolveServerMsg struct{ queueSeq int64 }

// conflictResolveLocalMsg is emitted by the sessions-conflict overlay when
// the user presses [l] (overwrite with local version).
type conflictResolveLocalMsg struct{ queueSeq int64 }

// conflictTakeoverMsg is emitted by the active-race overlay when the user
// presses [t] (take over the session from the other device).
type conflictTakeoverMsg struct {
	userID               string
	projectID            string
	currentServerVersion int64
}

// conflictParallelMsg is emitted by the active-race overlay when the user
// presses [n] (leave server's session alone, start a parallel session).
type conflictParallelMsg struct{}

// handleSyncMsg dispatches sync messages (conflict and pull-done) in the
// worktime Update loop. Split off from Update to keep Update's cognitive
// complexity within the gocognit/gocyclo budgets.
func (m Model) handleSyncMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pullDoneMsg:
		return m.handlePullDoneMsg()

	case conflictReceivedMsg:
		// A push-409 arrived from the sync worker. Build the appropriate overlay
		// and re-arm the listener so the next conflict is also handled.
		raw := ports.ConflictMsg(msg)
		ov, ok := m.buildConflictOverlay(raw)
		if !ok {
			ov = m.buildGenericFallbackOverlay()
		}
		m.conflictOverlay = ov
		m.currentConflict = raw
		return m, listenForConflicts(m.deps.Conflicts)

	case conflictResolveServerMsg:
		// User pressed [s] on a sessions-conflict overlay.
		m.conflictOverlay = nil
		if m.deps.Sync != nil {
			if err := m.deps.Sync.AcceptServerVersion(msg.queueSeq); err != nil {
				slog.Warn("worktime: AcceptServerVersion failed", "seq", msg.queueSeq, "err", err)
			}
			return m, nil
		}
		return m, emitConflictStubToast()

	case conflictResolveLocalMsg:
		// User pressed [l] on a sessions-conflict overlay.
		m.conflictOverlay = nil
		if m.deps.Sync != nil {
			if err := m.deps.Sync.OverwriteServerVersion(msg.queueSeq); err != nil {
				slog.Warn("worktime: OverwriteServerVersion failed", "seq", msg.queueSeq, "err", err)
			}
			return m, nil
		}
		return m, emitConflictStubToast()

	case conflictTakeoverMsg:
		// User pressed [t] on an active-race overlay.
		m.conflictOverlay = nil
		if m.deps.ActiveSessions != nil {
			if err := m.deps.ActiveSessions.ForceTakeover(
				msg.userID, msg.projectID, msg.currentServerVersion,
			); err != nil {
				slog.Warn(
					"worktime: ForceTakeover failed",
					"userID", msg.userID,
					"projectID", msg.projectID,
					"err", err,
				)
			}
		}
		return m, nil

	case conflictParallelMsg:
		// User pressed [n] on an active-race overlay. Close the overlay and
		// leave the server's session running — no local action needed.
		m.conflictOverlay = nil
		return m, nil

	case conflict_overlay.CancelMsg:
		// Esc on the conflict overlay (or the generic-fallback [s]/[l] stubs).
		m.conflictOverlay = nil
		return m, nil
	}
	return m, nil
}

// listenForConflicts returns a tea.Cmd that blocks until one ConflictMsg
// arrives on ch, then emits it as a conflictReceivedMsg. Returns nil when
// ch is nil so no goroutine is leaked on an un-wired Deps.Conflicts.
func listenForConflicts(ch <-chan ports.ConflictMsg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return conflictReceivedMsg(msg)
	}
}

// pullDoneMsg is emitted by listenForPullDone when the httpsync.Worker
// signals that a pull cycle completed. The worktime root handles this via
// handlePullDoneMsg, which broadcasts ChangedMsg{} to all sub-tabs and
// re-arms the listener.
type pullDoneMsg struct{}

// handlePullDoneMsg broadcasts a global ChangedMsg{} (zero Date) to every
// sub-tab so each can reload when cross-device data lands, then re-arms the
// pull-done listener so subsequent pull cycles are also caught. Split out of
// Update to keep Update's cognitive complexity within the gocognit budget.
func (m Model) handlePullDoneMsg() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i, s := range m.subs {
		updated, cmd := s.Update(ChangedMsg{})
		m.subs[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	cmds = append(cmds, listenForPullDone(m.deps.PullDone))
	return m, tea.Batch(cmds...)
}

// listenForPullDone returns a tea.Cmd that blocks until one signal arrives
// on ch (Worker.PullDone()), then emits a pullDoneMsg. Returns nil when ch
// is nil so no goroutine is leaked when PullDone is not wired.
//
// The goroutine exits cleanly when the worker closes the channel on shutdown
// (the ok==false branch returns nil, which bubbletea discards). Until the
// channel is closed the goroutine is re-armed on each signal by the
// pullDoneMsg handler in model.Update.
func listenForPullDone(ch <-chan struct{}) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-ch
		if !ok {
			return nil
		}
		return pullDoneMsg{}
	}
}

// buildConflictOverlay constructs the appropriate conflict_overlay.Model for
// msg. Returns (overlay, true) on success. Returns (nil, false) when the
// payload cast fails — the caller should render the generic-fallback overlay.
func (m Model) buildConflictOverlay(msg ports.ConflictMsg) (*conflict_overlay.Model, bool) {
	switch msg.Resource {
	case "sessions":
		local, lok := msg.Local.(domain.Session)
		server, sok := msg.Server.(domain.Session)
		if !lok || !sok {
			slog.Warn(
				"worktime: sessions conflict has unexpected payload types",
				"local_type", fmt.Sprintf("%T", msg.Local),
				"server_type", fmt.Sprintf("%T", msg.Server),
				"row_id", msg.RowID,
			)
			return nil, false
		}
		seq := msg.QueueSeq
		ov := conflict_overlay.NewSessionEditConflict(
			local, server, m.pal,
			func(accept bool) tea.Msg {
				if accept {
					return conflictResolveServerMsg{queueSeq: seq}
				}
				return conflictResolveLocalMsg{queueSeq: seq}
			},
		)
		ov = ov.SetSize(m.width, m.height)
		return &ov, true

	case "active_sessions", "active_sessions_stop":
		server, ok := msg.Server.(domain.ActiveSession)
		if !ok {
			slog.Warn(
				"worktime: active_sessions conflict has unexpected Server type",
				"server_type", fmt.Sprintf("%T", msg.Server),
				"row_id", msg.RowID,
			)
			return nil, false
		}
		userID := server.UserID
		projectID := server.ProjectID
		serverVersion := server.Version
		ov := conflict_overlay.NewActiveRaceConflict(
			server, m.pal,
			func() tea.Msg {
				return conflictTakeoverMsg{
					userID:               userID,
					projectID:            projectID,
					currentServerVersion: serverVersion,
				}
			},
			func() tea.Msg { return conflictParallelMsg{} },
		)
		ov = ov.SetSize(m.width, m.height)
		return &ov, true
	}

	// Unknown resource type.
	slog.Warn("worktime: unknown conflict resource type", "resource", msg.Resource)
	return nil, false
}

// buildGenericFallbackOverlay returns a minimal conflict overlay used when
// the ConflictMsg payload cannot be decoded.
func (m Model) buildGenericFallbackOverlay() *conflict_overlay.Model {
	ov := conflict_overlay.NewGenericFallback(m.pal)
	ov = ov.SetSize(m.width, m.height)
	return &ov
}

// conflictSyncStubMsg is emitted when Deps.Sync is nil and the user picks a
// sync action ([s] / [l]) in the conflict overlay. The worktime root logs it
// and drops it — it has no visual effect beyond the overlay closing (which
// already happened in the conflictResolveServer/LocalMsg handler before this
// cmd fires). Task 33 replaces the nil guard with a real SyncController call.
type conflictSyncStubMsg struct{}

// emitConflictStubToast returns a tea.Cmd that emits conflictSyncStubMsg.
// Used as the stub action when Deps.Sync == nil.
func emitConflictStubToast() tea.Cmd {
	return func() tea.Msg {
		slog.Info("worktime: sync action stubbed (Task 33 verdrahtet)")
		return conflictSyncStubMsg{}
	}
}
