package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"phenix/types"
	"phenix/types/version"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("must provide path to config file")
		os.Exit(1)
	}

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file: ", os.Args[1], " -- Error: ", err)
		os.Exit(1)
	}

	var config types.Config

	switch filepath.Ext(os.Args[1]) {
	case ".json":
		if err := json.Unmarshal(file, &config); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(file, &config); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	default:
		fmt.Println("You need to pass a file with an appropriate JSON or YAML extension.")
		os.Exit(1)
	}

	if err := types.ValidateConfigSpec(config); err != nil {
		panic(err)
	}

	spec, err := version.GetVersionedSpecForKind(config.Kind, config.APIVersion())
	if err != nil {
		panic(err)
	}

	if err := mapstructure.Decode(config.Spec, spec); err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", spec)
}
