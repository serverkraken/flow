package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	sessionStartCommand = `flow context --path "$CLAUDE_PROJECT_DIR"`
	stopCommand         = `flow context flush-check --path "$CLAUDE_PROJECT_DIR"`
)

func installHooksCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Install the SessionStart+Stop hooks into ~/.claude/settings.json (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _ := os.UserHomeDir()
			p := filepath.Join(home, ".claude", "settings.json")
			settings := map[string]any{}
			if b, err := os.ReadFile(p); err == nil {
				_ = json.Unmarshal(b, &settings)
			}
			merged, changed := mergeHooks(settings)
			b, _ := json.MarshalIndent(merged, "", "  ")
			if printOnly {
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}
			if !changed {
				fmt.Fprintln(cmd.OutOrStdout(), "hooks already installed")
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed SessionStart+Stop hooks into %s\n", p)
			fmt.Fprintln(cmd.OutOrStdout(), "tip: turn OFF native auto-memory and write memory only via flow (flow_create_doc/update_doc).")
			return nil
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the merged settings without writing")
	return cmd
}

// mergeHooks adds our two command entries to hooks.SessionStart and hooks.Stop,
// preserving any existing/foreign entries. Returns (merged, changed).
func mergeHooks(settings map[string]any) (map[string]any, bool) {
	if settings == nil {
		settings = map[string]any{}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	ensure := func(event, command string) {
		groups, _ := hooks[event].([]any)
		for _, g := range groups { // already present?
			gm, _ := g.(map[string]any)
			hs, _ := gm["hooks"].([]any)
			for _, h := range hs {
				if hm, _ := h.(map[string]any); hm != nil {
					if c, _ := hm["command"].(string); c == command {
						return
					}
				}
			}
		}
		groups = append(groups, map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command}},
		})
		hooks[event] = groups
		changed = true
	}
	ensure("SessionStart", sessionStartCommand)
	ensure("Stop", stopCommand)
	settings["hooks"] = hooks
	return settings, changed
}
