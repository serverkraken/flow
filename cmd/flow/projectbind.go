package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
	"github.com/serverkraken/flow/internal/projectresolve"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzylist"
	"github.com/spf13/cobra"
)

// --- helpers (testable without a real git repo) ---

// runBind binds originSlug (the git remote of cwd) to the project identified by slug.
func runBind(ctx context.Context, c *apiclient.Client, originSlug, slug string) (string, error) {
	id, err := resolveSlug(ctx, c, slug)
	if err != nil {
		return "", err
	}
	if _, err := c.BindRemote(ctx, id, originSlug); err != nil {
		return "", err
	}
	return fmt.Sprintf("bound repo %s → %s", originSlug, slug), nil
}

// runBindRemote binds originSlug to a project identified by id and name
// (the name is used only for the confirmation message).
func runBindRemote(ctx context.Context, c *apiclient.Client, originSlug, nodeID, projectName string) (string, error) {
	if _, err := c.BindRemote(ctx, nodeID, originSlug); err != nil {
		return "", err
	}
	return fmt.Sprintf("bound repo %s → %s", originSlug, projectName), nil
}

// bindSelection acts on the picker's resolved selection:
// if isCreate is true it calls CreateNode(picked.Label) then BindRemote with the new ID;
// otherwise it calls BindRemote with picked.ID directly.
// This is extracted so the post-selection logic can be unit-tested without launching the TUI.
func bindSelection(ctx context.Context, c *apiclient.Client, originSlug string, picked fuzzylist.Item, isCreate bool) (string, error) {
	var nodeID, projectName string
	if isCreate {
		p, err := c.CreateNode(ctx, apiclient.CreateNodeFields{Name: picked.Label, Kind: string(domain.KindRepo)})
		if err != nil {
			return "", fmt.Errorf("create project: %w", err)
		}
		nodeID, projectName = p.ID, p.Name
	} else {
		nodeID, projectName = picked.ID, picked.Label
	}
	return runBindRemote(ctx, c, originSlug, nodeID, projectName)
}

// runBindInteractive launches the fuzzylist picker, picks or creates a project,
// then binds originSlug to it. Returns the confirmation message.
func runBindInteractive(ctx context.Context, c *apiclient.Client, originSlug, defaultName string, pal theme.Palette) (string, error) {
	projects, err := c.ListNodes(ctx)
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	items := make([]fuzzylist.Item, 0, len(projects))
	for _, p := range projects {
		items = append(items, fuzzylist.Item{ID: p.ID, Label: p.Name})
	}

	prog := newPickProjectProgram(items, defaultName, pal)
	if _, err := tea.NewProgram(prog, tea.WithContext(ctx)).Run(); err != nil {
		return "", fmt.Errorf("picker: %w", err)
	}
	picked, isCreate, ok := prog.Selection()
	if !ok {
		// User cancelled (Esc/Ctrl-C) — not an error.
		return "", nil
	}

	return bindSelection(ctx, c, originSlug, picked, isCreate)
}

// runBindPathInteractive launches the fuzzylist picker, picks or creates a project,
// then binds the local cwd path on this machine to it.
func runBindPathInteractive(ctx context.Context, c *apiclient.Client, machine clientmachine.Machine, cwd, defaultName string, pal theme.Palette) (string, error) {
	projects, err := c.ListNodes(ctx)
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	items := make([]fuzzylist.Item, 0, len(projects))
	for _, p := range projects {
		items = append(items, fuzzylist.Item{ID: p.ID, Label: p.Name})
	}

	prog := newPickProjectProgram(items, defaultName, pal)
	if _, err := tea.NewProgram(prog, tea.WithContext(ctx)).Run(); err != nil {
		return "", fmt.Errorf("picker: %w", err)
	}
	picked, isCreate, ok := prog.Selection()
	if !ok {
		return "", nil
	}

	var nodeID, projectName string
	if isCreate {
		p, err := c.CreateNode(ctx, apiclient.CreateNodeFields{Name: picked.Label, Kind: string(domain.KindRepo)})
		if err != nil {
			return "", fmt.Errorf("create project: %w", err)
		}
		nodeID, projectName = p.ID, p.Name
	} else {
		nodeID, projectName = picked.ID, picked.Label
	}
	return runBindPath(ctx, c, machine, cwd, nodeID, projectName)
}

// runUnbind removes the binding for originSlug.
func runUnbind(ctx context.Context, c *apiclient.Client, originSlug string) (string, error) {
	if err := c.UnbindRemote(ctx, originSlug); err != nil {
		return "", err
	}
	return fmt.Sprintf("unbound repo %s", originSlug), nil
}

