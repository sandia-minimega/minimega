package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
)

func main() {

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file:", os.Args[1], "-- Error:", err)
		return
	}

	var prop Property
	extension := filepath.Ext(os.Args[1])

	if extension == ".json" {
		err = json.Unmarshal(file, &prop)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(prop)
	} else if extension == ".yml" {
		err = yaml.Unmarshal(file, &prop)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(prop)
	} else if extension == ".yaml" {
		err = yaml.Unmarshal(file, &prop)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(prop)
	} else {
		fmt.Println("You need to pass a file with JSON or YAML extension.")
	}

}
