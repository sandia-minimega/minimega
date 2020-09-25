package cmd

import (
	"fmt"

	"phenix/version"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version.Version)
			return nil
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newVersionCmd())
}
