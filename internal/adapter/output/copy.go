package output

import "fmt"

// Copy puts content into the system clipboard. The production setup
// targets macOS — pbcopy is invoked unconditionally. A Linux fallback
// (xclip / wl-copy) is documented in CLAUDE-worktime-menu-plan.md as a
// follow-up; adding it here means a small dispatch over exec.LookPath
// once a Linux user shows up.
func (t *Targets) Copy(content string) error {
	if _, err := t.runIn("pbcopy", nil, content); err != nil {
		return fmt.Errorf("pbcopy: %w", err)
	}
	return nil
}
