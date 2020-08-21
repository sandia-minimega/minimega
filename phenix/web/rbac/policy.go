package rbac

import (
	"path/filepath"
	"strings"

	v1 "phenix/types/version/v1"
)

type Policy struct {
	Spec *v1.PolicySpec
}

func (this Policy) resourceNameAllowed(name string) bool {
	var allowed bool

	for _, n := range this.Spec.ResourceNames {
		negate := strings.HasPrefix(n, "!")
		n = strings.Replace(n, "!", "", 1)

		if matched, _ := filepath.Match(n, name); matched {
			if negate {
				return false
			}

			allowed = true
		}
	}

	return allowed
}

func (this Policy) verbAllowed(verb string) bool {
	for _, v := range this.Spec.Verbs {
		if v == "*" || v == verb {
			return true
		}
	}

	return false
}
