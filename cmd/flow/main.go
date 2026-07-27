// Command flow is the flow client (CLI + later TUI).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "flow", Short: "flow client", RunE: runUI}
	root.AddCommand(whoamiCmd())
	root.AddCommand(worktimeCmd())
	root.AddCommand(sessionCmd())
	root.AddCommand(loginCmd())
	root.AddCommand(logoutCmd())
	root.AddCommand(dayoffCmd())
	root.AddCommand(exportCmd())
	root.AddCommand(nodeCmd())
	root.AddCommand(artifactCmd())
	root.AddCommand(docsCmd())
	root.AddCommand(contextCmd())
	root.AddCommand(uiCmd())
	return root
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
