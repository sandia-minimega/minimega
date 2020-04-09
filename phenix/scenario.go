package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
)

type Property struct {
	Nodes []Node `yaml:"nodes"`
	Apps  App    `yaml:"apps"`
}

type Node struct {
	Type    string `yaml:"type"`
	General struct {
		Hostname string `yaml:"hostname"`
	} `yaml:"general"`
	Hardware struct {
		OSType string              `yaml:"os_type"`
		Drive  []map[string]string `yaml:"drives"`
	} `yaml:"hardware"`
	Network struct {
		Interface []map[string]string `yaml:"interfaces"`
	} `yaml:"network"`
}

type App struct {
	Infrastructure []struct {
		Name     string                 `yaml:"name"`
		Metadata map[string]interface{} `yaml:"metadata"`
	} `yaml:"infrstructure"`
	Experiment []struct {
		Name     string                 `yaml:"name"`
		Metadata map[string]interface{} `yaml:"metadata"`
	} `yaml:"experiment"`
	Host []struct {
		Name  string `yaml:"name"`
		Hosts []struct {
			Name     string                 `yaml:"name"`
			Metadata map[string]interface{} `yaml:"metadata"`
		}
	} `yaml:"host"`
}

func main() {

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file:", os.Args[1], "-- Error:", err)
		return
	}

	var prop Property

	err = yaml.Unmarshal(file, &prop)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(prop)
}
