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
				logReq   = MustGetBool(cmd.Flags(), "log-requests")
				logFull  = MustGetBool(cmd.Flags(), "log-full")
			)

			if len(args) > 0 {
				endpoint = args[0]
			}

			opts := []web.ServerOption{
				web.ServeWithJWTKey(jwtKey),
				web.ServeOnEndpoint(endpoint),
			}

			if logReq {
				opts = append(opts, web.ServeWithLogs("requests"))
			}

			if logFull {
				opts = append(opts, web.ServeWithLogs("full"))
			}

			if err := web.Start(opts...); err != nil {
				return util.HumanizeError(err, "Unable to serve UI").Humanized()
			}

			return nil
		},
	}

	cmd.Flags().String("jwt-signing-key", "", "Secret key used to sign JWT for authentication")
	cmd.Flags().Bool("log-requests", false, "Log API requests")
	cmd.Flags().Bool("log-full", false, "Log API requests and responses")

	return cmd
}

func init() {
	rootCmd.AddCommand(newUiCmd())
}
