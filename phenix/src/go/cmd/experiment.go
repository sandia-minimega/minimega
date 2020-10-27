package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"phenix/api/config"
	"phenix/api/experiment"
	"phenix/app"
	"phenix/scheduler"
	"phenix/types"
	"phenix/util"
	"phenix/util/printer"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newExperimentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "experiment",
		Aliases: []string{"exp"},
		Short:   "Experiment management",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}

func newExperimentListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Display a table of available experiments",
		RunE: func(cmd *cobra.Command, args []string) error {
			exps, err := experiment.List()
			if err != nil {
				err := util.HumanizeError(err, "Unable to list known experiments")
				return err.Humanized()
			}

			if len(exps) == 0 {
				fmt.Println("\nThere are no experiments available\n")
			} else {
				printer.PrintTableOfExperiments(os.Stdout, exps...)
			}

			return nil
		},
	}

	return cmd
}

func newExperimentAppsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "List of available apps to assign an experiment",
		RunE: func(cmd *cobra.Command, args []string) error {
			apps := app.List()

			if len(apps) == 0 {
				fmt.Printf("\nApps: none\n\n")
				return nil
			}

			fmt.Printf("\nApps: %s\n", strings.Join(apps, ", "))

			return nil
		},
	}

	return cmd
}

func newExperimentSchedulersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedulers",
		Short: "List of available schedulers to assign an experiment",
		RunE: func(cmd *cobra.Command, args []string) error {
			schedulers := scheduler.List()

			if len(schedulers) == 0 {
				fmt.Printf("\nSchedulers: none\n\n")
				return nil
			}

			fmt.Printf("\nSchedulers: %s\n", strings.Join(schedulers, ", "))

			return nil
		},
	}

	return cmd
}

func newExperimentCreateCmd() *cobra.Command {
	desc := `Create an experiment

  Used to create an experiment from existing configurations; can be a
  topology, or topology and scenario, or paths to topology/scenario
  configuration files (YAML or JSON). (Optional are the arguments for
  scenario or base directory.)`

	example := `
  phenix experiment create <experiment name> -t <topology name or /path/to/filename>
  phenix experiment create <experiment name> -t <topology name or /path/to/filename> -s <scenario name or /path/to/filename>
  phenix experiment create <experiment name> -t <topology name or /path/to/filename> -s <scenario name or /path/to/filename> -d </path/to/dir/>`

	cmd := &cobra.Command{
		Use:     "create <experiment name>",
		Short:   "Create an experiment",
		Long:    desc,
		Example: example,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("Must provide an experiment name")
			}

			var (
				topology = MustGetString(cmd.Flags(), "topology")
				scenario = MustGetString(cmd.Flags(), "scenario")
			)

			if ext := filepath.Ext(topology); ext != "" {
				c, err := config.Create(topology, true)
				if err != nil {
					err := util.HumanizeError(err, "Unable to create configuration from "+topology)
					return err.Humanized()
				}

				topology = c.Metadata.Name
			}

			// If scenario is not provided, then ext will be an empty string, so the
			// following won't be run.
			if ext := filepath.Ext(scenario); ext != "" {
				c, err := config.Create(scenario, true)
				if err != nil {
					err := util.HumanizeError(err, "Unable to create configuration from "+scenario)
					return err.Humanized()
				}

				scenario = c.Metadata.Name
			}

			opts := []experiment.CreateOption{
				experiment.CreateWithName(args[0]),
				experiment.CreateWithTopology(topology),
				experiment.CreateWithScenario(scenario),
				experiment.CreateWithBaseDirectory(MustGetString(cmd.Flags(), "base-dir")),
				experiment.CreateWithVLANMin(MustGetInt(cmd.Flags(), "vlan-min")),
				experiment.CreateWithVLANMax(MustGetInt(cmd.Flags(), "vlan-max")),
			}

			ctx := context.Background()

			if err := experiment.Create(ctx, opts...); err != nil {
				err := util.HumanizeError(err, "Unable to create the "+args[0]+" experiment")
				return err.Humanized()
			}

			if warns := util.Warnings(ctx); warns != nil {
				printer := color.New(color.FgYellow)

				for _, warn := range warns {
					printer.Printf("[WARNING] %v\n", warn)
				}
			}

			fmt.Printf("The %s experiment was created\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringP("topology", "t", "", "Name of an existing topology to use")
	cmd.MarkFlagRequired("topology")
	cmd.Flags().StringP("scenario", "s", "", "Name of an existing scenario to use (optional)")
	cmd.Flags().StringP("base-dir", "d", "", "Base directory to use for experiment (optional)")
	cmd.Flags().Int("vlan-min", 0, "VLAN pool minimum")
	cmd.Flags().Int("vlan-max", 0, "VLAN pool maximum")

	return cmd
}

func newExperimentDeleteCmd() *cobra.Command {
	desc := `Delete an experiment

  Used to delete an exisitng experiment; experiment must be stopped.
  Using 'all' instead of a specific experiment name will include all 
  stopped experiments`

	cmd := &cobra.Command{
		Use:   "delete <experiment name>",
		Short: "Delete an experiment",
		Long:  desc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				name        = args[0]
				experiments []types.Experiment
			)

			if name == "all" {
				var err error

				experiments, err = experiment.List()
				if err != nil {
					err := util.HumanizeError(err, "Unable to delete all experiments")
					return err.Humanized()
				}
			} else {
				exp, err := experiment.Get(name)
				if err != nil {
					err := util.HumanizeError(err, "Unable to delete the "+name+" experiment")
					return err.Humanized()
				}

				experiments = []types.Experiment{*exp}
			}

			for _, exp := range experiments {
				if exp.Running() {
					fmt.Printf("Not deleting running experiment %s\n", exp.Metadata.Name)
					continue
				}

				if err := config.Delete("experiment/" + exp.Metadata.Name); err != nil {
					err := util.HumanizeError(err, "Unable to delete the "+exp.Metadata.Name+" experiment")
					fmt.Println(err.Humanize())
					continue
				}

				fmt.Printf("The %s experiment was deleted\n", exp.Metadata.Name)
			}

			return nil
		},
	}

	return cmd
}

