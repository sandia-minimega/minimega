package types

import (
	"fmt"
	"testing"

	v1 "phenix/types/version/v1"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

func TestOpenAPI3(t *testing.T) {
	l := openapi3.NewSwaggerLoader()

	s, err := l.LoadSwaggerFromData(v1.OpenAPI)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	if err := s.Validate(l.Context); err != nil {
		t.Log(err)
		t.FailNow()
	}

	var c Config

	if err := yaml.Unmarshal([]byte(topology), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	schema := s.Components.Schemas["Topology"].Value

	ok := schema.IsMatching(c.Spec)
	fmt.Println(ok)

	if err := schema.VisitJSON(c.Spec); err != nil {
		t.Log(err)
		t.FailNow()
	}
}
