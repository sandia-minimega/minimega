package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"phenix/api/config"
	"phenix/util"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func configKindArgsValidator(multi, allowAll bool) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if multi {
			if len(args) == 0 {
				return fmt.Errorf("Must provide at least one argument")
			}
		} else {
			if narg := len(args); narg != 1 {
				return fmt.Errorf("Expected a single argument, received %d", narg)
			}
		}

		for _, arg := range args {
			tokens := strings.Split(arg, "/")

			if len(tokens) != 2 {
				return fmt.Errorf("Expected an argument in the form of <config kind>/<config name>")
			}

			kinds := []string{"topology", "scenario", "experiment", "image", "user", "role"}

			if allowAll {
				kinds = append(kinds, "all")
			}

			if kind := tokens[0]; !util.StringSliceContains(kinds, kind) {
				return fmt.Errorf("Expects the configuration kind to be one of %v, received %s", kinds, kind)
			}
		}

		return nil
	}
}

func newConfigCmd() *cobra.Command {
	desc := `Configuration file management

  This subcommand is used to manage the different kinds of phenix configuration
  files: topology, scenario, experiment, or image.`

	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"cfg"},
		Short:   "Configuration file management",
		Long:    desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newConfigListCmd() *cobra.Command {
	example := `
  phenix config list all
  phenix config list topology
  phenix config list scenario
  phenix config list experiment
  phenix config list image
  phenix config list user`

	cmd := &cobra.Command{
		Use:       "list <kind>",
		Short:     "Show table of stored configuration files",
		Example:   example,
		ValidArgs: []string{"all", "topology", "scenario", "experiment", "image", "user"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var kinds string

			if len(args) > 0 {
				kinds = args[0]
			}

			configs, err := config.List(kinds)
			if err != nil {
				err := util.HumanizeError(err, "Unable to list known configurations")
				return err.Humanized()
			}

			fmt.Println()

			if len(configs) == 0 {
				fmt.Println("There are no configurations available")
			} else {
				util.PrintTableOfConfigs(os.Stdout, configs)
			}

			fmt.Println()

			return nil
		},
	}

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	desc := `Get a configuration

  This subcommand is used to get a specific configuration file by kind/name.
  Valid options for kinds of configuration files are the same as described
  for the parent config command.`

	example := `
  phenix config get topology/foo
  phenix config get scenario/bar
  phenix config get experiment/foobar`

	cmd := &cobra.Command{
		Use:     "get <kind/name>",
		Short:   "Get a configuration",
		Long:    desc,
		Example: example,
		Args:    configKindArgsValidator(false, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := config.Get(args[0])
			if err != nil {
				err := util.HumanizeError(err, "Unable to get the "+args[0]+" configuration")
				return err.Humanized()
			}

			output := MustGetString(cmd.Flags(), "output")

			switch output {
			case "yaml":
				m, err := yaml.Marshal(c)
				if err != nil {
					err := util.HumanizeError(err, "Unable to convert configuration to YAML")
					return err.Humanized()
				}

				fmt.Println(string(m))
			case "json":
				var (
					m   []byte
					err error
				)

				if MustGetBool(cmd.Flags(), "pretty") {
					m, err = json.MarshalIndent(c, "", "  ")
				} else {
					m, err = json.Marshal(c)
				}

				if err != nil {
					err := util.HumanizeError(err, "Unable to convert configuration to JSON")
					return err.Humanized()
				}

				fmt.Println(string(m))
			default:
				return fmt.Errorf("Unrecognized output format '%s'", output)
			}

			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "yaml", "Configuration output format ('yaml' or 'json')")
	cmd.Flags().BoolP("pretty", "p", false, "Pretty print the JSON output")

	return cmd
}

func newConfigCreateCmd() *cobra.Command {
	desc := `Create a configuration(s)

  This subcommand is used to create one or more configurations from JSON or
  YAML file(s).
	`

	cmd := &cobra.Command{
		Use:   "create </path/to/filename> ...",
		Short: "Create a configuration(s)",
		Long:  desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("Must provide at least one configuration file")
			}

			skip := MustGetBool(cmd.Flags(), "skip-validation")

			for _, f := range args {
				c, err := config.Create(f, !skip)
				if err != nil {
					err := util.HumanizeError(err, "Unable to create configuration from "+f)
					return err.Humanized()
				}

				fmt.Printf("The %s/%s configuration was created\n", c.Kind, c.Metadata.Name)
			}

			return nil
		},
	}

	cmd.Flags().Bool("skip-validation", false, "Skip configuration spec validation against schema")

	return cmd
}

func newConfigEditCmd() *cobra.Command {
	desc := `Edit a configuration

  This subcommand is used to edit a configuration using your default editor.
	`

	cmd := &cobra.Command{
		Use:   "edit <kind/name>",
		Short: "Edit a configuration",
		Long:  desc,
		Args:  configKindArgsValidator(false, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.Edit(args[0])
			if err != nil {
				if config.IsConfigNotModified(err) {
					fmt.Printf("The %s configuration was not updated\n", args[0])
					return nil
				}

				err := util.HumanizeError(err, "Unable to edit the "+args[0]+" configuration provided")
				return err.Humanized()
			}

			fmt.Printf("The %s configuration was updated\n", args[0])

			return nil
		},
	}

	return cmd
}

func newConfigDeleteCmd() *cobra.Command {
	desc := `Delete a configuration(s)

  This subcommand is used to delete one or more configurations.
	`

	cmd := &cobra.Command{
		Use:   "delete <kind/name> ...",
		Short: "Delete a configuration(s)",
		Long:  desc,
		Args:  configKindArgsValidator(true, true),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, c := range args {
				if err := config.Delete(c); err != nil {
					err := util.HumanizeError(err, "Unable to delete the "+c+" configuration")
					return err.Humanized()
				}

				fmt.Printf("The %s configuration was deleted\n", c)
			}

			return nil
		},
	}

	return cmd
}

func init() {
	configCmd := newConfigCmd()

	configCmd.AddCommand(newConfigListCmd())
	configCmd.AddCommand(newConfigGetCmd())
	configCmd.AddCommand(newConfigCreateCmd())
	configCmd.AddCommand(newConfigEditCmd())
	configCmd.AddCommand(newConfigDeleteCmd())

	rootCmd.AddCommand(configCmd)
}
