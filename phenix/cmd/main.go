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
		UsageText: "phenix [global options] <command>",
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
				Usage:       "configuration management",
				UsageText:   "phenix config [command options] <command>", // not showing up as expected
				Description: "used to manage the three kinds of configurations; kind can be topology, experiment, or scenario",
				Aliases:     []string{"cfg"},
				Subcommands: []*cli.Command{
					{
						Name:        "list",
						Usage:       "table of configuration(s)",
						UsageText:   "phenix config list",
						Description: "used to display a table of available configurations",
						Action: func(ctx *cli.Context) error {
							configs, err := config.List(ctx.Args().First())

							if err != nil {
								err := util.HumanizeError(err, "Unable to list known configurations")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Println()

							if len(configs) == 0 {
								fmt.Println("there are no configurations available")
							} else {
								util.PrintTableOfConfigs(os.Stdout, configs)
							}

							fmt.Println()

							return nil
						},
					},
					{
						Name:        "get",
						Usage:       "get a configuration",
						UsageText:   "phenix config get <config kind>/<config name>",
						Description: "used to get a specific configuration; kind can be topology, experiment, or scenario",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Usage:   "configuration output format can be yaml or json",
								Value:   "yaml",
							},
							&cli.BoolFlag{
								Name:    "pretty",
								Aliases: []string{"p"},
								Usage:   "pretty print the json output",
							},
						},
						Action: func(ctx *cli.Context) error {
							c, err := config.Get(ctx.Args().First())
							if err != nil {
								err := util.HumanizeError(err, "Unable to get the " + c.Kind + "/" + c.Metadata.Name + " configuration") // do we want to give the name used instead of given
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
								err := util.HumanizeError(fmt.Errorf("unrecognized output format %s", ctx.String("output")), "") // need to revert?
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "create",
						Usage:       "create a configuration",
						UsageText:   "phenix config create </path/to/filename(s)>",
						Description: "used to create a configuration from file(s); file types can be yaml or json",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "skip-validation",
								Usage: "skip configuration spec validation against schema",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("no configuration file(s) were provided", 1)
							}

							for _, f := range ctx.Args().Slice() {
								c, err := config.Create(f, !ctx.Bool("skip-validation"))
								if err != nil {
									err := util.HumanizeError(err, "Unable to create configuration "+f)
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("%s/%s configuration was created\n", c.Kind, c.Metadata.Name)
							}

							return nil
						},
					},
					{
						Name:        "edit",
						Usage:       "edit a configuration",
						UsageText:   "phenix config edit <config kind>/<config name>",
						Description: "used to edit a configuration with the default system editor; kind can be topology, experiment, or scenario",
						Action: func(ctx *cli.Context) error {
							c, err := config.Edit(ctx.Args().First())
							if err != nil {
								if config.IsConfigNotModified(err) {
									return cli.Exit("no changes were made to the configuration", 0)
								}

								err := util.HumanizeError(err, "Unable to edit given config")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("%s/%s configuration was updated\n", c.Kind, c.Metadata.Name)

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "delete a configuration",
						UsageText:   "phenix config delete <config kind>/<config name>",
						Description: "used to delete a configuration; kind can be topology, experiment, or scenario",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("no configuration(s) were provided", 1)
							}

							for _, c := range ctx.Args().Slice() {
								if err := config.Delete(c); err != nil {
									err := util.HumanizeError(err, "Unable to delete configuration "+c)
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("%s configuration was deleted\n", c)
							}

							return nil
						},
					},
				},
			},
			{
				Name:        "experiment",
				Usage:       "experiment management",
				UsageText:   "phenix experiment [command options] <command>", // not showing up as expected
				Description: "used to manage experiment(s)",
				Aliases:     []string{"exp"},
				Subcommands: []*cli.Command{
					{
						Name:  "apps",
						Usage: "list available experiment apps",
						Action: func(ctx *cli.Context) error {
							apps := app.List()

							if len(apps) == 0 {
								fmt.Printf("\nApps: none\n\n")
							}

							fmt.Printf("\nApps: %s\n\n", strings.Join(apps, ", "))
							return nil
						},
					},
					{
						Name:  "schedulers",
						Usage: "list available experiment schedulers",
						Action: func(ctx *cli.Context) error {
							schedulers := scheduler.List()

							if len(schedulers) == 0 {
								fmt.Printf("\nSchedulers: none\n\n")
							}

							fmt.Printf("\nSchedulers: %s\n\n", strings.Join(schedulers, ", "))
							return nil
						},
					},
					{
						Name:        "list",
						Usage:       "table of experiments",
						UsageText:   "phenix experiment list",
						Description: "used to display a table of available experiments",
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
						Usage:       "create an experiment",
						UsageText:   "phenix experiment create [command options]",
						Description: "used to create an experiment from an existing configuration; can be a topology, or topology and scenario",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "topology",
								Aliases: []string{"t"},
								Usage:   "name of an existing topology to use",
							},
							&cli.StringFlag{
								Name:    "scenario",
								Aliases: []string{"s"},
								Usage:   "name of an existing scenario to use (optional)",
							},
							&cli.StringFlag{
								Name:    "base-dir",
								Aliases: []string{"d"},
								Usage:   "base directory to use for experiment",
							},
						},
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("must provide an experiment name", 1)
							}

							var (
								topology = ctx.String("topology")
								scenario = ctx.String("scenario")
								baseDir  = ctx.String("base-dir")
							)

							if topology == "" {
								return cli.Exit("must provide a topology name", 1)
							}

							if err := experiment.Create(name, topology, scenario, baseDir); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "delete an experiment",
						UsageText:   "phenix experiment delete <experiment name>",
						Description: "used to delete an exisitng experiment; experiment must be stopped",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("must provide an experiment name", 1)
							}

							exp, err := experiment.Get(ctx.Args().First())
							if err != nil {
								err := util.HumanizeError(err, "Unable to get experiment " + name)
								return cli.Exit(err.Humanize(), 1)
							}

							if exp.Status.Running() {
								return cli.Exit("cannot delete a running experiment", 1)
							}

							if err := config.Delete("experiment/" + name); err != nil {
								err := util.HumanizeError(err, "Unable to delete experiment " + name)
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("experiment %s was deleted\n", name)

							return nil
						},
					},
					{
						Name:      "schedule",
						Usage:     "schedule an experiment",
						ArgsUsage: "<exp> <algorithm>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide all arguments", 1)
							}

							var (
								exp  = ctx.Args().Get(0)
								algo = ctx.Args().Get(1)
							)

							if err := experiment.Schedule(exp, algo); err != nil {
								err := util.HumanizeError(err, "Unable to schedule experiment " + exp)
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "start",
						Usage:       "start an experiment",
						UsageText:   "phenix experiment start <experiment name>",
						Description: "used to start a stopped experiment",
						ArgsUsage:   "[flags] <exp>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "do everything but actually call out to minimega",
							},
						},
						Action: func(ctx *cli.Context) error {
							var (
								exp    = ctx.Args().First()
								dryrun = ctx.Bool("dry-run")
							)

							if err := experiment.Start(exp, dryrun); err != nil {
								err := util.HumanizeError(err, "Unable to start experiment " + exp)
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "stop",
						Usage:       "stop an experiment",
						UsageText:   "phenix experiment stop <experiment name>",
						Description: "used to stop a running experiment",
						ArgsUsage:   "[flags] <exp>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "do everything but actually call out to minimega",
							},
						},
						Action: func(ctx *cli.Context) error {
							var (
								exp    = ctx.Args().First()
								dryrun = ctx.Bool("dry-run")
							)

							if err := experiment.Stop(exp, dryrun); err != nil {
								err := util.HumanizeError(err, "Unable to stop experiment " + exp)
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "restart",
						Usage:       "restart an experiment",
						UsageText:   "phenix experiment restart <experiment name>",
						Description: "used to restart a running experiment",
						ArgsUsage:   "[flags] <exp>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "do everything but actually call out to minimega",
							},
						},
						Action: func(ctx *cli.Context) error {
							var (
								exp    = ctx.Args().First()
								dryrun = ctx.Bool("dry-run")
							)

							if err := experiment.Stop(exp, dryrun); err != nil {
								err := util.HumanizeError(err, "Unable to stop the experiment " + exp)
								return cli.Exit(err.Humanize(), 1)
							}

							if err := experiment.Start(exp, dryrun); err != nil {
								err := util.HumanizeError(err, "Unable to start the experiment " + exp)
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:        "vm",
				Usage:       "virtual machine management",
				UsageText:   "phenix vm [command options] <command>", // not showing up as expected
				Description: "used to manage virtual machine(s)",
				Subcommands: []*cli.Command{
					{
						Name:        "info",
						Usage:       "table of virtual machine(s)",
						UsageText:   "phenix vm info <experiment name>/<vm name>",
						Description: "used to display a table of virtual machine(s) for a specific experiment; virtual machine name is optional, when included will display only that vm",
						Action: func(ctx *cli.Context) error {
							// Should look like `exp` or `exp/vm`
							parts := strings.Split(ctx.Args().First(), "/")

							switch len(parts) {
							case 1:
								vms, err := vm.List(parts[0])
								if err != nil {
									err := util.HumanizeError(err, "Unable to get list of VMs")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVMs(os.Stdout, vms...)
							case 2:
								vm, err := vm.Get(parts[0], parts[1])
								if err != nil {
									err := util.HumanizeError(err, "Unable to get information for the " + parts[1] + " VM")
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVMs(os.Stdout, *vm)
							default:
								return cli.Exit("invalid argument", 1)
							}

							return nil
						},
					},
					{
						Name:        "pause",
						Usage:       "pause a running virtual machine",
						UsageText:   "phenix vm pause <experiment name> <vm name>",
						Description: "used to pause a running virtual machine for a speific experiment",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide an experiment and virtual machine name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Pause(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to pause the " + vmName + " VM")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "resume",
						Usage:       "resume a paused virtual machine",
						UsageText:   "phenix vm resume <experiment name> <vm name>",
						Description: "used to resume a paused virtul machine for a specific experiment",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide an experiment and virtual machine name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Resume(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to resume the " + vmName + " VM")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "redeploy",
						Usage:       "redeploy a running virtual machine",
						UsageText:   "phenix vm redeploy [command options] <experiment name> <vm name>",
						Description: "used to redeploy a running virtual machine for a specific experiment; several values can be modified",
						ArgsUsage:   "<exp> <vm>",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "cpu",
								Aliases: []string{"c"},
								Usage:   "number of vm cpus (1-8 is valid)",
							},
							&cli.IntFlag{
								Name:    "mem",
								Aliases: []string{"m"},
								Usage:   "amount of memory in megabytes (512, 1024, 2048, 3072, 4096, 8192, 12288, 16384 are valid)",
							},
							&cli.StringFlag{
								Name:    "disk",
								Aliases: []string{"d"},
								Usage:   "vm backing disk image",
							},
							&cli.BoolFlag{
								Name:    "replicate-injects",
								Aliases: []string{"r"},
								Usage:   "recreate disk snapshot and vm injections",
							},
							&cli.IntFlag{
								Name:    "partition",
								Aliases: []string{"p"},
								Usage:   "partition of disk to inject files into (only used if disk option is specified)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide all arguments", 1)
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
								return cli.Exit("cpus can only be 1-8", 1)
							}

							if mem != 0 && (mem < 512 || mem > 16384 || mem%512 != 0) {
								return cli.Exit("memory must be one of 512, 1024, 2048, 3072, 4096, 8192, 12288, 16384", 1)
							}

							opts := []vm.RedeployOption{
								vm.CPU(cpu),
								vm.Memory(mem),
								vm.Disk(disk),
								vm.Inject(inject),
								vm.InjectPartition(part),
							}

							if err := vm.Redeploy(expName, vmName, opts...); err != nil {
								err := util.HumanizeError(err, "Unable to redeploy the " + vmName + " VM")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "kill",
						Usage:       "kill a running or paused virtual machine",
						UsageText:   "phenix vm kill <experiment name> <vm name>",
						Description: "used to kill or delete a running or paused virtual machine for a specific experiment",
						ArgsUsage:   "<exp> <vm>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide all arguments", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Kill(expName, vmName); err != nil {
								err := util.HumanizeError(err, "Unable to kill the " + vmName + " VM")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "set",
						Usage:       "set configuration value for a virtual machine",
						UsageText:   "phenix vm set {{ TO DO }}",
						Description: "used to set a configuration value for a virtual machine in a stopped experiment",
						Action: func(ctx *cli.Context) error {
							return cli.Exit("This command is not yet implemented. For now, you can edit the experiment directly.", 1)
						},
					},
					{
						Name:        "net",
						Usage:       "modify network connectivity for a virtual machine",
						UsageText:   "phenix vm net {{ TO DO }}", // not showing up as expected
						Description: "used to modify the network connectivity for a virtual machine in a running experiment",
						Subcommands: []*cli.Command{
							{
								Name:      "connect",
								Usage:     "connect a vm interface to a vlan",
								UsageText: "phenix vm net connect <experiment name> <vm name> <iface index> <vlan name>", // vlan name or identifier?
								ArgsUsage: "<exp> <vm> <iface index> <vlan>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 4 {
										return cli.Exit("must provide all arguments", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
										vlan    = ctx.Args().Get(3)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("the network interface index must be an integer", 1)
									}

									if err := vm.Connect(expName, vmName, iface, vlan); err != nil {
										err := util.HumanizeError(err, "Unable to modify the connectivity for the " + vmName + " VM")
										return cli.Exit(err.Humanize(), 1)
									}

									return nil
								},
							},
							{
								Name:      "disconnect",
								Usage:     "disconnect a vm interface",
								UsageText: "phenix vm net disconnect <experiment name> <vm name> <iface index>",
								ArgsUsage: "<exp> <vm> <iface index>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 3 {
										return cli.Exit("must provide all arguments", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("the network interface index must be an integer", 1)
									}

									if err := vm.Disonnect(expName, vmName, iface); err != nil {
										err := util.HumanizeError(err, "Unable to disconnect the interface on the " + vmName + " VM")
										return cli.Exit(err.Humanize(), 1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:        "capture",
						Usage:       "modify network packet captures for a virutal machine",
						UsageText:   "phenix vm capture {{ TO DO }}",
						Description: "used to modify the network packet captures for a virtual machine in a running experiment",
						Subcommands: []*cli.Command{
							{
								Name:      "start",
								Usage:     "start a packet capture",
								UsageText: "phenix vm capture start <experiment name> <vm name> <iface index> </path/to/out file>",
								ArgsUsage: "<exp> <vm> <iface index> <out file>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 4 {
										return cli.Exit("must provide all arguments", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
										out     = ctx.Args().Get(3)
									)

									iface, err := strconv.Atoi(ctx.Args().Get(2))
									if err != nil {
										return cli.Exit("the network interface index must be an integer", 1)
									}

									if err := vm.StartCapture(expName, vmName, iface, out); err != nil {
										err := util.HumanizeError(err, "Unable to start a capture on the interface on the " + vmName + " VM")
										return cli.Exit(err.Humanize(), 1)
									}

									return nil
								},
							},
							{
								Name:      "stop",
								Usage:     "stop packet capture(s)",
								UsageText: "phenix vm capture stop <experiment name> <vm name>",
								ArgsUsage: "<exp> <vm>",
								Action: func(ctx *cli.Context) error {
									if ctx.Args().Len() != 2 {
										return cli.Exit("must provide all arguments", 1)
									}

									var (
										expName = ctx.Args().Get(0)
										vmName  = ctx.Args().Get(1)
									)

									if err := vm.StopCaptures(expName, vmName); err != nil {
										err := util.HumanizeError(err, "Unable to stop the packet captures on the " + vmName + " VM")
										return cli.Exit(err.Humanize(), 1)
									}

									return nil
								},
							},
						},
					},
				},
			},
			{
				Name:  "image",
				Usage: "virtual disk image management",
				Subcommands: []*cli.Command{
					{
						Name:        "create",
						Usage:       "create image configuration",
						UsageText:   "phenix image create [flag(s)] <image name>",
						Description: "used to create a virtual disk image configuration from which to build an image; flags are optional",
						ArgsUsage:   "[flags] <name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "size",
								Aliases: []string{"z"},
								Usage:   "image size to use",
								Value:   "5G",
							},
							&cli.StringFlag{
								Name:    "variant",
								Aliases: []string{"v"},
								Usage:   "image variant to use",
								Value:   "minbase",
							},
							&cli.StringFlag{
								Name:    "release",
								Aliases: []string{"r"},
								Usage:   "os release code name",
								Value:   "bionic",
							},
							&cli.StringFlag{
								Name:    "mirror",
								Aliases: []string{"m"},
								Usage:   "debootstrap mirror (must match release)",
								Value:   "http://us.archive.ubuntu.com/ubuntu/",
							},
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Usage:   "format of disk image",
								Value:   "raw",
							},
							&cli.BoolFlag{
								Name:    "compress",
								Aliases: []string{"c"},
								Usage:   "compress image after creation (does not apply to raw image)",
							},
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "list of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "list of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "list of scripts to include in addition to the default one (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "debootstrap-append",
								Aliases: []string{"d"},
								Usage:   "additional arguments to debootstrap (default: --components=main,restricted,universe,multiverse)",
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
								err := util.HumanizeError(err, "Unable to create the " + name + " image" )
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:      "create-from",
						Usage:     "create image configuration from existing one",
						UsageText: "phenix image create-from [flag(s)] <existing name> <new name>",
						Description: "used to create a new virtual disk image configuration from an existing one; flags are optional, if used they will be added to the exisiting configuration",
						ArgsUsage: "[flags] <name> <saveas>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "list of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "list of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "list of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("name of existing config is required", 1)
							}

							if ctx.Args().Get(1) == "" {
								return cli.Exit("name for new config is required", 1)
							}

							var (
								name     = ctx.Args().First()
								saveas   = ctx.Args().Get(1)
								overlays = strings.Split(ctx.String("overlays"), ",")
								packages = strings.Split(ctx.String("packages"), ",")
								scripts  = strings.Split(ctx.String("scripts"), ",")
							)

							if err := image.CreateFromConfig(name, saveas, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to create the configuration file " + saveas)
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "build",
						Usage:       "build an image",
						UsageText:   "phenix image build [flag(s)] <configuration name>",
						Description: "used to build a new virtual disk using an exisitng configuration",
						ArgsUsage:   "[flags] <name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "verbosity",
								Aliases: []string{"v"},
								Usage:   "enable verbose output from debootstrap (options are v, vv, vvv)",
							},
							&cli.BoolFlag{
								Name:    "cache",
								Aliases: []string{"c"},
								Usage:   "cache rootfs as tar archive",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("name of configuration to build the disk image is required", 1)
							}

							var (
								name      = ctx.Args().First()
								verbosity = ctx.String("verbosity")
								cache     = ctx.Bool("cache")
							)

							if err := image.Build(name, verbosity, cache); err != nil {
								err := util.HumanizeError(err, "Unable to build the " + name + " image")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:        "list",
						Usage:       "table of image configuration(s)",
						UsageText:   "phenix image list",
						Description: "used to display a table of exisitng virtual disk image configuration(s); includes size, vairant, release, overlays, and packages",
						Action: func(ctx *cli.Context) error {
							imgs, err := image.List()
							if err != nil {
								err := util.HumanizeError(err, "Unable to print a list of configurations")
								return cli.Exit(err.Humanize(), 1)
							}

							util.PrintTableOfImageConfigs(os.Stdout, imgs...)

							return nil
						},
					},
					{
						Name:        "delete",
						Usage:       "delete image configuration",
						UsageText:   "phenix image delete <image name>",
						Description: "used to delete an existing virtual disk image configuration by name",
						ArgsUsage:   "<name>",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("name of config to delete is required", 1)
							}

							if err := config.Delete("image/" + name); err != nil {
								err := util.HumanizeError(err, "Unable to delete the " + name + " image")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("%s image configuration deleted\n", name)

							return nil
						},
					},
					{
						Name:      "append",
						Usage:     "append to an image configuration",
						UsageText: "phenix image append [flag(s)] <image name>",
						Description: "used to add scripts, packages, and/or overlays to an existing virtual disk image configuration",
						ArgsUsage: "[flags] <name>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "list of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "list of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "list of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("name of config file to append to is required", 1)
							}

							var (
								name     = ctx.Args().First()
								overlays = strings.Split(ctx.String("overlays"), ",")
								packages = strings.Split(ctx.String("packages"), ",")
								scripts  = strings.Split(ctx.String("scripts"), ",")
							)

							if err := image.Append(name, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to append to the " + name + " image")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:      "remove",
						Usage:     "remove from an image configuration",
						UsageText: "phenix image remove [flag(s)] <image name>",
						Description: "used to remove scripts, packages, and/or overlays to an existing virtual disk image configuration",
						ArgsUsage: "[flags] <name>>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "overlays",
								Aliases: []string{"o"},
								Usage:   "list of overlay names (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "packages",
								Aliases: []string{"p"},
								Usage:   "list of packages to include in addition to those provided by variant (separated by comma)",
							},
							&cli.StringFlag{
								Name:    "scripts",
								Aliases: []string{"s"},
								Usage:   "list of scripts to include in addition to the default one (separated by comma)",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().First() == "" {
								return cli.Exit("name of config file to remove from is required", 1)
							}

							var (
								name     = ctx.Args().First()
								overlays = strings.Split(ctx.String("overlays"), ",")
								packages = strings.Split(ctx.String("packages"), ",")
								scripts  = strings.Split(ctx.String("scripts"), ",")
							)

							if err := image.Remove(name, overlays, packages, scripts); err != nil {
								err := util.HumanizeError(err, "Unable to remove from the " + name + " image")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:  "vlan",
				Usage: "vlan management",
				Subcommands: []*cli.Command{
					{
						Name:        "alias",
						Usage:       "view or set a vlan alias",
						UsageText:   "phenix vlan alias [flag(s)] <experiment name> <alias name> <vlan name>", // is it vlan name or index or ??
						Description: "used to view or set an alias for a given vlan",
						ArgsUsage:   "[flags] [experiment] [alias] [vlan]",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "force update on set action if alias already exists",
							},
						},
						Action: func(ctx *cli.Context) error {
							switch ctx.NArg() {
							case 0:
								info, err := vlan.Aliases()
								if err != nil {
									err := util.HumanizeError(err, "Unable to display all aliases") // not sure this is accurate
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANAliases(os.Stdout, info)
							case 1:
								info, err := vlan.Aliases(vlan.Experiment(ctx.Args().First()))
								if err != nil {
									err := util.HumanizeError(err, "Unable to display aliases for the experiment") // not sure this is accurate; if it is, should represent exp name
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
									return cli.Exit("the vlan identifier provided is not a valid integer", 1)
								}

								if err := vlan.SetAlias(vlan.Experiment(exp), vlan.Alias(alias), vlan.ID(vid), vlan.Force(force)); err != nil {
									err := util.HumanizeError(err, "Unable to set the alias for the " + exp + " experiment")
									return cli.Exit(err.Humanize(), 1)
								}
							default:
								return cli.Exit("unexpected number of arguments provided", 1)
							}

							return nil
						},
					},
					{
						Name:      "range",
						Usage:     "view or set a vlan range",
						UsageText: "phenix vlan range [flag(s)] <experiment name> <range minimum> <range maximum>",
						Description: "used to view or set a range for a given vlan",
						ArgsUsage: "[flags] [experiment] [min] [max]",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "force update on set action if range is already set",
							},
						},
						Action: func(ctx *cli.Context) error {
							switch ctx.NArg() {
							case 0:
								info, err := vlan.Ranges()
								if err != nil {
									err := util.HumanizeError(err, "Unable to display VLAN range(s)") // not sure this is accurate
									return cli.Exit(err.Humanize(), 1)
								}

								util.PrintTableOfVLANRanges(os.Stdout, info)
							case 1:
								info, err := vlan.Ranges(vlan.Experiment(ctx.Args().First()))
								if err != nil {
									err := util.HumanizeError(err, "Unable to display VLAN range(s) for the experiment") // not sure this is accurate; if it is, should represent exp name
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
									return cli.Exit("the vlan range minimum identifier provided not a valid integer", 1)
								}

								vmax, err := strconv.Atoi(max)
								if err != nil {
									return cli.Exit("the vlan range maximum identifier provided not a valid integer", 1)
								}

								if err := vlan.SetRange(vlan.Experiment(exp), vlan.Min(vmin), vlan.Max(vmax), vlan.Force(force)); err != nil {
									err := util.HumanizeError(err, "Unable to set the VLAN range for the " + exp + " experiment")
									return cli.Exit(err.Humanize(), 1)
								}
							default:
								return cli.Exit("unexpected number of arguments provided", 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:    "util",
				Usage:   "phenix utility commands",
				Subcommands: []*cli.Command{
					{
						Name:  "app-json",
						Usage: "print application JSON input for given experiment to STDOUT",
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
								return cli.Exit("no experiment provided", 1)
							}

							exp, err := experiment.Get(name)
							if err != nil {
								err := util.HumanizeError(err, "Unable to get the " + name + " experiment")
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
