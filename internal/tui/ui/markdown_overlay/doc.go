// Package markdown_overlay is the shared Markdown reader component
// used by the worktime brief overlay, the worktime today-note overlay,
// and the kompendium full-screen viewer. It hosts a viewport, a
// re-flowing markdown body, an optional search mode, optional code-
// snippet copy, and a uniform chrome (rounded frame + title + separator
// + footer + status bar).
//
// Render abstraction is passed in as a RenderFunc closure so the
// component doesn't bind to a specific renderer (some callers use
// ports.MarkdownRenderer, others bind markdown.Render with options).
//
// # Usage
//
// The construction sequence is: New → SetSize (typically on the host's
// first WindowSizeMsg) → optional SetSource / SetError. Every setter
// returns a fresh Model value — pointer-mutation is NOT the API. Hosts
// store the result back into their state:
//
//	overlay := markdown_overlay.New(renderFn,
//	    markdown_overlay.WithTitle("Brief 2026-05-12"),
//	    markdown_overlay.WithSource(body),
//	    markdown_overlay.WithSearch(),
//	)
//	overlay = overlay.SetSize(width, height)
//	// later, on WindowSizeMsg:
//	overlay = overlay.SetSize(msg.Width, msg.Height)
//	// on a reload:
//	overlay = overlay.SetSource(newBody)
//
// View() returns "" until SetSize has been called at least once with a
// non-trivial width/height — the host can render that empty string
// without special-casing.
//
// Closing: the overlay does not know what triggered its construction,
// so it cannot unmount itself. When the user hits a configured close
// key the overlay emits markdown_overlay.ExitMsg via tea.Cmd; the host
// observes the message in its own Update and clears its overlay-state
// field. Example:
//
//	case markdown_overlay.ExitMsg:
//	    m.overlay = nil
//	    return m, nil
package markdown_overlay
