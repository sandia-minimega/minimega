package cmd

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"phenix/api/config"
	"phenix/internal/common"
	"phenix/store"
	"phenix/util"

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

	uid, home := getCurrentUserInfo()

	if uid == "0" {
		os.MkdirAll("/etc/phenix", 0755)
		os.MkdirAll("/var/log/phenix", 0755)

		rootCmd.PersistentFlags().StringVar(&storeEndpoint, "store.endpoint", fmt.Sprintf("bolt:///etc/phenix/store.bdb"), "endpoint for storage service")
		rootCmd.PersistentFlags().StringVar(&errFile, "log.error-file", "/var/log/phenix/error.log", "log fatal errors to file")
	} else {
		rootCmd.PersistentFlags().StringVar(&storeEndpoint, "store.endpoint", fmt.Sprintf("bolt://%s/.phenix.bdb", home), "endpoint for storage service")
		rootCmd.PersistentFlags().StringVar(&errFile, "log.error-file", fmt.Sprintf("%s/.phenix.err", home), "log fatal errors to file")
	}

	viper.BindPFlags(rootCmd.PersistentFlags())
}

func initConfig() {
	viper.SetConfigName("config")

	// Config paths - first look in current directory, then home directory (if
	// discoverable), then finally global config directory.
	viper.AddConfigPath(".")

	uid, home := getCurrentUserInfo()

	// The default config path added below is the same config path that should be
	// used for the root user, so don't worry about handling uid = 0 here.
	if uid != "0" {
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

func getCurrentUserInfo() (string, string) {
	u, err := user.Current()
	if err != nil {
		panic("unable to determine current user: " + err.Error())
	}

	var (
		uid  = u.Uid
		home = u.HomeDir
		sudo = os.Getenv("SUDO_USER")
	)

	// Only trust `SUDO_USER` env variable if we're currently running as root and,
	// if set, use it to lookup the actual user that ran the sudo command.
	if u.Uid == "0" && sudo != "" {
		u, err := user.Lookup(sudo)
		if err != nil {
			panic("unable to lookup sudo user: " + err.Error())
		}

		// `uid` and `home` will now reflect the user ID and home directory of the
		// actual user that ran the sudo command.
		uid = u.Uid
		home = u.HomeDir
	}

	return uid, home
}