// runBindPath binds a local directory path on this machine to a project.
// cwd must already be cleaned (filepath.Clean) by the caller.
func runBindPath(ctx context.Context, c *apiclient.Client, machine clientmachine.Machine, cwd, nodeID, projectName string) (string, error) {
	if _, err := c.BindPath(ctx, nodeID, machine.ID, machine.Label, cwd); err != nil {
		return "", err
	}
	return fmt.Sprintf("bound path %s on %s → %s", cwd, machine.Label, projectName), nil
}

// runUnbindPath removes the path binding for this machine and cwd.
// cwd must already be cleaned (filepath.Clean) by the caller.
func runUnbindPath(ctx context.Context, c *apiclient.Client, machine clientmachine.Machine, cwd string) (string, error) {
	if err := c.UnbindPath(ctx, machine.ID, cwd); err != nil {
		return "", err
	}
	return fmt.Sprintf("unbound path %s on %s", cwd, machine.Label), nil
}

// runBindings lists all bindings; the binding whose NodeID matches the
// project resolved via the resolution chain (FLOW_PROJECT env override →
// git-remote) is marked with *.
// getenv and cwd are injected so the function is testable without a real env/git repo.
func runBindings(ctx context.Context, c *apiclient.Client, getenv func(string) string, cwd string) (string, error) {
	bs, err := c.ListBindings(ctx)
	if err != nil {
		return "", err
	}
	if len(bs) == 0 {
		return "no bindings", nil
	}

	// Best-effort resolution: no match is fine (no star shown).
	resolved, ok, _ := projectresolve.Resolve(ctx, c, getenv, cwd)

	out := ""
	for _, b := range bs {
		marker := "  "
		if ok && b.NodeID == resolved.ID {
			marker = "* "
		}
		switch b.Kind {
		case domain.BindingRemote:
			out += fmt.Sprintf("%sremote  %s  (project %s)\n", marker, b.RemoteSlug, b.NodeID)
		default:
			out += fmt.Sprintf("%s%s  %s\n", marker, b.Kind, b.NodeID)
		}
	}
	return out, nil
}

// --- cobra wrappers ---

func nodeBindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bind [<slug>]",
		Short: "bind the current directory to a project (git origin → remote binding; else → path binding)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			originSlug, ok, err := gitremote.OriginSlug(cwd)
			if err != nil {
				return err
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}

			var out string
			if ok {
				// Git repo with origin → remote binding (unchanged behaviour).
				if len(args) == 1 {
					out, err = runBind(cmd.Context(), c, originSlug, args[0])
				} else {
					// Interactive picker: slog/stderr must not corrupt the TUI.
					logf, lerr := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
						os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
					if lerr == nil {
						defer func() { _ = logf.Close() }()
						os.Stderr = logf
					}
					out, err = runBindInteractive(cmd.Context(), c, originSlug, repoName(originSlug), theme.Load())
				}
			} else {
				// Not a git repo (or no origin) → path binding on this machine.
				cwd = filepath.Clean(cwd)
				m, merr := clientmachine.Load()
				if merr != nil {
					return fmt.Errorf("load machine identity: %w", merr)
				}
				defaultName := filepath.Base(cwd)
				if len(args) == 1 {
					// Non-interactive: resolve project by slug then bind path.
					nodeID, rerr := resolveSlug(cmd.Context(), c, args[0])
					if rerr != nil {
						return rerr
					}
					out, err = runBindPath(cmd.Context(), c, m, cwd, nodeID, args[0])
				} else {
					// Interactive picker with path-bind action.
					logf, lerr := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
						os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
					if lerr == nil {
						defer func() { _ = logf.Close() }()
						os.Stderr = logf
					}
					out, err = runBindPathInteractive(cmd.Context(), c, m, cwd, defaultName, theme.Load())
				}
			}
			if err != nil {
				return err
			}
			if out != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			}
			return nil
		},
	}
}

func nodeUnbindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unbind",
		Short: "remove the binding for the current directory (remote or path binding)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			originSlug, ok, err := gitremote.OriginSlug(cwd)
			if err != nil {
				return err
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			var out string
			if ok {
				// Git repo with origin → remove remote binding (unchanged).
				out, err = runUnbind(cmd.Context(), c, originSlug)
			} else {
				// Not a git repo → remove path binding on this machine.
				m, merr := clientmachine.Load()
				if merr != nil {
					return fmt.Errorf("load machine identity: %w", merr)
				}
				out, err = runUnbindPath(cmd.Context(), c, m, filepath.Clean(cwd))
			}
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

func nodeBindingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bindings",
		Short: "list all project bindings (current repo marked with *)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runBindings(cmd.Context(), c, os.Getenv, cwd)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
