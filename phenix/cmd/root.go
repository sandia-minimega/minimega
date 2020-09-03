package cmd

import (
	goflag "flag"
	"fmt"
	"os"
	"strings"

	"phenix/api/config"
	"phenix/internal/common"
	"phenix/store"
	"phenix/util"

	log "github.com/activeshadow/libminimega/minilog"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	phenixBase       string
	minimegaBase     string
	hostnameSuffixes string
	storeEndpoint    string
	errFile          string
)

var rootCmd = &cobra.Command{
	Use:   "phenix",
	Short: "A cli application for phēnix",
	// Version: version.Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		common.PhenixBase = viper.GetString("base-dir.phenix")
		common.MinimegaBase = viper.GetString("base-dir.minimega")
		common.HostnameSuffixes = viper.GetString("hostname-suffixes")

		var (
			endpoint = viper.GetString("store.endpoint")
			errFile  = viper.GetString("log.error-file")
			errOut   = viper.GetBool("log.error-stderr")
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
		viper.WriteConfigAs("/tmp/phenix.yml")
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

	rootCmd.PersistentFlags().StringVar(&phenixBase, "base-dir.phenix", "/phenix", "base phenix directory")
	rootCmd.PersistentFlags().StringVar(&minimegaBase, "base-dir.minimega", "/tmp/minimega", "base minimega directory")
	rootCmd.PersistentFlags().StringVar(&hostnameSuffixes, "hostname-suffixes", "", "hostname suffixes to strip")
	// rootCmd.PersistentFlags().Int("log.verbosity", 0, "log verbosity (0 - 10)")
	rootCmd.PersistentFlags().Bool("log.error-stderr", false, "log fatal errors to STDERR")
	// rootCmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)

	if home, err := homedir.Dir(); err == nil {
		rootCmd.PersistentFlags().StringVar(&storeEndpoint, "store.endpoint", fmt.Sprintf("bolt://%s/.phenix.bdb", home), "endpoint for storage service")
		rootCmd.PersistentFlags().StringVar(&errFile, "log.error-file", fmt.Sprintf("%s/.phenix.err", home), "log fatal errors to file")
	} else {
		rootCmd.PersistentFlags().StringVar(&storeEndpoint, "store.endpoint", "/etc/phenix/.phenix.bdb", "endpoint for storage service")
		rootCmd.PersistentFlags().StringVar(&errFile, "log.error-file", "/etc/phenix/.phenix.err", "log fatal errors to file")
	}

	viper.BindPFlags(rootCmd.PersistentFlags())
}

func initConfig() {
	// TODO: stop setting minimega debug logging by default
	goflag.VisitAll(func(f *goflag.Flag) {
		switch f.Name {
		case "level":
			f.Value.Set("debug")
		case "verbose":
			f.Value.Set("true")
		}
	})

	log.Init()

	viper.SetConfigName("config")

	// Config paths - first look in current directory, then home directory (if
	// discoverable), then finally global config directory.
	viper.AddConfigPath(".")

	if home, err := homedir.Dir(); err == nil {
		viper.AddConfigPath(home + "/.config/phenix")
	}

	viper.AddConfigPath("/etc/phenix")

	viper.SetEnvPrefix("PHENIX")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
