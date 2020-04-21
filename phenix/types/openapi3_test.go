package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPI3(t *testing.T) {
	var c Config

	if err := yaml.Unmarshal([]byte(topology), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	if err := ValidateConfigSpec(c); err != nil {
		t.Log(err)
		t.FailNow()
	}
}
