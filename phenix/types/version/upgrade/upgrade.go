package upgrade

import (
	"strings"

	// TODO: this may cause a cyclic import issue at some point in the future
	// since phenix/types is a direct parent of this package...
	"phenix/types"
)

type Upgrader interface {
	Upgrade(oldVersion string, spec map[string]interface{}, md types.ConfigMetadata) ([]interface{}, error)
}

// Key should be in the form of `kind/version` -- ie. topology/v1
var upgraders = make(map[string]Upgrader)

func RegisterUpgrader(v string, u Upgrader) {
	v = strings.ToLower(v)
	upgraders[v] = u
}

func GetUpgrader(v string) Upgrader {
	v = strings.ToLower(v)
	return upgraders[v]
}
