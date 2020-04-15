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
	"phenix/tmpl"
	"phenix/types"
	"phenix/types/version"
	v1 "phenix/types/version/v1"
	"phenix/util"
	"phenix/util/envflag"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

var (
	f_help bool

	f_storePath string

	f_topologyFile   string
	f_scenarioFile   string
	f_experimentName string
)

func init() {
	flag.BoolVar(&f_help, "help", false, "show this help message")
	flag.StringVar(&f_storePath, "store", "phenix.bdb", "path to Bolt store file")
	flag.StringVar(&f_topologyFile, "topology", "", "path to topology config file")
	flag.StringVar(&f_scenarioFile, "scenario", "", "path to scenario config file")
	flag.StringVar(&f_experimentName, "experiment", "", "create experiment with given name if provided")
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

	var (
		topo     *v1.TopologySpec
		scenario *v1.ScenarioSpec
	)

	if f_topologyFile != "" {
		c, err := createConfig(s, f_topologyFile)
		if err != nil {
			fmt.Println("Error creating topology config:", err)
		}

		spec, err := version.GetVersionedSpecForKind(c.Kind, c.APIVersion())
		if err != nil {
			panic(err)
		}

		if err := mapstructure.Decode(c.Spec, spec); err != nil {
			panic(err)
		}

		topo = spec.(*v1.TopologySpec)

		exp := &v1.ExperimentSpec{
			ExperimentName: c.Metadata.Name,
			Topology:       topo,
		}

		exp.SetDefaults()
	}

	if f_scenarioFile != "" {
		c, err := createConfig(s, f_scenarioFile)
		if err != nil {
			fmt.Println("Error creating scenario config:", err)
		}

		spec, err := version.GetVersionedSpecForKind(c.Kind, c.APIVersion())
		if err != nil {
			panic(err)
		}

		if err := mapstructure.Decode(c.Spec, spec); err != nil {
			panic(err)
		}

		scenario = spec.(*v1.ScenarioSpec)
	}

	if f_experimentName != "" {
		exp := &v1.ExperimentSpec{
			ExperimentName: f_experimentName,
			Topology:       topo,
			Scenario:       scenario,
		}

		exp.SetDefaults()

		fmt.Printf("%+v\n", structs.MapDefaultSnakeCase(topo))

		spec := structs.MapDefaultSnakeCase(exp)

		expConfig := types.Config{
			Version: "phenix.sandia.gov/v1",
			Kind:    "Experiment",
			Metadata: types.ConfigMetadata{
				Name: f_experimentName,
			},
			Spec: spec,
		}

		m, _ := yaml.Marshal(expConfig)
		fmt.Println(string(m))

		if err := s.Create(&expConfig); err != nil {
			panic(err)
		}

		if err := tmpl.GenerateFromTemplate("minimega_script.tmpl", exp, os.Stdout); err != nil {
			panic(err)
		}
	}

	configs, err := s.List("Topology", "Scenario", "Experiment")
	if err != nil {
		panic(err)
	}

	fmt.Println()
	util.PrintTableOfConfigs(os.Stdout, configs)
	fmt.Println()
}

func createConfig(s store.Store, path string) (*types.Config, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var config types.Config

	switch filepath.Ext(path) {
	case ".json":
		if err := json.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid config extension")
	}

	if err := types.ValidateConfigSpec(config); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if err := s.Create(&config); err != nil {
		return nil, fmt.Errorf("storing config: %w", err)
	}

	return &config, nil
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
