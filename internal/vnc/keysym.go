// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"fmt"
	"maps"
	"slices"
)

// Inverse of keysym.
var keysymInverse = buildKeysymInverse()

// preferredKeysyms are the names to use for keysyms that have more than one
// name, where the first name in sort order is not the usual name.
var preferredKeysyms = []string{"KP_Page_Down", "Mode_switch", "Page_Down"}

// buildKeysymInverse maps each keysym to one name. It uses the preferred
// name, or else the first name in sort order.
func buildKeysymInverse() map[uint32]string {
	res := make(map[uint32]string, len(keysym))

	for _, k := range slices.Sorted(maps.Keys(keysym)) {
		if _, ok := res[keysym[k]]; !ok {
			res[keysym[k]] = k
		}
	}

	for _, k := range preferredKeysyms {
		res[keysym[k]] = k
	}

	return res
}

func xKeysymToString(k uint32) (string, error) {
	if v, ok := keysymInverse[k]; ok {
		return v, nil
	}

	return "", fmt.Errorf("unknown keysym: %x", k)
}

func xStringToKeysym(s string) (uint32, error) {
	if v, ok := keysym[s]; ok {
		return v, nil
	}

	return uint32(0), fmt.Errorf("unknown key: `%s`", s)
}

// pasteKey returns the kb name of the key that types c. It returns false if
// paste cannot type c: paste types only printable ASCII, Tab, and Enter.
func pasteKey(c rune) (string, bool) {
	switch {
	case c == '\t':
		return "Tab", true
	case c == '\n':
		return "Return", true
	case c >= 0x20 && c <= 0x7e:
		// printable ASCII characters are also their keysyms
		name, ok := keysymInverse[uint32(c)]
		return name, ok
	}

	return "", false
}

func requiresShift(s string) bool {
	_, ok := shiftedKeysyms[s]
	return ok
}
