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
			var (
				endpoint = ":3000"
				jwtKey   = MustGetString(cmd.Flags(), "jwt-signing-key")
			)

			if len(args) > 0 {
				endpoint = args[0]
			}

			if err := web.Start(web.ServeOnEndpoint(endpoint), web.ServeWithJWTKey(jwtKey)); err != nil {
				return util.HumanizeError(err, "Unable to serve UI").Humanized()
			}

			return nil
		},
	}

	cmd.Flags().String("jwt-signing-key", "", "Secret key used to sign JWT for authentication")

	return cmd
}

func init() {
	rootCmd.AddCommand(newUiCmd())
}
