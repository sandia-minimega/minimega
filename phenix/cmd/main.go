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
				Name:    "config",
				Aliases: []string{"cfg"},
				Usage:   "phenix config management",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "list known phenix configs",
						Action: func(ctx *cli.Context) error {
							configs, err := config.List(ctx.Args().First())

							if err != nil {
								err := util.HumanizeError(err, "Unable to list known configs")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Println()

							if len(configs) == 0 {
								fmt.Println("no configs currently exist")
							} else {
								util.PrintTableOfConfigs(os.Stdout, configs)
							}

							fmt.Println()

							return nil
						},
					},
					{
						Name:  "get",
						Usage: "get phenix config",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Usage:   "output format (yaml, json)",
								Value:   "yaml",
							},
							&cli.BoolFlag{
								Name:    "pretty",
								Aliases: []string{"p"},
								Usage:   "pretty print JSON output",
							},
						},
						Action: func(ctx *cli.Context) error {
							c, err := config.Get(ctx.Args().First())
							if err != nil {
								err := util.HumanizeError(err, "Unable to get given config")
								return cli.Exit(err.Humanize(), 1)
							}

							switch ctx.String("output") {
							case "yaml":
								m, err := yaml.Marshal(c)
								if err != nil {
									err := util.HumanizeError(err, "Unable to convert config to YAML")
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
									err := util.HumanizeError(err, "Unable to convert config to JSON")
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Println(string(m))
							default:
								err := util.HumanizeError(fmt.Errorf("unrecognized output format %s", ctx.String("output")), "")
								return cli.Exit(err.Humanize(), 1)
							}

							return nil
						},
					},
					{
						Name:  "create",
						Usage: "create phenix config(s)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "skip-validation",
								Usage: "skip config spec validation against schema",
							},
						},
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("No config file(s) provided", 1)
							}

							for _, f := range ctx.Args().Slice() {
								c, err := config.Create(f, !ctx.Bool("skip-validation"))
								if err != nil {
									err := util.HumanizeError(err, "Unable to create config "+f)
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("%s/%s config created\n", c.Kind, c.Metadata.Name)
							}

							return nil
						},
					},
					{
						Name:  "edit",
						Usage: "edit phenix config",
						Action: func(ctx *cli.Context) error {
							c, err := config.Edit(ctx.Args().First())
							if err != nil {
								if config.IsConfigNotModified(err) {
									return cli.Exit("No changes made to config", 0)
								}

								err := util.HumanizeError(err, "Unable to edit given config")
								return cli.Exit(err.Humanize(), 1)
							}

							fmt.Printf("%s/%s config updated\n", c.Kind, c.Metadata.Name)

							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "delete phenix config(s)",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("No config(s) provided", 1)
							}

							for _, c := range ctx.Args().Slice() {
								if err := config.Delete(c); err != nil {
									err := util.HumanizeError(err, "Unable to delete config "+c)
									return cli.Exit(err.Humanize(), 1)
								}

								fmt.Printf("%s deleted\n", c)
							}

							return nil
						},
					},
				},
			},
			{
				Name:    "experiment",
				Aliases: []string{"exp"},
				Usage:   "phenix experiment management",
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
						Name:  "list",
						Usage: "list all experiments",
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
						Name:  "create",
						Usage: "create a new experiment",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "topology",
								Aliases: []string{"t"},
								Usage:   "name of existing topology to use",
							},
							&cli.StringFlag{
								Name:    "scenario",
								Aliases: []string{"s"},
								Usage:   "name of existing scenario to use",
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
								return cli.Exit("must provide experiment name", 1)
							}

							var (
								topology = ctx.String("topology")
								scenario = ctx.String("scenario")
								baseDir  = ctx.String("base-dir")
							)

							if topology == "" {
								return cli.Exit("must provide topology name", 1)
							}

							if err := experiment.Create(name, topology, scenario, baseDir); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "delete an experiment",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("must provide experiment name", 1)
							}

							exp, err := experiment.Get(ctx.Args().First())
							if err != nil {
								return cli.Exit(err, 1)
							}

							if exp.Status.Running() {
								return cli.Exit("cannot delete a running experiment", 1)
							}

							if err := config.Delete("experiment/" + name); err != nil {
								return cli.Exit(err, 1)
							}

							fmt.Printf("experiment %s deleted\n", name)

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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "start",
						Usage:     "start an experiment",
						ArgsUsage: "[flags] <exp>",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "stop",
						Usage:     "stop an experiment",
						ArgsUsage: "[flags] <exp>",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "restart",
						Usage:     "restart an experiment",
						ArgsUsage: "[flags] <exp>",
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
								return cli.Exit(err, 1)
							}

							if err := experiment.Start(exp, dryrun); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:  "vm",
				Usage: "phenix VM management",
				Subcommands: []*cli.Command{
					{
						Name:  "info",
						Usage: "show VM info",
						Action: func(ctx *cli.Context) error {
							// Should look like `exp` or `exp/vm`
							parts := strings.Split(ctx.Args().First(), "/")

							switch len(parts) {
							case 1:
								vms, err := vm.List(parts[0])
								if err != nil {
									return cli.Exit(err, 1)
								}

								util.PrintTableOfVMs(os.Stdout, vms...)
							case 2:
								vm, err := vm.Get(parts[0], parts[1])
								if err != nil {
									return cli.Exit(err, 1)
								}

								util.PrintTableOfVMs(os.Stdout, *vm)
							default:
								return cli.Exit("invalid argument", 1)
							}

							return nil
						},
					},
					{
						Name:  "pause",
						Usage: "pause running VM",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide experiment and VM name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Pause(expName, vmName); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:  "resume",
						Usage: "resume paused VM",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide experiment and VM name", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Resume(expName, vmName); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "redeploy",
						Usage:     "redeploy running VM",
						ArgsUsage: "<exp> <vm>",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "cpu",
								Aliases: []string{"c"},
								Usage:   "number of VM CPUs (1-8 is valid)",
							},
							&cli.IntFlag{
								Name:    "mem",
								Aliases: []string{"m"},
								Usage:   "amount of memory in MB (512, 1024, 2048, 3072, 4096, 8192, 12288, 16384 are valid)",
							},
							&cli.StringFlag{
								Name:    "disk",
								Aliases: []string{"d"},
								Usage:   "VM backing disk image",
							},
							&cli.BoolFlag{
								Name:    "replicate-injects",
								Aliases: []string{"r"},
								Usage:   "recreate disk snapshot and VM injections",
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
								return cli.Exit("CPUs must be 1-8 only", 1)
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "kill",
						Usage:     "kill (delete) running or paused VM",
						ArgsUsage: "<exp> <vm>",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() != 2 {
								return cli.Exit("must provide all arguments", 1)
							}

							var (
								expName = ctx.Args().Get(0)
								vmName  = ctx.Args().Get(1)
							)

							if err := vm.Kill(expName, vmName); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:  "set",
						Usage: "set config value for VM in stopped experiment",
						Action: func(ctx *cli.Context) error {
							return cli.Exit("This command is not yet implemented. For now, you can edit the experiment directly.", 1)
						},
					},
					{
						Name:  "net",
						Usage: "modify network connectivity for running VM",
						Subcommands: []*cli.Command{
							{
								Name:      "connect",
								Usage:     "connect a VM interface to a VLAN",
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
										return cli.Exit("interface index must be an integer", 1)
									}

									if err := vm.Connect(expName, vmName, iface, vlan); err != nil {
										return cli.Exit(err, 1)
									}

									return nil
								},
							},
							{
								Name:      "disconnect",
								Usage:     "disconnect a VM interface",
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
										return cli.Exit("interface index must be an integer", 1)
									}

									if err := vm.Disonnect(expName, vmName, iface); err != nil {
										return cli.Exit(err, 1)
									}

									return nil
								},
							},
						},
					},
					{
						Name:  "capture",
						Usage: "modify network packet captures for running VM",
						Subcommands: []*cli.Command{
							{
								Name:      "start",
								Usage:     "start a packet capture on a VM interface",
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
										return cli.Exit("interface index must be an integer", 1)
									}

									if err := vm.StartCapture(expName, vmName, iface, out); err != nil {
										return cli.Exit(err, 1)
									}

									return nil
								},
							},
							{
								Name:      "stop",
								Usage:     "stop packet captures for a VM",
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
										return cli.Exit(err, 1)
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
				Usage: "phenix Image management",
				Subcommands: []*cli.Command{
					{
						Name:      "create",
						Usage:     "create configuration from which to build an image",
						ArgsUsage: "[flags] <name>",
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
								Usage:   "os release codename",
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
								Usage:   "compress image after creation",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "create-from",
						Usage:     "create a new configuration from an existing one",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "build",
						Usage:     "build an image from a configuration",
						ArgsUsage: "[flags] <name>",
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
								return cli.Exit("name of config to build is required", 1)
							}

							var (
								name      = ctx.Args().First()
								verbosity = ctx.String("verbosity")
								cache     = ctx.Bool("cache")
							)

							if err := image.Build(name, verbosity, cache); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "list",
						Usage:     "prints a list of image build configuration",
						ArgsUsage: "",
						Action: func(ctx *cli.Context) error {
							imgs, err := image.List()
							if err != nil {
								return cli.Exit(err, 1)
							}

							util.PrintTableOfImageConfigs(os.Stdout, imgs...)

							return nil
						},
					},
					{
						Name:      "delete",
						Usage:     "delete image build configuration by name",
						ArgsUsage: "<name>",
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()

							if name == "" {
								return cli.Exit("name of config to delete is required", 1)
							}

							if err := config.Delete("image/" + name); err != nil {
								return cli.Exit(err, 1)
							}

							fmt.Printf("image config %s deleted\n", name)

							return nil
						},
					},
					{
						Name:      "append",
						Usage:     "append scripts, packages, and/or overlays to image build config",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:      "remove",
						Usage:     "remove scripts, packages, and/or overlays from an image build config",
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
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
				},
			},
			{
				Name:  "vlan",
				Usage: "phenix VLAN management",
				Subcommands: []*cli.Command{
					{
						Name:      "alias",
						Usage:     "view or set VLAN alias",
						ArgsUsage: "[flags] [experiment] [alias] [vlan]",
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
									return cli.Exit(err, 1)
								}

								util.PrintTableOfVLANAliases(os.Stdout, info)
							case 1:
								info, err := vlan.Aliases(vlan.Experiment(ctx.Args().First()))
								if err != nil {
									return cli.Exit(err, 1)
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
									return cli.Exit("VLAN ID provided not a valid integer", 1)
								}

								if err := vlan.SetAlias(vlan.Experiment(exp), vlan.Alias(alias), vlan.ID(vid), vlan.Force(force)); err != nil {
									return cli.Exit(err, 1)
								}
							default:
								return cli.Exit("unexpected number of arguments provided", 1)
							}

							return nil
						},
					},
					{
						Name:      "range",
						Usage:     "view or set VLAN range",
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
									return cli.Exit(err, 1)
								}

								util.PrintTableOfVLANRanges(os.Stdout, info)
							case 1:
								info, err := vlan.Ranges(vlan.Experiment(ctx.Args().First()))
								if err != nil {
									return cli.Exit(err, 1)
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
									return cli.Exit("VLAN min ID provided not a valid integer", 1)
								}

								vmax, err := strconv.Atoi(max)
								if err != nil {
									return cli.Exit("VLAN max ID provided not a valid integer", 1)
								}

								if err := vlan.SetRange(vlan.Experiment(exp), vlan.Min(vmin), vlan.Max(vmax), vlan.Force(force)); err != nil {
									return cli.Exit(err, 1)
								}
							default:
								return cli.Exit("unexpected number of arguments provided", 1)
							}

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
