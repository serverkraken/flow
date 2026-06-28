package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/spf13/cobra"
)

const flushReminder = "Du hast in dieser Session gearbeitet, aber active-context (wo war ich / was offen / nächster Schritt) nicht aktualisiert — flush jetzt via flow_set_active_context, bevor du stoppst."

type stopHookInput struct {
	StopHookActive bool   `json:"stop_hook_active"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

type flushInput struct {
	StopHookActive   bool
	MutatingToolUses int
	ActiveStale      bool
}

// flushDecision: remind only when real work happened AND activeContext is stale,
// never while a stop-hook continuation is already in flight.
func flushDecision(in flushInput) bool {
	if in.StopHookActive {
		return false
	}
	return in.MutatingToolUses > 0 && in.ActiveStale
}

func flushCheckCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:    "flush-check",
		Short:  "Stop-hook: conditionally remind to flush active-context",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var hin stopHookInput
			_ = json.NewDecoder(cmd.InOrStdin()).Decode(&hin) // best-effort; missing fields default
			uses, sessionStart := scanTranscript(hin.TranscriptPath)
			stale := activeContextStale(cmd.Context(), path, hin.Cwd, sessionStart)
			if !flushDecision(flushInput{StopHookActive: hin.StopHookActive, MutatingToolUses: uses, ActiveStale: stale}) {
				return nil // silent, exit 0
			}
			out := map[string]any{"hookSpecificOutput": map[string]any{
				"hookEventName":     "Stop",
				"additionalContext": flushReminder,
			}}
			b, _ := json.Marshal(out)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "directory to resolve (default: cwd / hook cwd)")
	return cmd
}

// scanTranscript counts mutating tool_use entries and returns the first timestamp seen.
// Heuristic per spec §13 — refine at dogfood. Returns (0,"") on any read error.
func scanTranscript(p string) (int, string) {
	f, err := os.Open(p)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = f.Close() }()
	mutating := map[string]bool{"Edit": true, "Write": true, "Bash": true, "NotebookEdit": true}
	uses, first := 0, ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		var row struct {
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content []struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if first == "" && row.Timestamp != "" {
			first = row.Timestamp
		}
		for _, c := range row.Message.Content {
			if c.Type == "tool_use" && (mutating[c.Name] ||
				strings.HasPrefix(c.Name, "mcp__flow__flow_set") ||
				strings.HasPrefix(c.Name, "mcp__flow__flow_create") ||
				strings.HasPrefix(c.Name, "mcp__flow__flow_update")) {
				uses++
			}
		}
	}
	return uses, first
}

// activeContextStale asks the server for the current activeContext updatedAt and compares
// it to the session start. Any error → treat as NOT stale (stay silent, never nag wrongly).
func activeContextStale(ctx context.Context, path, hookCwd, sessionStart string) bool {
	dir := path
	if dir == "" {
		dir = hookCwd
	}
	remote, _, _ := gitremote.OriginSlug(dir)
	m, _ := clientmachine.Load()
	c, err := clientFromStore(ctx)
	if err != nil {
		return false
	}
	cc, err := c.ComposeContext(ctx, apiclient.ContextQuery{Remote: remote, Machine: m.ID, Path: dir})
	if err != nil || cc.ActiveContext == nil {
		return cc.ActiveContext == nil && err == nil // no activeContext yet + work done → stale
	}
	return sessionStart != "" && cc.ActiveContext.UpdatedAt < sessionStart
}
