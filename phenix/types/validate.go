package types

import (
	"fmt"

	"phenix/types/version"
)

// ValidateConfigSpec validates the spec in the given config using the
// appropriate `openapi3.Schema` validator. Any validation errors encountered
// are returned.
func ValidateConfigSpec(c Config) error {
	if g := c.APIGroup(); g != API_GROUP {
		return fmt.Errorf("invalid API group %s: expected %s", g, API_GROUP)
	}

	v, err := version.GetVersionedValidatorForKind(c.Kind, c.APIVersion())
	if err != nil {
		return fmt.Errorf("getting validator for config: %w", err)
	}

	if err := v.VisitJSON(c.Spec); err != nil {
		return fmt.Errorf("validating config spec against schema: %w", err)
	}

	return nil
}
