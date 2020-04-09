package types

import (
	"testing"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

var config = `
version: v0
kind: Experiment
metadata:
  name: foobar
spec:
  schedules:
  - hostname: suckafish
    clusterNode: compute1
`

func TestConfig(t *testing.T) {
	var c Config

	if err := yaml.Unmarshal([]byte(config), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	t.Logf("%+v", c)

	switch c.Kind {
	case "Experiment":
		var e ExperimentSpec

		if err := mapstructure.Decode(c.Spec, &e); err != nil {
			t.Log(err)
			t.FailNow()
		}

		t.Logf("%+v", e)
	default:
		t.Log("unknown config kind")
		t.FailNow()
	}
}
