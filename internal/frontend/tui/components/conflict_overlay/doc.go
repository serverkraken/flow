// Package conflict_overlay is a bubbletea component that presents sync
// conflicts to the user and asks them to resolve the conflict before the
// background worker can continue.
//
// Two variants are supported:
//
//   - VariantSessionEdit — a sessions push returned HTTP 409 (optimistic
//     concurrency conflict): local and server versions of the same session
//     differ. The user picks "Server-Version übernehmen" (discard local),
//     "Lokal überschreiben" (force-push local over server), or Abbrechen.
//
//   - VariantActiveRace — an active_sessions start returned HTTP 409:
//     another device is already tracking time for the same project. The
//     user picks "Übernehmen" (force-takeover), "Neue parallele Session
//     starten" (start alongside), or Abbrechen.
//
// # Usage
//
// Construct via NewSessionEditConflict or NewActiveRaceConflict, then pass
// dimensions via SetSize on the host's WindowSizeMsg:
//
//	overlay := conflict_overlay.NewSessionEditConflict(local, server, p, onResolve)
//	overlay = overlay.SetSize(w, h)
//
// Route tea.Msg through Update. The component emits a tea.Cmd carrying the
// resolution message returned by the callback on a key match, or a CancelMsg
// on Esc. View() returns "" when the terminal is too small.
//
// Closing: like all overlays, conflict_overlay does not unmount itself.
// The host observes CancelMsg (and the resolution msg) in its own Update
// and clears its overlay-state field.
package conflict_overlay
