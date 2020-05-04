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
	"phenix/util/envflag"

	assetfs "github.com/elazarl/go-bindata-assetfs"
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
	envflag.Parse("PHENIX")
	flag.Parse()

	if f_help {
		usage()
		return
	}

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	if err := store.Init(store.Path(f_storePath)); err != nil {
		fmt.Println("error initializing config store:", err)
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "list":
		configs, err := config.ListConfigs(flag.Arg(1))

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println()

		if len(configs) == 0 {
			fmt.Println("no configs currently exist")
		} else {
			util.PrintTableOfConfigs(os.Stdout, configs)
		}

		fmt.Println()
	case "get":
		var (
			flags  = flag.NewFlagSet("get", flag.ExitOnError)
			format = flags.String("o", "yaml", "output format (yaml or json)")
		)

		if err := flags.Parse(flag.Args()[1:]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		c, err := config.GetConfig(flags.Arg(0))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		switch *format {
		case "yaml":
			m, err := yaml.Marshal(c)
			if err != nil {
				fmt.Println(fmt.Errorf("marshaling config to YAML: %w", err))
				os.Exit(1)
			}

			fmt.Println(string(m))
		case "json":
			m, err := json.Marshal(c)
			if err != nil {
				fmt.Println(fmt.Errorf("marshaling config to JSON: %w", err))
				os.Exit(1)
			}

			fmt.Println(string(m))
		default:
			fmt.Printf("unrecognized output format '%s'\n", *format)
			os.Exit(1)
		}
	case "create":
		if flag.NArg() == 1 {
			fmt.Println("no config files provided")
			os.Exit(1)
		}

		for _, f := range flag.Args()[1:] {
			c, err := config.CreateConfig(f)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			fmt.Printf("%s/%s config created\n", c.Kind, c.Metadata.Name)
		}
	case "edit":
		c, err := config.EditConfig(flag.Arg(1))
		if err != nil {
			if config.IsConfigNotModified(err) {
				fmt.Println("no changes made to config")
				os.Exit(0)
			}

			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Printf("%s/%s config updated\n", c.Kind, c.Metadata.Name)
	case "delete":
		if err := config.DeleteConfig(flag.Arg(1)); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Printf("%s deleted\n", flag.Arg(1))
	case "experiment":
		switch flag.Arg(1) {
		case "start":
			if err := experiment.Start(flag.Arg(2)); err != nil {
				panic(err)
			}
		case "stop":
			if err := experiment.Stop(flag.Arg(2)); err != nil {
				panic(err)
			}
		default:
			panic("unknown experiment command")
		}
	case "vm":
		switch flag.Arg(1) {
		case "info":
			// Should look like `expName/vmName`
			parts := strings.Split(flag.Arg(2), "/")

			vm, err := vm.Get(parts[0], parts[1])
			if err != nil {
				panic(err)
			}

			fmt.Println(vm)
		}
	case "docs":
		port := ":8000"

		if flag.NArg() > 1 {
			port = ":" + flag.Arg(1)
		}

		fs := &assetfs.AssetFS{
			Asset:     docs.Asset,
			AssetDir:  docs.AssetDir,
			AssetInfo: docs.AssetInfo,
		}

		http.Handle("/", http.FileServer(fs))

		fmt.Printf("\nStarting documentation server at %s\n", port)

		http.ListenAndServe(port, nil)
	default:
		panic("unknown command")
	}

	os.Exit(0)
}

func usage() {
	fmt.Fprintln(flag.CommandLine.Output(), "minimega phenix")

	fmt.Fprintln(flag.CommandLine.Output(), "")

	fmt.Fprintln(flag.CommandLine.Output(), "Global Options:")
	flag.PrintDefaults()

	fmt.Fprintln(flag.CommandLine.Output(), "")

	fmt.Fprintln(flag.CommandLine.Output(), "Subcommands:")
	fmt.Fprintln(flag.CommandLine.Output(), "  list [all,topology,scenario,experiment] - get a list of configs")
	fmt.Fprintln(flag.CommandLine.Output(), "  get <kind/name>                         - get an existing config")
	fmt.Fprintln(flag.CommandLine.Output(), "  create <path/to/config>                 - create a new config")
	fmt.Fprintln(flag.CommandLine.Output(), "  edit <kind/name>                        - edit an existing config")
	fmt.Fprintln(flag.CommandLine.Output(), "  delete <kind/name>                      - delete a config")
	fmt.Fprintln(flag.CommandLine.Output(), "  experiment <start,stop> <name>          - start an existing experiment")
	fmt.Fprintln(flag.CommandLine.Output(), "  docs <port>                             - start documentation server on port (default 8000)")
	// fmt.Fprintln(flag.CommandLine.Output(), "  help <cmd> - print help message for subcommand")

	fmt.Fprintln(flag.CommandLine.Output(), "")
}
