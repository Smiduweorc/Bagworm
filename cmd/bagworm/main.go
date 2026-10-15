// Command bagworm drops you into an interactive shell inside your
// project's OCI image - whichever container runtime happens to be
// installed. Bagworms don't build houses; they inhabit whatever they find.
package main

import (
	"fmt"
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
		Long:         "Bagworm finds your project's bagworm.yaml, picks whichever OCI runtime\nis installed, and drops you into a shell inside the image - your user,\nyour files, and whichever git/ssh/gh identity the config opts into.\nZero flags for the 90% case.\n\nA config is only run once you have approved it (bagworm trust), and\nagain after every change to it.",
		Version:      version,
		SilenceUsage: true,
	}
	root.AddCommand(enterCmd())
	root.AddCommand(trustCmd())
	root.AddCommand(untrustCmd())
	return root
}

func enterCmd() *cobra.Command {
	var dryRun, noIdentity bool
	cmd := &cobra.Command{
		Use:   "enter",
		Short: "Enter an interactive shell inside the project's image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return enter.Run(enter.Options{DryRun: dryRun, NoIdentity: noIdentity})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the runtime command instead of executing it (works on untrusted configs)")
	cmd.Flags().BoolVar(&noIdentity, "no-identity", false, "mount no git/ssh/gh credentials, whatever the config asks for")
	return cmd
}

func trustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trust",
		Short: "Approve the bagworm.yaml that enter would use, as it is now",
		Long:  "Approve the bagworm.yaml that `bagworm enter` would use from here.\nThe approval is for its current contents: any change to the file\nneeds approving again. Review it (or run `bagworm enter --dry-run`) first.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, err := enter.Trust(cwd)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "trusted %s\n", path)
			return nil
		},
	}
}

func untrustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "untrust",
		Short: "Withdraw approval for the bagworm.yaml that enter would use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, removed, err := enter.Untrust(cwd)
			if err != nil {
				return err
			}
			if removed {
				fmt.Fprintf(cmd.OutOrStdout(), "untrusted %s\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s was not trusted\n", path)
			}
			return nil
		},
	}
}
