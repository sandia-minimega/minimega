package cmd

import (
	goflag "flag"
	"fmt"
	"os"
	"phenix/api/config"
	"phenix/store"
	"phenix/util"
	"phenix/version"

	log "github.com/activeshadow/libminimega/minilog"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile       string
	storeEndpoint string
	errFile       string
)

var rootCmd = &cobra.Command{
	Use:     "phenix",
	Short:   "A cli application for phēnix",
	Version: version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var (
			endpoint = MustGetString(cmd.Flags(), "store.endpoint")
			errFile  = MustGetString(cmd.Flags(), "log.error-file")
			errOut   = MustGetBool(cmd.Flags(), "log.error-stderr")
		)

		if err := store.Init(store.Endpoint(endpoint)); err != nil {
			return fmt.Errorf("initializing storage: %w", err)
		}

		if err := util.InitFatalLogWriter(errFile, errOut); err != nil {
			return fmt.Errorf("Unable to initialize fatal log writer: %w", err)
		}

		if err := config.Init(); err != nil {
			return fmt.Errorf("Unable to initialize default configs: %w", err)
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		util.CloseLogWriter()
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage: true, // don't print help when subcommands return an error
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config.file", "C", "", "config file (default: $HOME/.phenix.yml)")
	rootCmd.PersistentFlags().StringVarP(&storeEndpoint, "store.endpoint", "S", "", "endpoint for storage service (default: bolt://$HOME/.phenix.bdb)")
	// rootCmd.PersistentFlags().IntP("log.verbosity", "V", 0, "log verbosity (0 - 10)")
	rootCmd.PersistentFlags().StringVarP(&errFile, "log.error-file", "E", "", "log fatal errors to file (default: $HOME/.phenix.err)")
	rootCmd.PersistentFlags().BoolP("log.error-stderr", "V", false, "log fatal errors to STDERR")
	// rootCmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
}

func initConfig() {
	goflag.VisitAll(func(f *goflag.Flag) {
		switch f.Name {
		case "level":
			f.Value.Set("debug")
		case "verbose":
			f.Value.Set("true")
		}
	})

	log.Init()

	var home string

	if cfgFile == "" || storeEndpoint == "" || errFile == "" {
		var err error

		// Find home directory.
		home, err = homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if cfgFile == "" {
		viper.AddConfigPath(home)
		viper.SetConfigName(".phenix")
	} else {
		viper.SetConfigFile(cfgFile)
	}

	if storeEndpoint == "" {
		storeEndpoint = "bolt://" + home + "/.phenix.bdb"
	}

	if errFile == "" {
		errFile = home + "/.phenix.err"
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
