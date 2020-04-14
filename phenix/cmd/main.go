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

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file: ", os.Args[1], " -- Error: ", err)
		return
	}

	var config types.Config

	if filepath.Ext(os.Args[1]) == ".json" {
		err = json.Unmarshal(file, &config)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else if filepath.Ext(os.Args[1]) == ".yml" || filepath.Ext(os.Args[1]) == ".yaml" {
		err = yaml.Unmarshal(file, &config)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		fmt.Println("You need to pass a file with an appropriate JSON or YAML extension.")
		return
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
