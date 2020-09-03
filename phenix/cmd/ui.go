package cmd

import (
	"phenix/util"
	"phenix/web"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newUiCmd() *cobra.Command {
	desc := `Run the phenix UI server

  Starts the UI server on the IP:port provided.`
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Run the phenix UI",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []web.ServerOption{
				web.ServeOnEndpoint(viper.GetString("ui.listen-endpoint")),
				web.ServeWithJWTKey(viper.GetString("ui.jwt-signing-key")),
			}

			if MustGetBool(cmd.Flags(), "log-requests") {
				opts = append(opts, web.ServeWithMiddlewareLogging("requests"))
			}

			if MustGetBool(cmd.Flags(), "log-full") {
				opts = append(opts, web.ServeWithMiddlewareLogging("full"))
			}

			if err := web.Start(opts...); err != nil {
				return util.HumanizeError(err, "Unable to serve UI").Humanized()
			}

			return nil
		},
	}

	cmd.Flags().StringP("listen-endpoint", "e", "0.0.0.0:3000", "endpoint to listen on")
	cmd.Flags().StringP("jwt-signing-key", "k", "", "Secret key used to sign JWT for authentication")
	cmd.Flags().String("logs.phenix-path", "", "path to phenix log file to publish to UI")
	cmd.Flags().String("logs.minimega-path", "", "path to minimega log file to publish to UI")

	viper.BindPFlag("ui.listen-endpoint", cmd.Flags().Lookup("listen-endpoint"))
	viper.BindPFlag("ui.jwt-signing-key", cmd.Flags().Lookup("jwt-signing-key"))
	viper.BindPFlag("logs.phenix-path", cmd.Flags().Lookup("logs.phenix-path"))
	viper.BindPFlag("logs.minimega-path", cmd.Flags().Lookup("logs.minimega-path"))

	viper.BindEnv("ui.listen-endpoint")
	viper.BindEnv("ui.jwt-signing-key")
	viper.BindEnv("logs.phenix-path")
	viper.BindEnv("logs.minimega-path")

	cmd.Flags().Bool("log-requests", false, "Log API requests")
	cmd.Flags().Bool("log-full", false, "Log API requests and responses")

	cmd.Flags().MarkHidden("log-requests")
	cmd.Flags().MarkHidden("log-full")

	return cmd
}

func init() {
	rootCmd.AddCommand(newUiCmd())
}
