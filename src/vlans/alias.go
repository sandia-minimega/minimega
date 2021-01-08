// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vlans

import "strings"

// AliasSep separates namespace from VLAN alias
const AliasSep = "//"

type Alias struct {
	Namespace string
	Value     string
}

func (a Alias) String() string {
	return a.Namespace + AliasSep + a.Value
}

func ParseAlias(namespace, alias string) Alias {
	// If the alias includes the alias separator, assume the user wants to
	// override the namespace.
	if !strings.Contains(alias, AliasSep) {
		return Alias{
			Namespace: namespace,
			Value:     alias,
		}
	}

	i := strings.Index(alias, AliasSep)

	return Alias{
		Namespace: alias[:i],
		Value:     alias[i+len(AliasSep):],
	}
}
