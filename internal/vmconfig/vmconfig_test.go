// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vmconfig

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func create_config(input string) (string, error) {
	f, err := ioutil.TempFile("", "vmconfig_test_")
	if err != nil {
		return "", err
	}

	f.WriteString(input)

	n := f.Name()
	f.Close()
	return n, nil
}

func write_config(path, input string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	f.WriteString(input)
	f.Close()
	return nil
}

func TestSimilarPath(t *testing.T) {
	parent, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(parent)

	child, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(child)

	parent_input := `
packages = "parent_test"
`
	child_input := `
parents = "` + filepath.Base(parent) + `"`

	err = write_config(parent, parent_input)
	if err != nil {
		t.Fatal(err)
	}
	err = write_config(child, child_input)
	if err != nil {
		t.Fatal(err)
	}

	config, err := ReadConfig(child)
	if err != nil {
		t.Fatal(err)
	}

	expected := Config{
		Path:     child,
		Parents:  []string{filepath.Base(parent)},
		Packages: []string{"parent_test"},
	}

	if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", config) {
		t.Fatalf("invalid config: %#v\nexpected: %#v", config, expected)
	}
}

func TestPostBuild(t *testing.T) {
	input := "postbuild = `\ntest testing\ntest2`"

	path, err := create_config(input)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	config, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := Config{
		Path: path,
		Postbuilds: []string{`
test testing
test2`},
	}

	if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", config) {
		t.Fatalf("invalid config: %#v\nexpected: %#v", config, expected)
	}
}

func TestConfigNoParents(t *testing.T) {
	input := `
// comment
parents = "" //more comments
// another comment
packages = "linux-headers openvswitch-switch
overlay = "/home/foo/bar"`

	path, err := create_config(input)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	config, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if config.Path != path {
		t.Fatal("path not set")
	}
	if config.Parents != nil {
		t.Fatal("too many parents")
	}
	if fmt.Sprintf("%v", config.Packages) != fmt.Sprintf("%v", []string{"linux-headers", "openvswitch-switch"}) {
		t.Fatal("invalid packages")
	}
	if fmt.Sprintf("%v", config.Overlays) != fmt.Sprintf("%v", []string{"/home/foo/bar"}) {
		t.Fatal("invalid overlay")
	}
}

func TestConfigNotWorking(t *testing.T) {
	input := `
// comment
v = v data //more comments
// another comment
d = "d data"
boo = 35`

	path, err := create_config(input)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	config, err := ReadConfig(path)
	if err == nil {
		t.Fatal(config)
	}
}

func TestConfigRecursive(t *testing.T) {
	path1, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path1)

	path2, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path2)

	path3, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path3)

	path4, err := create_config("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path4)

	input1 := `
// comment
parents = "` + fmt.Sprintf("%v %v", path2, path4) + `" //more comments
// another comment
packages = "linux-headers openvswitch-switch
overlay = "/home/foo/bar"`

	input2 := `
parents = "` + path3 + `"
packages = "path2_package1 path2_package2"
overlay = ""`

	input3 := `
packages = "path3_package1"
overlay = "/path3"`

	input4 := `
parents = ""
packages = "path4_package1 path4_package2"
overlay = "/path4"

`

	write_config(path1, input1)
	write_config(path2, input2)
	write_config(path3, input3)
	write_config(path4, input4)

	config, err := ReadConfig(path1)
	if err != nil {
		t.Fatal(err)
	}

	expected := Config{
		Path:     path1,
		Parents:  []string{path3, path2, path4},
		Packages: []string{"path3_package1", "path2_package1", "path2_package2", "path4_package1", "path4_package2", "linux-headers", "openvswitch-switch"},
		Overlays: []string{"/path3", "/path4", "/home/foo/bar"},
	}

	if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", config) {
		t.Fatalf("invalid config: %#v\nexpected: %#v", config, expected)
	}
}
