package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"phenix/api/experiment"
	"phenix/store"
	"phenix/types"
	"phenix/util"
	"phenix/util/editor"
	"phenix/util/envflag"

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
		fmt.Println("error initializing Bolt store:", err)
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "list":
		var (
			configs types.Configs
			err     error
		)

		switch flag.Arg(1) {
		case "", "all":
			configs, err = store.List("Topology", "Scenario", "Experiment")
		case "topology":
			configs, err = store.List("Topology")
		case "scenario":
			configs, err = store.List("Scenario")
		case "experiment":
			configs, err = store.List("Experiment")
		default:
			err = fmt.Errorf("unknown config kind provided")
		}

		if err != nil {
			panic(err)
		}

		fmt.Println()
		util.PrintTableOfConfigs(os.Stdout, configs)
		fmt.Println()
	case "get":
		n := strings.Split(flag.Arg(1), "/")

		if len(n) != 2 {
			panic("invalid config kind/name provided")
		}

		c := types.NewConfig(strings.Title(n[0]), n[1])

		if err := store.Get(c); err != nil {
			panic(err)
		}

		m, err := yaml.Marshal(c)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(m))
	case "create":
		if flag.Arg(1) == "" {
			panic("no config file provided")
		}

		c, err := types.NewConfigFromFile(flag.Arg(1))
		if err != nil {
			panic(err)
		}

		if c.Kind == "Experiment" && c.Spec == nil {
			if err := experiment.CreateConfigSpec(c); err != nil {
				panic(err)
			}
		}

		if err := types.ValidateConfigSpec(*c); err != nil {
			panic(fmt.Errorf("validating config: %w", err))
		}

		if err := store.Create(c); err != nil {
			panic(fmt.Errorf("storing config: %w", err))
		}

		fmt.Printf("%s/%s config created\n", c.Kind, c.Metadata.Name)
	case "edit":
		n := strings.Split(flag.Arg(1), "/")

		if len(n) != 2 {
			panic("invalid config kind/name provided")
		}

		c := types.NewConfig(strings.Title(n[0]), n[1])

		if err := store.Get(c); err != nil {
			panic(err)
		}

		body, err := yaml.Marshal(c.Spec)
		if err != nil {
			panic(err)
		}

		body, err = editor.EditData(body)
		if err != nil {
			if err == editor.ErrNoChange {
				fmt.Printf("no changes made to %s/%s\n", c.Kind, c.Metadata.Name)
				os.Exit(0)
			}

			panic(err)
		}

		var spec map[string]interface{}

		if err := yaml.Unmarshal(body, &spec); err != nil {
			panic(err)
		}

		c.Spec = spec

		if err := store.Update(c); err != nil {
			panic(err)
		}

		fmt.Printf("%s/%s config updated\n", c.Kind, c.Metadata.Name)
	case "delete":
		n := strings.Split(flag.Arg(1), "/")

		if len(n) != 2 {
			panic("invalid config kind/name provided")
		}

		if err := store.Delete(strings.Title(n[0]), n[1]); err != nil {
			panic(err)
		}

		fmt.Printf("%s deleted\n", flag.Arg(1))
	case "experiment":
		switch flag.Arg(1) {
		case "start":
			if err := experiment.Start(flag.Arg(2)); err != nil {
				panic(err)
			}
		default:
			panic("unknown experiment command")
		}
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
	fmt.Fprintln(flag.CommandLine.Output(), "  list [all, topology, scenario, experiment] - get a list of configs")
	fmt.Fprintln(flag.CommandLine.Output(), "  get <kind/name>                            - get an existing config")
	fmt.Fprintln(flag.CommandLine.Output(), "  create <path/to/config>                    - create a new config")
	fmt.Fprintln(flag.CommandLine.Output(), "  edit <kind/name>                           - edit an existing config")
	fmt.Fprintln(flag.CommandLine.Output(), "  delete <kind/name>                         - delete a config")
	fmt.Fprintln(flag.CommandLine.Output(), "  experiment <start> <name>                  - start an existing experiment")
	// fmt.Fprintln(flag.CommandLine.Output(), "  help <cmd> - print help message for subcommand")

	fmt.Fprintln(flag.CommandLine.Output(), "")
}
