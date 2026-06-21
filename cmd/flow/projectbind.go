package main

import (
	"context"
	"fmt"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
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

// runUnbind removes the binding for originSlug.
func runUnbind(ctx context.Context, c *apiclient.Client, originSlug string) (string, error) {
	if err := c.UnbindRemote(ctx, originSlug); err != nil {
		return "", err
	}
	return fmt.Sprintf("unbound repo %s", originSlug), nil
}

// runBindings lists all bindings; the binding whose RemoteSlug matches originSlug
// (the resolved current repo) is marked with *.
// originSlug may be empty (not a git repo) — in that case no star is shown.
func runBindings(ctx context.Context, c *apiclient.Client, originSlug string) (string, error) {
	bs, err := c.ListBindings(ctx)
	if err != nil {
		return "", err
	}
	if len(bs) == 0 {
		return "no bindings", nil
	}
	out := ""
	for _, b := range bs {
		marker := "  "
		if b.Kind == domain.BindingRemote && originSlug != "" && b.RemoteSlug == originSlug {
			marker = "* "
		}
		switch b.Kind {
		case domain.BindingRemote:
			out += fmt.Sprintf("%sremote  %s  (project %s)\n", marker, b.RemoteSlug, b.ProjectID)
		default:
			out += fmt.Sprintf("%s%s  %s\n", marker, b.Kind, b.ProjectID)
		}
	}
	return out, nil
}

// --- cobra wrappers ---

func projectBindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bind <slug>",
		Short: "bind the current git repo's origin remote to a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			originSlug, ok, err := gitremote.OriginSlug(cwd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("not in a git repo with an 'origin' remote")
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runBind(cmd.Context(), c, originSlug, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

func projectUnbindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unbind",
		Short: "remove the binding for the current git repo's origin remote",
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
			if !ok {
				return fmt.Errorf("not in a git repo with an 'origin' remote")
			}
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runUnbind(cmd.Context(), c, originSlug)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

func projectBindingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bindings",
		Short: "list all project bindings (current repo marked with *)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Best-effort: derive current repo's origin for the * marker.
			// Not fatal if not in a git repo.
			cwd, _ := os.Getwd()
			originSlug, _, _ := gitremote.OriginSlug(cwd)

			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			out, err := runBindings(cmd.Context(), c, originSlug)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
