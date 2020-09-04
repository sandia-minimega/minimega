// +build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"phenix/api/config"
	"phenix/api/experiment"
	"phenix/api/image"
	"phenix/api/vlan"
	"phenix/api/vm"
	"phenix/app"
	"phenix/scheduler"
	"phenix/store"
	v1 "phenix/types/version/v1"
	"phenix/util"
	"phenix/version"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

var (
	f_help      bool
	f_storePath string
)

func init() {
	flag.BoolVar(&f_help, "help", false, "show this help message")
	flag.StringVar(&f_storePath, "store", "phenix.bdb", "path to Bolt store file")
}

func main() {
	app := &cli.App{
		Name:    "phenix",
		Usage:   "A cli application for phēnix",
		Version: version.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "store.endpoint",
				Aliases: []string{"s"},
				Usage:   "endpoint for store service",
				Value:   "bolt://phenix.bdb",
				EnvVars: []string{"PHENIX_STORE_ENDPOINT"},
			},
			&cli.IntFlag{
				Name:    "log.verbosity",
				Aliases: []string{"V"},
				Usage:   "log verbosity (0 - 10)",
				EnvVars: []string{"PHENIX_LOG_VERBOSITY"},
			},
			&cli.StringFlag{
				Name:    "log.error-file",
				Aliases: []string{"e"},
				Usage:   "log fatal errors to file",
				Value:   "phenix.err",
				EnvVars: []string{"PHENIX_LOG_ERROR_FILE"},
			},
			&cli.BoolFlag{
				Name:    "log.error-stderr",
				Aliases: []string{"vvv"},
				Usage:   "log fatal errors to STDERR",
				EnvVars: []string{"PHENIX_LOG_ERROR_STDERR"},
			},
		},
		Before: func(ctx *cli.Context) error {
			if err := store.Init(store.Endpoint(ctx.String("store.endpoint"))); err != nil {
				return cli.Exit(err, 1)
			}

			if err := util.InitFatalLogWriter(ctx.String("log.error-file"), ctx.Bool("log.error-stderr")); err != nil {
				msg := fmt.Sprintf("Unable to initialize fatal log writer: %v", err)
				return cli.Exit(msg, 1)
			}

			return nil
		},
		After: func(ctx *cli.Context) error {
			util.CloseLogWriter()
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:        "config",
				Usage:       "Configuration management",
				Description: "Used to manage the three kinds of configurations; kind can be topology, experiment, or scenario",
				Aliases:     []string{"cfg"},
				Subcommands: []*cli.Command{
					{
						Name:        "list",
						Usage:       "Table of configuration(s)",
						Description: "Used to display a table of available configurations",
						Action: func(ctx *cli.Context) error {
							configs, err := config.List(ctx.Args().First())

							if err != nil {
								err := util.HumanizeError(err, "Unable to list known configurations")
								return cli.Exit(err.Humanize(), 1)
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
					},
					{
						Name:        "get",
						Usage:       "Get a configuration",
						Description: "Used to get a specific configuration; kind can be topology, experiment, or scenario",
						ArgsUsage:   "<config kind>/<config name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Usage:   "Configuration output format can be YAML or JSON",
								Value:   "yaml",
							},
							&cli.BoolFlag{
								Name:    "pretty",
								Aliases: []string{"p"},
								Usage:   "Pretty print the JSON output",
							},
						},
						Action: func(ctx *cli.Context) error {
							c, err := config.Get(ctx.Args().First())
							if err != nil {
								err := util.HumanizeError(err, "Unable to get the "+ctx.Args().Get(0)+ctx.Args().Get(1)+" configuration")
								return cli.Exit(err.Humanize(), 1)
							}

							switch ctx.String("output") {
							case "yaml":
								m, err := yaml.Marshal(c)
								if err != nil {
									err := util.HumanizeError(err, "Unable to convert configuration to YAML")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Println(string(m))
							case "json":
								var (
									m   []byte
									err error
								)

								if ctx.Bool("pretty") {
									m, err = json.MarshalIndent(c, "", "  ")
								} else {
									m, err = json.Marshal(c)
								}

								if err != nil {
									err := util.HumanizeError(err, "Unable to convert configuration to JSON")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Println(string(m))
							default:
								return cli.Exit(fmt.Sprintf("Unrecognized output format '%s'\n", ctx.String("output")), 1)
							}

							return nil
						},
					},
					{
						Name:        "create",
						Usage:       "Create a configuration",
						Description: "Used to create a configuration from file(s); file types can be YAML or JSON",
						ArgsUsage:   "</path/to/filename(s)>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "skip-validation",
								Usage: "Skip configuration spec validation against schema",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("No configuration file(s) were provided", 1)
							}

							for _, f := range ctx.Args().Slice() {
								c, err := config.Create(f, !ctx.Bool("skip-validation"))
								if err != nil {
									err := util.HumanizeError(err, "Unable to create configuration from "+f+"; verify the file is present")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("The %s/%s configuration was created\n", c.Kind, c.Metadata.Name)
							}

							return nil
						},
					},
					{
						Name:        "edit",
						Usage:       "Edit a configuration",
						Description: "Used to edit a configuration with the default system editor; kind can be topology, experiment, or scenario",
						ArgsUsage:   "<config kind>/<config name>",
						Action: func(ctx *cli.Context) error {
							c, err := config.Edit(ctx.Args().First())
							if err != nil {
								if config.IsConfigNotModified(err) {
									return cli.Exit("The "+ctx.Args().Get(0)+ctx.Args().Get(1)+" configuration was not updated", 0)
								}

								err := util.HumanizeError(err, "Unable to edit the "+ctx.Args().Get(0)+ctx.Args().Get(1)+" configuration provided")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s/%s configuration was updated\n", c.Kind, c.Metadata.Name)

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "Delete a configuration",
						Description: "Used to delete a configuration; kind can be topology, experiment, or scenario",
						ArgsUsage:   "<config kind>/<config name>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("No configuration(s) were provided", 1)
							}

							for _, c := range ctx.Args().Slice() {
								if err := config.Delete(c); err != nil {
									err := util.HumanizeError(err, "Unable to delete the "+c+" configuration")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("The %s configuration was deleted\n", c)
							}

							return nil
						},
					},
				},
			},
			{
				Name:        "experiment",
				Usage:       "Experiment management",
				Description: "Used to manage experiment(s)",
				Aliases:     []string{"exp"},
				Subcommands: []*cli.Command{
					{
						Name:        "apps",
						Usage:       "List of available experiment apps",
						Description: "List of available apps to assign an experiment",
						Action: func(ctx *cli.Context) error {
							apps := app.List()

							if len(apps) == 0 {
								fmt.Printf("\nApps: none\n\n")
								return nil
							}

							fmt.Printf("\nApps: %s\n\n", strings.Join(apps, ", "))
							return nil
						},
					},
					{
						Name:        "schedulers",
						Usage:       "List of available experiment schedulers",
						Description: "List of available schedules to assign an experiment",
						Action: func(ctx *cli.Context) error {
							schedulers := scheduler.List()

							if len(schedulers) == 0 {
								fmt.Printf("\nSchedulers: none\n\n")
								return nil
							}

							fmt.Printf("\nSchedulers: %s\n\n", strings.Join(schedulers, ", "))
							return nil
						},
					},
					{
						Name:        "list",
						Usage:       "Table of experiments",
						Description: "Used to display a table of available experiments",
						Action: func(ctx *cli.Context) error {
							exps, err := experiment.List()
							if err != nil {
								return cli.Exit(err, 1)
							}

							util.PrintTableOfExperiments(os.Stdout, exps...)

							return nil
						},
					},
					{
						Name:        "create",
						Usage:       "Create an experiment",
						Description: "Used to create an experiment from an existing configuration; can be a topology, or topology and scenario",
						ArgsUsage:   "<experiment name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "topology",
								Aliases: []string{"t"},
								Usage:   "Name of an existing topology to use",
							},
							&cli.StringFlag{
								Name:    "scenario",
								Aliases: []string{"s"},
								Usage:   "Name of an existing scenario to use (optional)",
							},
							&cli.StringFlag{
								Name:    "base-dir",
								Aliases: []string{"d"},
								Usage:   "Base directory to use for experiment (optional)",
							},
						},
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("Must provide an experiment name", 1)
							}

							var (
								topology = ctx.String("topology")
								scenario = ctx.String("scenario")
								baseDir  = ctx.String("base-dir")
							)

							if topology == "" {
								return cli.Exit("Must provide a topology name", 1)
							}

							if err := experiment.Create(name, topology, scenario, baseDir); err != nil {
								err := util.HumanizeError(err, "Unable to create the "+name+" experiment")
								return cli.Exit(err, 1)
							}

							fmt.Printf("The %s experiment was created\n", name)

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "Delete an experiment",
						Description: "Used to delete an exisitng experiment; experiment must be stopped",
						ArgsUsage:   "<experiment name>",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("Must provide an experiment name", 1)
							}

							exp, err := experiment.Get(ctx.Args().First())
							if err != nil {
								err := util.HumanizeError(err, "Unable to get the "+name+" experiment")
								return cli.Exit(err.Humanize(), 1)
							}

							if exp.Status.Running() {
								return cli.Exit("Cannot delete a running experiment", 1)
							}

							if err := config.Delete("experiment/" + name); err != nil {
								err := util.HumanizeError(err, "Unable to delete the "+name+" experiment")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s experiment was deleted\n", name)

							return nil
						},
					},
					{
						Name:        "schedule",
						Usage:       "Schedule an experiment",
						Description: "Schedule an experiment; run `phenix experiment schedulers` to get a list of algorithms",
						ArgsUsage:   "<experiment name> <algorithm>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("Must provide an experiment name and algorithm", 1)
							}

							var (
								exp  = ctx.Args().Get(0)
								algo = ctx.Args().Get(1)
							)

							if err := experiment.Schedule(exp, algo); err != nil {
								err := util.HumanizeError(err, "Unable to schedule the "+exp+" experiment with the "+algo+" algorithm")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s experiment was scheduled with %s\n", exp, algo)

							return nil
						},
					},
					{
						Name:        "start",
						Usage:       "Start an experiment",
						Description: "Used to start a stopped experiment, `all` will include all experiments; dry-run will do everything but call out to minimega",
						ArgsUsage:   "<experiment name>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Do everything but actually call out to minimega",
							},
						},
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("Must provide an experiment name", 1)
							}

							var (
								dryrun      = ctx.Bool("dry-run")
								experiments []string
							)

							if ctx.Args().First() == "all" {
								exps, err := experiment.List()
								if err != nil {
									err := util.HumanizeError(err, "Unable to start all experiments")
									return cli.Exit(err.Humanize(), 1)
								}

								for _, exp := range exps {
									if exp.Status.StartTime == "" {
										experiments = append(experiments, exp.Spec.ExperimentName)
									}
								}
							} else {
								experiments = []string{ctx.Args().First()}
							}

							for _, exp := range experiments {
								if err := experiment.Start(exp, dryrun); err != nil {
									err := util.HumanizeError(err, "Unable to start the "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								if dryrun {
									fmt.Printf("The %s experiment was started in a dry-run\n", exp)
								} else {
									fmt.Printf("The %s experiment was started\n", exp)
								}
							}

							return nil
						},
					},
					{
						Name:        "stop",
						Usage:       "Stop an experiment",
						Description: "Used to stop a running experiment, `all` will include all experiments",
						ArgsUsage:   "<experiment name>",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("Must provide an experiment name", 1)
							}

							var experiments []string

							if ctx.Args().First() == "all" {
								exps, err := experiment.List()
								if err != nil {
									err := util.HumanizeError(err, "Unable to stop all experiments")
									return cli.Exit(err.Humanize(), 1)
								}

								for _, exp := range exps {
									if exp.Status.StartTime != "" {
										experiments = append(experiments, exp.Spec.ExperimentName)
									}
								}
							} else {
								experiments = []string{ctx.Args().First()}
							}

							for _, exp := range experiments {
								if err := experiment.Stop(exp); err != nil {
									err := util.HumanizeError(err, "Unable to stop the "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("The %s experiment was stopped\n", exp)
							}

							return nil
						},
					},
					{
						Name:        "restart",
						Usage:       "Restart an experiment",
						Description: "Used to restart a running experiment; dry-run will do everything but call out to minimega",
						ArgsUsage:   "<experiment name>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Do everything but actually call out to minimega",
							},
						},
						Action: func(ctx *cli.Context) error {
							var (
								exp    = ctx.Args().First()
								dryrun = ctx.Bool("dry-run")
							)

							if err := experiment.Stop(exp); err != nil {
								err := util.HumanizeError(err, "Unable to stop the "+exp+" experiment")
								return cli.Exit(err.Humanize(), 1)
							}

							if err := experiment.Start(exp, dryrun); err != nil {
								err := util.HumanizeError(err, "Unable to start the "+exp+" experiment")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s experiment was restarted\n", exp)

							return nil
						},
					},
				},
			},
			{
				Name:        "vm",
				Usage:       "Virtual machine management",
				Description: "Used to manage virtual machine(s)",
				Subcommands: []*cli.Command{
					{
						Name:        "info",
						Usage:       "Table of virtual machine(s)",
						Description: "Used to display a table of virtual machine(s) for a specific experiment; virtual machine name is optional, when included will display only that VM",
						ArgsUsage:   "<experiment name> *OR* <experiment name>/<vm name>",
						Action: func(ctx *cli.Context) error {
							parts := strings.Split(ctx.Args().First(), "/")

							switch len(parts) {
							case 1:
								vms, err := vm.List(parts[0])
								if err != nil {
									err := util.HumanizeError(err, "Unable to get a list of VMs")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVMs(os.Stdout, vms...)
							case 2:
								vm, err := vm.Get(parts[0], parts[1])
								if err != nil {
									err := util.HumanizeError(err, "Unable to get information for the "+parts[1]+" VM")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVMs(os.Stdout, *vm)
							default:
								return cli.Exit("Invalid argument", 1)
							}

							return nil
						},
					},
					{
						Name:        "pause",
						Usage:       "Pause a running virtual machine",
						Description: "Used to pause a running virtual machine for a speific experiment",
						ArgsUsage:   "<experiment name> <vm name>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("Must provide an experiment and virtual machine name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Pause(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to pause the "+vmName+" VM")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s VM in the %s experiment was paused\n", vmName, expName)

							return nil
						},
					},
					{
						Name:        "resume",
						Usage:       "Resume a paused virtual machine",
						Description: "Used to resume a paused virtul machine for a specific experiment",
						ArgsUsage:   "<experiment name> <vm name>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("Must provide an experiment and virtual machine name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Resume(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to resume the "+vmName+" VM")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s VM in the %s experiment was resumed\n", vmName, expName)

							return nil
						},
					},
					{
						Name:        "redeploy",
						Usage:       "Redeploy a running virtual machine",
						Description: "Used to redeploy a running virtual machine for a specific experiment; several values can be modified",
						ArgsUsage:   "<experiment name> <vm name>",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "cpu",
								Aliases: []string{"c"},
								Usage:   "Number of VM CPUs (1-8 is valid)",
							},
							&cli.IntFlag{
								Name:    "mem",
								Aliases: []string{"m"},
								Usage:   "Amount of memory in megabytes (512, 1024, 2048, 3072, 4096, 8192, 12288, 16384 are valid)",
							},
							&cli.StringFlag{
								Name:    "disk",
								Aliases: []string{"d"},
								Usage:   "VM backing disk image",
							},
							&cli.BoolFlag{
								Name:    "replicate-injects",
								Aliases: []string{"r"},
								Usage:   "Recreate disk snapshot and VM injections",
							},
							&cli.IntFlag{
								Name:    "partition",
								Aliases: []string{"p"},
								Usage:   "Partition of disk to inject files into (only used if disk option is specified)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("Must provide an experiment name and VM name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
								cpu     = ctx.Int("cpu")
								mem     = ctx.Int("mem")
								disk    = ctx.String("disk")
								inject  = ctx.Bool("replicate-injects")
								part    = ctx.Int("partition")
							)

							if cpu != 0 && (cpu < 1 || cpu > 8) {
								return cli.Exit("CPUs can only be 1-8", 1)
							}

							if mem != 0 && (mem < 512 || mem > 16384 || mem%512 != 0) {
								return cli.Exit("Memory must be one of 512, 1024, 2048, 3072, 4096, 8192, 12288, 16384", 1)
							}

							opts := []vm.RedeployOption{
								vm.CPU(cpu),
								vm.Memory(mem),
								vm.Disk(disk),
								vm.Inject(inject),
								vm.InjectPartition(part),
							}

							if err := vm.Redeploy(expName, vmName, opts...); err != nil {
								err := util.HumanizeError(err, "Unable to redeploy the "+vmName+" VM")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s VM in the %s experiment was redeployed\n", vmName, expName)

							return nil
						},
					},
					{
						Name:        "kill",
						Usage:       "Kill a running or paused virtual machine",
						Description: "Used to kill or delete a running or paused virtual machine for a specific experiment",
						ArgsUsage:   "<experiment name> <vm name>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("Must provide an experiment name and VM name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Kill(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to kill the "+vmName+" VM")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s VM in the %s experiment was killed\n", vmName, expName)

							return nil
						},
					},
					{
						Name:        "set",
						Usage:       "Set configuration value for a virtual machine",
						Description: "Used to set a configuration value for a virtual machine in a stopped experiment",
						Action: func(ctx *cli.Context) error {
							return cli.Exit("This command is not yet implemented. For now, you can edit the experiment directly with `phenix config edit`.", 1)
						},
					},
					{
						Name:        "net",
						Usage:       "Modify network connectivity for a virtual machine",
						Description: "Used to modify the network connectivity for a virtual machine in a running experiment; see command help for connect or disconnect for additional arguments",
						Subcommands: []*cli.Command{
							{
								Name:      "connect",
								Usage:     "Connect a VM interface to a VLAN",
								ArgsUsage: "<experiment name> <vm name> <iface index> <vlan id>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 4 {
										return cli.Exit("Must provide an experiment name, VM name, IFACE index, VLAN ID", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
										vlan    = ctx.Args().Get(3)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("The network interface index must be an integer", 1)
									}

									if err := vm.Connect(expName, vmName, iface, vlan); err != nil {
										err := util.HumanizeError(err, "Unable to modify the connectivity for the "+vmName+" VM")
										return cli.Exit(err.Humanize(), 1)
									}

									fmt.Printf("The network for the %s VM in the %s experiment was modified\n", vmName, expName)

									return nil
								},
							},
							{
								Name:      "disconnect",
								Usage:     "Disconnect a vm interface",
								ArgsUsage: "<experiment name> <vm name> <iface index>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 3 {
										return cli.Exit("Must provide an experiment name, VM name, IFACE index", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("The network interface index must be an integer", 1)
									}

									if err := vm.Disonnect(expName, vmName, iface); err != nil {
										err := util.HumanizeError(err, "Unable to disconnect the interface on the "+vmName+" VM")
										return cli.Exit(err.Humanize(), 1)
									}

									fmt.Printf("The %s interface on the %s VM in the %s experiment was paused\n", iface, vmName, expName)

									return nil
								},
							},
						},
					},
					{
						Name:        "capture",
						Usage:       "Modify network packet captures for a virutal machine",
						Description: "Used to modify the network packet captures for a virtual machine in a running experiment; see command help for start and stop for additional arguments",
						Subcommands: []*cli.Command{
							{
								Name:      "start",
								Usage:     "Start a packet capture",
								ArgsUsage: "<experiment name> <vm name> <iface index> </path/to/out file>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 4 {
										return cli.Exit("Must provide an experiment name, VM name, IFACE index, /path/to/out file", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
										out     = ctx.Args().Get(3)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("The network interface index must be an integer", 1)
									}

									if err := vm.StartCapture(expName, vmName, iface, out); err != nil {
										err := util.HumanizeError(err, "Unable to start a capture on the interface on the "+vmName+" VM")
										return cli.Exit(err.Humanize(), 1)
									}

									fmt.Printf("A packet capture was started for the %s interface on the %s VM in the %s experiment\n", iface, vmName, expName)

									return nil
								},
							},
							{
								Name:      "stop",
								Usage:     "Stop packet capture(s)",
								ArgsUsage: "<experiment name> <vm name>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 2 {
										return cli.Exit("Must provide an experiment name and VM name", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
									)

									if err := vm.StopCaptures(expName, vmName); err != nil {
										err := util.HumanizeError(err, "Unable to stop the packet capture(s) on the "+vmName+" VM")
										return cli.Exit(err.Humanize(), 1)
									}

									fmt.Printf("The packet capture(s) for the %s VM in the %s experiment was stopped\n", vmName, expName)

									return nil
								},
							},
						},
					},
				},
			},
			{
				Name:        "image",
				Usage:       "Virtual disk image management",
				Description: "Used to manage virtual disk image(s)",
				Subcommands: []*cli.Command{
					{
						Name:        "create",
						Usage:       "Create image configuration",
						Description: "Used to create a virtual disk image configuration from which to build an image",
						ArgsUsage:   "<image name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "size",
								Aliases: []string{"z"},
								Usage:   "Image size to use",
								Value:   "5G",
							},
							&cli.StringFlag{
								Name:    "variant",
								Aliases: []string{"v"},
								Usage:   "Image variant to use",
								Value:   "minbase",
							},
							&cli.StringFlag{
								Name:    "release",
								Aliases: []string{"R"},
								Usage:   "OS release codename",
								Value:   "bionic",
							},
							&cli.StringFlag{
								Name:    "mirror",
								Aliases: []string{"m"},
								Usage:   "Debootstrap mirror (must match release)",
								Value:   "http://us.archive.ubuntu.com/ubuntu/",
							},
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Usage:   "Format of disk image",
								Value:   "raw",
							},
							&cli.BoolFlag{
								Name:    "compress",
								Aliases: []string{"c"},
								Usage:   "Compress image after creation (does not apply to raw image)",
							},
							&cli.BoolFlag{
								Name:    "ramdisk",
								Aliases: []string{"r"},
								Usage:   "Create a kernel/initrd pair in addition to a disk image",
							},
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "List of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "List of packages to include in addition to those provided by variant (include full path; separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "List of scripts to include in addition to the default one (include full path; separated by comma)",
							},
							&cli.StringFlag{
								Name:    "debootstrap-append",
								Aliases: []string{"d"},
								Usage:   "Additional arguments to debootstrap (default: --components=main,restricted,universe,multiverse)",
							},
						},
						Action: func(ctx *cli.Context) error {
							var img v1.Image

							name := ctx.Args().First()
							img.Size = ctx.String("size")
							img.Variant = ctx.String("variant")
							img.Release = ctx.String("release")
							img.Mirror = ctx.String("mirror")
							img.Format = v1.Format(ctx.String("format"))
							img.Compress = ctx.Bool("compress")
							img.Ramdisk = ctx.Bool("ramdisk")
							img.DebAppend = ctx.String("debootstrap-append")

							if overlays := ctx.String("overlays"); overlays != "" {
								img.Overlays = strings.Split(overlays, ",")
							}

							if packages := ctx.String("packages"); packages != "" {
								img.Packages = strings.Split(packages, ",")
							}

							if scripts := ctx.String("scripts"); scripts != "" {
								img.ScriptPaths = strings.Split(scripts, ",")
							}

							if err := image.Create(name, &img); err != nil {
								err := util.HumanizeError(err, "Unable to create the "+name+" image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The configuration for the %s image was created\n", name)

							return nil
						},
					},
					{
						Name:        "create-from",
						Usage:       "Create image configuration from existing one",
						Description: "Used to create a new virtual disk image configuration from an existing one; if options are used they will be added to the exisiting configuration",
						ArgsUsage:   "<existing name> <new name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "List of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "List of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "List of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("The name of a existing configuration is required", 1)
							}

							if ctx.Args().Get(1) == "" {
								return cli.Exit("The name for the new configuration is required", 1)
							}

							var (
								name     = ctx.Args().First()
								saveas   = ctx.Args().Get(1)
								overlays []string
								packages []string
								scripts  []string
							)

							if opt := ctx.String("overlays"); opt != "" {
								overlays = strings.Split(opt, ",")
							}

							if opt := ctx.String("packages"); opt != "" {
								packages = strings.Split(opt, ",")
							}

							if opt := ctx.String("scripts"); opt != "" {
								scripts = strings.Split(opt, ",")
							}

							if err := image.CreateFromConfig(name, saveas, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to create the configuration file "+saveas)
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The configuration for the %s image was created from %s\n", saveas, name)

							return nil
						},
					},
					{
						Name:        "build",
						Usage:       "Build an virtual disk image",
						Description: "Used to build a new virtual disk using an exisitng configuration; vmdb2 must be in path",
						ArgsUsage:   "<configuration name>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Enable verbose output",
							},
							&cli.BoolFlag{
								Name:    "very-verbose",
								Aliases: []string{"vv"},
								Usage:   "Enable very verbose output",
							},
							&cli.BoolFlag{
								Name:    "very-very-verbose",
								Aliases: []string{"vvv"},
								Usage:   "Enable very verbose output plus additional verbose output from debootstrap",
							},
							&cli.BoolFlag{
								Name:    "cache",
								Aliases: []string{"c"},
								Usage:   "Cache rootfs as tar archive",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Do everything but actually call out to vmdb2",
							},
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Usage:   "Specify the output directory for the disk image to be saved to",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("The name of a configuration to build the disk image is required", 1)
							}

							var (
								name      = ctx.Args().First()
								cache     = ctx.Bool("cache")
								dryrun    = ctx.Bool("dry-run")
								output    string
								verbosity int
							)

							if ctx.String("output") == "" {
								cwd, err := os.Getwd()
								if err != nil {
									err := util.HumanizeError(err, "Unable to get the current working directory")
									return cli.Exit(err.Humanize(), 1)
								}

								output = cwd
							} else {
								output = ctx.String("output")
							}

							if ctx.Bool("verbose") {
								verbosity = verbosity | image.V_VERBOSE
							}

							if ctx.Bool("very-verbose") {
								verbosity = verbosity | image.V_VVERBOSE
							}

							if ctx.Bool("very-very-verbose") {
								verbosity = verbosity | image.V_VVVERBOSE
							}

							if err := image.Build(name, verbosity, cache, dryrun, output); err != nil {
								err := util.HumanizeError(err, "Unable to build the "+name+" image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The %s image was successfully built\n", name)

							return nil
						},
					},
					{
						Name:        "list",
						Usage:       "Table of image configuration(s)",
						Description: "Used to display a table of exisitng virtual disk image configuration(s); includes size, vairant, release, overlays, packages, and scripts",
						ArgsUsage:   "[flag]",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Usage:   "Include disk image format",
							},
							&cli.BoolFlag{
								Name:    "compressed",
								Aliases: []string{"c"},
								Usage:   "Include disk compression",
							},
							&cli.BoolFlag{
								Name:    "mirror",
								Aliases: []string{"m"},
								Usage:   "Include debootstrap mirror",
							},
						},
						Action: func(ctx *cli.Context) error {
							imgs, err := image.List()
							if err != nil {
								err := util.HumanizeError(err, "Unable to print a list of configurations")
								return cli.Exit(err.Humanize(), 1)
							}

							var optional []string

							if ctx.Bool("format") {
								optional = append(optional, "Format")
							}

							if ctx.Bool("compressed") {
								optional = append(optional, "Compressed")
							}

							if ctx.Bool("mirror") {
								optional = append(optional, "Mirror")
							}

							util.PrintTableOfImageConfigs(os.Stdout, optional, imgs...)

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "Delete image configuration",
						Description: "Used to delete an existing virtual disk image configuration by name",
						ArgsUsage:   "<image name>",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("The name of the configuration to delete is required", 1)
							}

							if err := config.Delete("image/" + name); err != nil {
								err := util.HumanizeError(err, "Unable to delete the "+name+" image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("The configuration for the %s image was deleted\n", name)

							return nil
						},
					},
					{
						Name:        "append",
						Usage:       "Append to an image configuration",
						Description: "Used to add scripts, packages, and/or overlays to an existing virtual disk image configuration",
						ArgsUsage:   "<image name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "List of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "List of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "List of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("The name of a configuration to append to is required", 1)
							}

							var (
								name     = ctx.Args().First()
								overlays = strings.Split(ctx.String("overlays"), ",")
								packages = strings.Split(ctx.String("packages"), ",")
								scripts  = strings.Split(ctx.String("scripts"), ",")
							)

							if err := image.Append(name, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to append to the "+name+" image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("Scripts, packages, and/or overlays for the %s configuration were appended\n", name)

							return nil
						},
					},
					{
						Name:        "remove",
						Usage:       "Remove from an image configuration",
						Description: "Used to remove scripts, packages, and/or overlays to an existing virtual disk image configuration",
						ArgsUsage:   "<image name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "List of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "List of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "List of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("The name of a configuration to remove from is required", 1)
							}

							var (
								name     = ctx.Args().First()
								overlays = strings.Split(ctx.String("overlays"), ",")
								packages = strings.Split(ctx.String("packages"), ",")
								scripts  = strings.Split(ctx.String("scripts"), ",")
							)

							if err := image.Remove(name, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to remove from the "+name+" image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("Scripts, packages, and/or overlays for the %s configuration were removed\n", name)

							return nil
						},
					},
				},
			},
			{
				Name:        "vlan",
				Usage:       "VLAN management",
				Description: "Used to manage VLAN(s)",
				Subcommands: []*cli.Command{
					{
						Name:        "alias",
						Usage:       "View or set a VLAN alias",
						Description: "Used to view or set an alias for a given VLAN ID",
						ArgsUsage:   "<experiment name> <alias name> <vlan id>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Force update on set action if alias already exists",
							},
						},
						Action: func(ctx *cli.Context) error {
							switch ctx.NArg() {
							case 0:
								info, err := vlan.Aliases()
								if err != nil {
									err := util.HumanizeError(err, "Unable to display all aliases")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANAliases(os.Stdout, info)
							case 1:
								exp := ctx.Args().First()

								info, err := vlan.Aliases(vlan.Experiment(exp))
								if err != nil {
									err := util.HumanizeError(err, "Unable to display aliases for the "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANAliases(os.Stdout, info)
							case 3:
								var (
									exp   = ctx.Args().Get(0)
									alias = ctx.Args().Get(1)
									id    = ctx.Args().Get(2)
									force = ctx.Bool("force")
								)

								vid, err := strconv.Atoi(id)
								if err != nil {
									return cli.Exit("The VLAN identifier provided is not a valid integer", 1)
								}

								if err := vlan.SetAlias(vlan.Experiment(exp), vlan.Alias(alias), vlan.ID(vid), vlan.Force(force)); err != nil {
									err := util.HumanizeError(err, "Unable to set the alias for the "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("The VLAN alias %s was set for the %s experiment\n", alias, exp)
							default:
								return cli.Exit("There were an unexpected number of arguments provided", 1)
							}

							return nil
						},
					},
					{
						Name:        "range",
						Usage:       "View or set a vlan range",
						Description: "Used to view or set a range for a given VLAN",
						ArgsUsage:   "<experiment name> <range minimum> <range maximum>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Force update on set action if range is already set",
							},
						},
						Action: func(ctx *cli.Context) error {
							switch ctx.NArg() {
							case 0:
								info, err := vlan.Ranges()
								if err != nil {
									err := util.HumanizeError(err, "Unable to display VLAN range(s)")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANRanges(os.Stdout, info)
							case 1:
								exp := ctx.Args().First()

								info, err := vlan.Ranges(vlan.Experiment(exp))
								if err != nil {
									err := util.HumanizeError(err, "Unable to display VLAN range(s) for "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANRanges(os.Stdout, info)
							case 3:
								var (
									exp   = ctx.Args().Get(0)
									min   = ctx.Args().Get(1)
									max   = ctx.Args().Get(2)
									force = ctx.Bool("force")
								)

								vmin, err := strconv.Atoi(min)
								if err != nil {
									return cli.Exit("The VLAN range minimum identifier provided is not a valid integer", 1)
								}

								vmax, err := strconv.Atoi(max)
								if err != nil {
									return cli.Exit("The VLAN range maximum identifier provided is not a valid integer", 1)
								}

								if err := vlan.SetRange(vlan.Experiment(exp), vlan.Min(vmin), vlan.Max(vmax), vlan.Force(force)); err != nil {
									err := util.HumanizeError(err, "Unable to set the VLAN range for the "+exp+" experiment")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("The VLAN range was set for the %s experiment\n", exp)
							default:
								return cli.Exit("There were an unexpected number of arguments provided", 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:  "util",
				Usage: "Utility commands",
				Subcommands: []*cli.Command{
					{
						Name:      "app-json",
						Usage:     "Print application JSON input for given experiment to STDOUT",
						ArgsUsage: "<experiment>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "pretty",
								Aliases: []string{"p"},
								Usage:   "pretty print JSON output",
							},
						},
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("There was no experiment provided", 1)
							}

							exp, err := experiment.Get(name)
							if err != nil {
								err := util.HumanizeError(err, "Unable to get the "+name+" experiment")
								return cli.Exit(err.Humanize(), 1)
							}

							var m []byte

							if ctx.Bool("pretty") {
								m, err = json.MarshalIndent(exp, "", "  ")
							} else {
								m, err = json.Marshal(exp)
							}

							if err != nil {
								err := util.HumanizeError(err, "Unable to convert experiment to JSON")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Println(string(m))

							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
	}
}
