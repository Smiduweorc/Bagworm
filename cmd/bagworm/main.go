// Command bagworm drops you into an interactive shell inside your
// project's OCI image - whichever container runtime happens to be
// installed. Bagworms don't build houses; they inhabit whatever they find.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Smiduweorc/bagworm/internal/enter"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "bagworm",
		Short:        "Inhabit your project's container image",
		Long:         "Bagworm finds your project's bagworm.yaml, picks whichever OCI runtime\nis installed, and drops you into a shell inside the image - your user,\nyour files, your git/ssh identity. Zero flags for the 90% case.",
		Version:      version,
		SilenceUsage: true,
	}
	root.AddCommand(enterCmd())
	return root
}

func enterCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "enter",
		Short: "Enter an interactive shell inside the project's image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return enter.Run(enter.Options{DryRun: dryRun})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the runtime command instead of executing it")
	return cmd
}
