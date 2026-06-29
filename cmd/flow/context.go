package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

func contextCmd() *cobra.Command {
	var path, repo string
	var capN int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show the composed start-context for this repo (SessionStart hook source)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runContext(cmd.Context(), cmd.OutOrStdout(), path, repo, capN, asJSON)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "directory to resolve (default: cwd)")
	cmd.Flags().StringVar(&repo, "repo", "", "explicit node slug override")
	cmd.Flags().IntVar(&capN, "cap", 0, "token budget override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the raw compose JSON")
	cmd.AddCommand(installHooksCmd(), flushCheckCmd(), contextMigrateCmd(), contextArchiveCmd(), contextArchivedCmd())
	return cmd
}

func runContext(ctx context.Context, out interface{ Write([]byte) (int, error) }, path, repo string, capN int, asJSON bool) error {
	dir := path
	if dir == "" {
		dir, _ = os.Getwd()
	}
	dir = filepath.Clean(dir)
	remote, _, _ := gitremote.OriginSlug(dir)
	m, _ := clientmachine.Load()
	q := apiclient.ContextQuery{Remote: remote, Machine: m.ID, Path: dir, Node: repo, Cap: capN}

	c, err := clientFromStore(ctx)
	if err == nil {
		cc, cerr := c.ComposeContext(ctx, q)
		if cerr == nil {
			_ = writeContextCache(q, cc)
			return emit(out, cc, false, "", asJSON)
		}
		err = cerr
	}
	// Network/auth failure → offline cache (SessionStart must never hard-fail).
	if cc, stamp, ok := readContextCache(q); ok {
		_ = emit(out, cc, true, stamp, asJSON) // swallow stdout error: the hook must not break
		return nil
	}
	_, _ = fmt.Fprintf(out, "# flow context\n\n_(offline — no cached context for this repo; %v)_\n", err)
	return nil // exit 0: do not break the hook
}

func emit(out interface{ Write([]byte) (int, error) }, cc usecase.ComposedContext, offline bool, stamp string, asJSON bool) error {
	if asJSON {
		b, _ := json.MarshalIndent(cc, "", "  ")
		_, err := out.Write(append(b, '\n'))
		return err
	}
	_, err := out.Write([]byte(renderContext(cc, offline, stamp)))
	return err
}

// renderContext is pure: ComposedContext → the Markdown block the SessionStart hook injects.
func renderContext(cc usecase.ComposedContext, offline bool, stamp string) string {
	var b strings.Builder
	b.WriteString("# flow context\n")
	if cc.Resolution.Unresolved {
		b.WriteString("\n_(repo not bound — run `flow node bind` to attach this directory)_\n")
	}
	if len(cc.Instructions) > 0 {
		b.WriteString("\n## Instructions\n")
		for _, it := range cc.Instructions {
			fmt.Fprintf(&b, "\n### [%s]\n%s\n", it.ScopeLabel, it.Body)
		}
	}
	b.WriteString("\n## Active Context\n")
	if cc.ActiveContext != nil {
		fmt.Fprintf(&b, "%s\n", cc.ActiveContext.Body)
	} else {
		b.WriteString("_(none yet — flush with `flow_set_active_context`)_\n")
	}
	groups := []struct{ key, label string }{
		{"leaf", "Leaf"}, {"vorhaben", "Vorhaben"}, {"engagement", "Engagement"}, {"global", "Global"},
	}
	wrote := false
	for _, g := range groups {
		items := cc.Memories[g.key]
		if len(items) == 0 {
			continue
		}
		if !wrote {
			b.WriteString("\n## Memories\n")
			wrote = true
		}
		for _, it := range items {
			fmt.Fprintf(&b, "\n### [%s] %s\n%s\n", g.label, it.ScopeLabel, it.Body)
		}
	}
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "%d/%d tokens", cc.Budget.Used, cc.Budget.Cap)
	for _, d := range []struct {
		n     int
		label string
	}{
		{cc.Budget.Dropped.Leaf, "leaf"},
		{cc.Budget.Dropped.Vorhaben, "vorhaben"},
		{cc.Budget.Dropped.Engagement, "engagement"},
		{cc.Budget.Dropped.Global, "global"},
	} {
		if d.n > 0 {
			fmt.Fprintf(&b, " · +%d %s not shown", d.n, d.label)
		}
	}
	if cc.Budget.Dropped.Pinned > 0 {
		fmt.Fprintf(&b, " · !! %d pinned not shown — raise --cap or unpin", cc.Budget.Dropped.Pinned)
	}
	if offline {
		fmt.Fprintf(&b, " · offline — Stand %s", stamp)
	}
	b.WriteString("\n")
	return b.String()
}

// nowStamp returns the current UTC time formatted as RFC3339.
func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

type cachedContext struct {
	Stamp string                  `json:"stamp"`
	CC    usecase.ComposedContext `json:"cc"`
}

func contextCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".flow", "context-cache")
}

func cacheKey(q apiclient.ContextQuery) string {
	sum := sha256.Sum256([]byte(q.Remote + "|" + q.Node + "|" + q.Path))
	return fmt.Sprintf("%x", sum[:8])
}

func writeContextCache(q apiclient.ContextQuery, cc usecase.ComposedContext) error {
	dir := contextCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(cachedContext{Stamp: nowStamp(), CC: cc})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, cacheKey(q)+".json"), b, 0o644)
}

func readContextCache(q apiclient.ContextQuery) (usecase.ComposedContext, string, bool) {
	b, err := os.ReadFile(filepath.Join(contextCacheDir(), cacheKey(q)+".json"))
	if err != nil {
		return usecase.ComposedContext{}, "", false
	}
	var cached cachedContext
	if err := json.Unmarshal(b, &cached); err != nil {
		return usecase.ComposedContext{}, "", false
	}
	return cached.CC, cached.Stamp, true
}
