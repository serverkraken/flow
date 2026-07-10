package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/projectresolve"
	"github.com/spf13/cobra"
)

// resolveArtifactNode maps a --node flag to a node ID: an explicit ref resolves
// via resolveSlug (slug/name-path/id); an empty ref falls back to the
// cwd-resolved project (git-origin or FLOW_PROJECT binding), erroring if none
// is bound — an artifact always needs exactly one node.
func resolveArtifactNode(ctx context.Context, c *apiclient.Client, nodeFlag string) (string, error) {
	if strings.TrimSpace(nodeFlag) != "" {
		return resolveSlug(ctx, c, nodeFlag)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	n, ok, err := projectresolve.Resolve(ctx, c, os.Getenv, cwd)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no node bound to %s (pass --node or bind this directory)", cwd)
	}
	return n.ID, nil
}

// resolveArtifactMime returns override when set, else a best-effort guess from
// path's extension (stripping any "; charset=..." parameter), else the
// catch-all application/octet-stream. No content sniffing — the server
// validates the final MIME type against the allowed set.
func resolveArtifactMime(path, override string) string {
	if override != "" {
		return override
	}
	if m := mime.TypeByExtension(filepath.Ext(path)); m != "" {
		if i := strings.Index(m, ";"); i >= 0 {
			m = strings.TrimSpace(m[:i])
		}
		return m
	}
	return "application/octet-stream"
}

func runArtifactAdd(ctx context.Context, c *apiclient.Client, w io.Writer, path, nodeFlag, mimeFlag string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	nodeID, err := resolveArtifactNode(ctx, c, nodeFlag)
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	mimeType := resolveArtifactMime(path, mimeFlag)
	a, err := c.UploadArtifact(ctx, nodeID, name, mimeType, data)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "uploaded %s (%s, %d bytes) → %s\n", a.Name, a.Mime, a.SizeBytes, a.Slug)
	return nil
}

func runArtifactLs(ctx context.Context, c *apiclient.Client, w io.Writer, nodeFlag string) error {
	nodeID, err := resolveArtifactNode(ctx, c, nodeFlag)
	if err != nil {
		return err
	}
	as, err := c.ListArtifacts(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(as) == 0 {
		_, _ = fmt.Fprintln(w, "keine Artefakte")
		return nil
	}
	for _, a := range as {
		_, _ = fmt.Fprintf(w, "%-16s %-24s %-24s %d\n", a.Slug, a.Name, a.Mime, a.SizeBytes)
	}
	return nil
}

func runArtifactRm(ctx context.Context, c *apiclient.Client, w io.Writer, slug, nodeFlag string) error {
	nodeID, err := resolveArtifactNode(ctx, c, nodeFlag)
	if err != nil {
		return err
	}
	if err := c.DeleteArtifact(ctx, nodeID, slug); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "deleted %s\n", slug)
	return nil
}

// --- cobra wrappers ---

func artifactCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "manage node artifacts (images, downloadable files)"}
	cmd.AddCommand(artifactAddCmd())
	cmd.AddCommand(artifactLsCmd())
	cmd.AddCommand(artifactRmCmd())
	return cmd
}

func artifactAddCmd() *cobra.Command {
	var node, mimeType string
	cmd := &cobra.Command{
		Use:   "add <datei>",
		Short: "upload a file as an artifact onto a node (default: the cwd-resolved project)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runArtifactAdd(cmd.Context(), c, cmd.OutOrStdout(), args[0], node, mimeType)
		},
	}
	cmd.Flags().StringVar(&node, "node", "", "node slug, name-path, or id (default: cwd-resolved project)")
	cmd.Flags().StringVar(&mimeType, "mime", "", "override the MIME type (default: guessed from the file extension)")
	return cmd
}

func artifactLsCmd() *cobra.Command {
	var node string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "list a node's artifact library (own + inherited from ancestors)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runArtifactLs(cmd.Context(), c, cmd.OutOrStdout(), node)
		},
	}
	cmd.Flags().StringVar(&node, "node", "", "node slug, name-path, or id (default: cwd-resolved project)")
	return cmd
}

func artifactRmCmd() *cobra.Command {
	var node string
	cmd := &cobra.Command{
		Use:   "rm <slug>",
		Short: "delete an artifact by slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runArtifactRm(cmd.Context(), c, cmd.OutOrStdout(), args[0], node)
		},
	}
	cmd.Flags().StringVar(&node, "node", "", "node slug, name-path, or id (default: cwd-resolved project)")
	return cmd
}
