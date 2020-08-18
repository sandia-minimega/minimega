package cmd

import (
	"phenix/util"
	"phenix/web"

	"github.com/spf13/cobra"
)

func newUiCmd() *cobra.Command {
	desc := `Run the phenix UI server

	Starts the UI server on the IP:port provided (or 0.0.0.0:3000 if not
	provided).`
	cmd := &cobra.Command{
		Use:   "ui <ip:port>",
		Short: "Run the phenix UI",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint := ":3000"

			if len(args) > 0 {
				endpoint = args[0]
			}

			if err := web.Start(web.ServeOnEndpoint(endpoint)); err != nil {
				return util.HumanizeError(err, "Unable to serve UI").Humanized()
			}

			return nil
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newUiCmd())
}
