package upgrade

import "strings"

type Upgrader interface {
	Upgrade(oldVersion string, spec map[string]interface{}) ([]interface{}, error)
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
