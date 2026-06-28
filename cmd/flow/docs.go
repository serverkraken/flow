package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/adapter/opener"
	"github.com/serverkraken/flow/internal/tui"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/spf13/cobra"
)

func docsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Compendium documents (TUI)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			// slog/stderr must never corrupt the TUI: send logs to a file.
			logf, err := os.OpenFile(filepath.Join(os.TempDir(), "flow-tui.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err == nil {
				defer func() { _ = logf.Close() }()
				os.Stderr = logf
			}
			m := tui.NewDocs(client, editor.New(), opener.New(), theme.Load(), os.Getenv("USER"))
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context())).Run()
			return err
		},
	}
	cmd.AddCommand(docsImportCmd())
	cmd.AddCommand(docsStripFrontmatterCmd())
	return cmd
}

func docsStripFrontmatterCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "strip-frontmatter",
		Short: "Remove leading YAML frontmatter from document bodies (verlustfrei)",
		Long: `Removes the leading YAML frontmatter block (---…---) from every document body
and preserves the full parsed map into documents.extra.frontmatter so the
operation is reversible. Run with --dry-run first to see how many documents
would be affected without making any changes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := c.StripFrontmatter(cmd.Context(), dryRun)
			if err != nil {
				return err
			}
			mode := ""
			if dryRun {
				mode = " (dry-run)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scanned %d, stripped %d%s\n",
				rep.Scanned, rep.Stripped, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without mutating")
	return cmd
}