func newExperimentScheduleCmd() *cobra.Command {
	desc := `Schedule an experiment
	
  Apply an algorithm to a given experiment. Run 'phenix experiment schedulers' 
  to return a list of algorithms`

	cmd := &cobra.Command{
		Use:   "schedule <experiment name> <algorithm>",
		Short: "Schedule an experiment",
		Long:  desc,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []experiment.ScheduleOption{
				experiment.ScheduleForName(args[0]),
				experiment.ScheduleWithAlgorithm(args[1]),
			}

			if err := experiment.Schedule(opts...); err != nil {
				err := util.HumanizeError(err, "Unable to schedule the "+args[0]+" experiment with the "+args[1]+" algorithm")
				return err.Humanized()
			}

			fmt.Printf("The %s experiment was scheduled with %s\n", args[0], args[1])

			return nil
		},
	}

	return cmd
}

func newExperimentStartCmd() *cobra.Command {
	desc := `Start an experiment

  Used to start a stopped experiment, using 'all' instead of a specific 
  experiment name will include all stopped experiments; dry-run will do 
  everything but call out to minimega.`

	cmd := &cobra.Command{
		Use:   "start <experiment name>",
		Short: "Start an experiment",
		Long:  desc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				name        = args[0]
				dryrun      = MustGetBool(cmd.Flags(), "dry-run")
				experiments []types.Experiment
			)

			if name == "all" {
				var err error

				experiments, err = experiment.List()
				if err != nil {
					err := util.HumanizeError(err, "Unable to start all experiments")
					return err.Humanized()
				}
			} else {
				exp, err := experiment.Get(name)
				if err != nil {
					err := util.HumanizeError(err, "Unable to start the "+name+" experiment")
					return err.Humanized()
				}

				experiments = []types.Experiment{*exp}
			}

			for _, exp := range experiments {
				if exp.Running() {
					fmt.Printf("Not starting already running experiment %s\n", exp.Metadata.Name)
					continue
				}

				opts := []experiment.StartOption{
					experiment.StartWithName(exp.Metadata.Name),
					experiment.StartWithDryRun(dryrun),
					experiment.StartWithVLANMin(MustGetInt(cmd.Flags(), "vlan-min")),
					experiment.StartWithVLANMax(MustGetInt(cmd.Flags(), "vlan-max")),
				}

				if err := experiment.Start(opts...); err != nil {
					err := util.HumanizeError(err, "Unable to start the "+exp.Metadata.Name+" experiment")
					return err.Humanized()
				}

				if dryrun {
					fmt.Printf("The %s experiment was started in a dry-run\n", exp.Metadata.Name)
				} else {
					fmt.Printf("The %s experiment was started\n", exp.Metadata.Name)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolP("dry-run", "", false, "Do everything but actually call out to minimega")
	cmd.Flags().Int("vlan-min", 0, "VLAN pool minimum")
	cmd.Flags().Int("vlan-max", 0, "VLAN pool maximum")

	return cmd
}

func newExperimentStopCmd() *cobra.Command {
	desc := `Stop an experiment

  Used to stop a running experiment, using 'all' instead of a specific 
  experiment name will include all running experiments.`

	cmd := &cobra.Command{
		Use:   "stop <experiment name>",
		Short: "Stop an experiment",
		Long:  desc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				name        = args[0]
				experiments []types.Experiment
			)

			if name == "all" {
				var err error

				experiments, err = experiment.List()
				if err != nil {
					err := util.HumanizeError(err, "Unable to stop all experiments")
					return err.Humanized()
				}
			} else {
				exp, err := experiment.Get(name)
				if err != nil {
					err := util.HumanizeError(err, "Unable to stop the "+name+" experiment")
					return err.Humanized()
				}

				experiments = []types.Experiment{*exp}
			}

			for _, exp := range experiments {
				if !exp.Running() {
					fmt.Printf("Not stopping already stopped experiment %s\n", exp.Metadata.Name)
					continue
				}

				if err := experiment.Stop(exp.Metadata.Name); err != nil {
					err := util.HumanizeError(err, "Unable to stop the "+exp.Metadata.Name+" experiment")
					return err.Humanized()
				}

				fmt.Printf("The %s experiment was stopped\n", exp.Metadata.Name)
			}

			return nil
		},
	}

	return cmd
}

