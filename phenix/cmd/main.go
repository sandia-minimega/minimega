package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"phenix"

	"gopkg.in/yaml.v3"
)

func main() {

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file:", os.Args[1], "-- Error:", err)
		return
	}

	var prop phenix.Property

	if filepath.Ext(os.Args[1]) == ".json" {
		err = json.Unmarshal(file, &prop)
		if err != nil {
			fmt.Println(err)
		}
	} else if filepath.Ext(os.Args[1]) == ".yml" || filepath.Ext(os.Args[1]) == ".yaml" {
		err = yaml.Unmarshal(file, &prop)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Println("You need to pass a file with an appropriate JSON or YAML extension.")
		return
	}

	fmt.Println(prop)
	
}
