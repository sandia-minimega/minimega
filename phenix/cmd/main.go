package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"phenix/store"
	"phenix/store/bolt"
	"phenix/types"
	"phenix/util"
	"phenix/util/envflag"

	"gopkg.in/yaml.v3"
)

var (
	f_help bool

	f_storePath string

	f_topologyFile string
	f_scenarioFile string
)

func init() {
	flag.BoolVar(&f_help, "help", false, "show this help message")
	flag.StringVar(&f_storePath, "store", "phenix.bdb", "path to Bolt store file")
	flag.StringVar(&f_topologyFile, "topology", "", "path to topology config file")
	flag.StringVar(&f_scenarioFile, "scenario", "", "path to scenario config file")
}

func main() {
	envflag.Parse("PHENIX")
	flag.Parse()

	if f_help {
		usage()
		return
	}

	/*
		if flag.NArg() < 1 {
			usage()
			os.Exit(1)
		}
	*/

	s := bolt.NewBoltDB()

	if err := s.Init(store.Path(f_storePath)); err != nil {
		fmt.Println("error initializing Bolt store:", err)
		os.Exit(1)
	}

	if f_topologyFile != "" {
		if err := createConfig(s, f_topologyFile); err != nil {
			fmt.Println("Error creating topology config:", err)
		}
	}

	if f_scenarioFile != "" {
		if err := createConfig(s, f_scenarioFile); err != nil {
			fmt.Println("Error creating scenario config:", err)
		}
	}

	configs, err := s.List("Topology", "Scenario")
	if err != nil {
		panic(err)
	}

	fmt.Println()
	util.PrintTableOfConfigs(os.Stdout, configs)
	fmt.Println()

	/*
		spec, err := version.GetVersionedSpecForKind(config.Kind, config.APIVersion())
		if err != nil {
			panic(err)
		}

		if err := mapstructure.Decode(config.Spec, spec); err != nil {
			panic(err)
		}

		fmt.Printf("%+v\n", spec)
	*/
}

func createConfig(s store.Store, path string) error {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read config: %w", err)
	}

	var config types.Config

	switch filepath.Ext(path) {
	case ".json":
		if err := json.Unmarshal(file, &config); err != nil {
			return fmt.Errorf("unmarshaling config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(file, &config); err != nil {
			return fmt.Errorf("unmarshaling config: %w", err)
		}
	default:
		return fmt.Errorf("invalid config extension")
	}

	if err := types.ValidateConfigSpec(config); err != nil {
		return fmt.Errorf("validating config: %w", err)
	}

	if err := s.Create(&config); err != nil {
		return fmt.Errorf("storing config: %w", err)
	}

	return nil
}

func usage() {
	fmt.Fprintln(flag.CommandLine.Output(), "minimega phenix")

	fmt.Fprintln(flag.CommandLine.Output(), "")

	fmt.Fprintln(flag.CommandLine.Output(), "Global Options:")
	flag.PrintDefaults()

	fmt.Fprintln(flag.CommandLine.Output(), "")

	/*
		fmt.Fprintln(flag.CommandLine.Output(), "Subcommands:")
		fmt.Fprintln(flag.CommandLine.Output(), "  experiment")
		fmt.Fprintln(flag.CommandLine.Output(), "  vm")
		fmt.Fprintln(flag.CommandLine.Output(), "  vlan")
		fmt.Fprintln(flag.CommandLine.Output(), "  image")
		fmt.Fprintln(flag.CommandLine.Output(), "  help <cmd> - print help message for subcommand")

		fmt.Fprintln(flag.CommandLine.Output(), "")
	*/
}