func newExperimentRestartCmd() *cobra.Command {
	desc := `Restart an experiment

  Used to restart a running experiment, using 'all' instead of a specific 
  experiment name will include all running experiments; dry-run will do 
  everything but call out to minimega.`

	cmd := &cobra.Command{
		Use:   "restart <experiment name>",
		Short: "Start an experiment",
		Long:  desc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				name        = args[0]
				dryrun      = MustGetBool(cmd.Flags(), "dry-run")
				experiments []types.Experiment
			)

			if name == "all" {
				var err error

				experiments, err = experiment.List()
				if err != nil {
					err := util.HumanizeError(err, "Unable to restart all experiments")
					return err.Humanized()
				}
			} else {
				exp, err := experiment.Get(name)
				if err != nil {
					err := util.HumanizeError(err, "Unable to restart the "+name+" experiment")
					return err.Humanized()
				}

				experiments = []types.Experiment{*exp}
			}

			for _, exp := range experiments {
				if !exp.Running() {
					fmt.Printf("Not restarting stopped experiment %s\n", exp.Metadata.Name)
					continue
				}

				if err := experiment.Stop(exp.Metadata.Name); err != nil {
					err := util.HumanizeError(err, "Unable to stop the "+exp.Metadata.Name+" experiment")
					return err.Humanized()
				}

				if err := experiment.Start(experiment.StartWithName(exp.Metadata.Name), experiment.StartWithDryRun(dryrun)); err != nil {
					err := util.HumanizeError(err, "Unable to start the "+exp.Metadata.Name+" experiment")
					return err.Humanized()
				}

				fmt.Printf("The %s experiment was restarted\n", exp.Metadata.Name)
			}

			return nil
		},
	}

	cmd.Flags().BoolP("dry-run", "", false, "Do everything but actually call out to minimega")

	return cmd
}

func init() {
	experimentCmd := newExperimentCmd()

	experimentCmd.AddCommand(newExperimentListCmd())
	experimentCmd.AddCommand(newExperimentAppsCmd())
	experimentCmd.AddCommand(newExperimentSchedulersCmd())
	experimentCmd.AddCommand(newExperimentCreateCmd())
	experimentCmd.AddCommand(newExperimentDeleteCmd())
	experimentCmd.AddCommand(newExperimentScheduleCmd())
	experimentCmd.AddCommand(newExperimentStartCmd())
	experimentCmd.AddCommand(newExperimentStopCmd())
	experimentCmd.AddCommand(newExperimentRestartCmd())

	rootCmd.AddCommand(experimentCmd)
}
