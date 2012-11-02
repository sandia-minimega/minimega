package vmconfig

import (
	"testing"
	"io/ioutil"
	"os"
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

func TestConfigWorking(t *testing.T) {
	input := `
// comment
v = "v data" //more comments
// another comment
d = "d data"`

	path, err := create_config(input)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	config, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(config) != 2 {
		t.Fatal("too many elements:", config)
	}

	if config["d"] != "d data" || config["v"] != "v data" {
		t.Fatalf("invalid parsing: %#v", config)
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
