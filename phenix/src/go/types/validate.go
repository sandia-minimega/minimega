package types

import (
	"encoding/json"
	"fmt"

	"phenix/store"
	"phenix/types/version"
)

// ValidateConfigSpec validates the spec in the given config using the
// appropriate `openapi3.Schema` validator. Any validation errors encountered
// are returned.
func ValidateConfigSpec(c store.Config) error {
	if g := c.APIGroup(); g != store.API_GROUP {
		return fmt.Errorf("invalid API group %s: expected %s", g, store.API_GROUP)
	}

	v, err := version.GetVersionedValidatorForKind(c.Kind, c.APIVersion())
	if err != nil {
		return fmt.Errorf("getting validator for config: %w", err)
	}

	// FIXME: using JSON marshal/unmarshal to get Go types converted to JSON
	// types. This is mainly needed for Go int types, since JSON only has float64.
	// There's a better way to do this, but it requires an update to the openapi3
	// package we're using.
	data, _ := json.Marshal(c.Spec)
	var spec interface{}
	json.Unmarshal(data, &spec)

	if err := v.VisitJSON(spec); err != nil {
		return fmt.Errorf("validating config spec against schema: %w", err)
	}

	return nil
}
