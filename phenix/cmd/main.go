package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
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
				Usage:   "endpoint for store service",
				Value:   "bolt://phenix.bdb",
				EnvVars: []string{"PHENIX_STORE_ENDPOINT"},
			},
			&cli.IntFlag{
				Name:    "log.verbosity",
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
				Name:     "list",
				Aliases:  []string{"l"},
				Category: "config",
				Usage:    "list known phenix configs",
				Action: func(ctx *cli.Context) error {
					configs, err := config.ListConfigs(ctx.Args().First())

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
				Name:     "get",
				Aliases:  []string{"g"},
				Category: "config",
				Usage:    "get phenix config",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "output format (yaml, json)",
						Value:   "yaml",
					},
				},
				Action: func(ctx *cli.Context) error {
					c, err := config.GetConfig(ctx.Args().First())
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
						m, err := json.Marshal(c)
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
				Name:     "create",
				Aliases:  []string{"c"},
				Category: "config",
				Usage:    "create phenix config(s)",
				Action: func(ctx *cli.Context) error {
					if ctx.Args().Len() == 0 {
						return cli.Exit("no config files provided", 1)
					}

					for _, f := range ctx.Args().Slice() {
						c, err := config.CreateConfig(f)
						if err != nil {
							return cli.Exit(err, 1)
						}

						fmt.Printf("%s/%s config created\n", c.Kind, c.Metadata.Name)
					}

					return nil
				},
			},
			{
				Name:     "edit",
				Aliases:  []string{"e"},
				Category: "config",
				Usage:    "edit phenix config",
				Action: func(ctx *cli.Context) error {
					c, err := config.EditConfig(ctx.Args().First())
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
				Name:     "delete",
				Aliases:  []string{"d"},
				Category: "config",
				Usage:    "delete phenix config(s)",
				Action: func(ctx *cli.Context) error {
					if ctx.Args().Len() == 0 {
						return cli.Exit("no config(s) provided", 1)
					}

					for _, c := range ctx.Args().Slice() {
						if err := config.DeleteConfig(c); err != nil {
							return cli.Exit(err, 1)
						}

						fmt.Printf("%s deleted\n", c)
					}

					return nil
				},
			},
			{
				Name:     "experiment",
				Aliases:  []string{"exp"},
				Category: "experiments",
				Usage:    "phenix experiment management",
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

							if err := config.DeleteConfig("experiment/" + name); err != nil {
								return cli.Exit(err, 1)
							}

							fmt.Printf("experiment %s deleted\n", name)

							return nil
						},
					},
				},
			},
			{
				Name:     "vm",
				Category: "virtual machines",
				Usage:    "phenix VM management",
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
				},
			},
			{
				Name:     "docs",
				Category: "documentation",
				Usage:    "serve documenation over HTTP",
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
