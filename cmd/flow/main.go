// Command flow is the flow client (CLI + later TUI).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "flow", Short: "flow client"}
	root.AddCommand(whoamiCmd())
	root.AddCommand(worktimeCmd())
	return root
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
