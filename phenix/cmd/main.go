package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"phenix/api/config"
	"phenix/api/experiment"
	"phenix/api/vm"
	"phenix/docs"
	"phenix/store"
	"phenix/util"
	"phenix/version"

	assetfs "github.com/elazarl/go-bindata-assetfs"
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
		},
		Before: func(ctx *cli.Context) error {
			if err := store.Init(store.Endpoint(ctx.String("store.endpoint"))); err != nil {
				return cli.Exit(err, 1)
			}

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
								return cli.Exit(err, 1)
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
								return cli.Exit(err, 1)
							}

							switch ctx.String("output") {
							case "yaml":
								m, err := yaml.Marshal(c)
								if err != nil {
									return cli.Exit(fmt.Errorf("marshaling config to YAML: %w", err), 1)
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
									return cli.Exit(fmt.Errorf("marshaling config to JSON: %w", err), 1)
								}

								fmt.Println(string(m))
							default:
								return cli.Exit(fmt.Sprintf("unrecognized output format '%s'\n", ctx.String("output")), 1)
							}

							return nil
						},
					},
					{
						Name:  "create",
						Usage: "create phenix config(s)",
						Action: func(ctx *cli.Context) error {
							if ctx.Args().Len() == 0 {
								return cli.Exit("no config files provided", 1)
							}

							for _, f := range ctx.Args().Slice() {
								c, err := config.Create(f)
								if err != nil {
									return cli.Exit(err, 1)
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
									return cli.Exit("no changes made to config", 0)
								}

								return cli.Exit(err, 1)
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
								return cli.Exit("no config(s) provided", 1)
							}

							for _, c := range ctx.Args().Slice() {
								if err := config.Delete(c); err != nil {
									return cli.Exit(err, 1)
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
						Name:  "start",
						Usage: "start an experiment",
						Action: func(ctx *cli.Context) error {
							if err := experiment.Start(ctx.Args().First()); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:  "stop",
						Usage: "stop an experiment",
						Action: func(ctx *cli.Context) error {
							if err := experiment.Stop(ctx.Args().First()); err != nil {
								return cli.Exit(err, 1)
							}

							return nil
						},
					},
					{
						Name:  "restart",
						Usage: "restart an experiment",
						Action: func(ctx *cli.Context) error {
							if err := experiment.Stop(ctx.Args().First()); err != nil {
								return cli.Exit(err, 1)
							}

							if err := experiment.Start(ctx.Args().First()); err != nil {
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
				Name:  "docs",
				Usage: "serve documenation over HTTP",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "endpoint",
						Aliases: []string{"e"},
						Usage:   "endpoint to bind HTTP server to",
						Value:   ":8080",
					},
				},
				Action: func(ctx *cli.Context) error {
					endpoint := ctx.String("endpoint")

					fs := &assetfs.AssetFS{
						Asset:     docs.Asset,
						AssetDir:  docs.AssetDir,
						AssetInfo: docs.AssetInfo,
					}

					http.Handle("/", http.FileServer(fs))

					fmt.Printf("\nStarting documentation server at %s\n", endpoint)

					http.ListenAndServe(endpoint, nil)

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
	}
}
