// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"fmt"
	"unicode"
)

// Inverse of keysym.
var keysymInverse = map[uint32]string{}

func init() {
	for k, v := range keysym {
		// Use the first mapping for each key
		if _, ok := keysymInverse[v]; !ok {
			keysymInverse[v] = k
		}
	}
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

func asciiCharToKeysymString(c rune) (string, error) {
	// ascii 0x20 - 0x7E map directly to keysym values.
	// manually shift cases for tab, nl, and cr
	if c == 0x9 || c == 0xa || c == 0xd {
		c += 0xff00
	} else if c >= unicode.MaxASCII {
		return "", fmt.Errorf("unknown non-ascii character: %U %c", c, c)
	}
	keysym, err := xKeysymToString(uint32(c))
	if err != nil {
		return "uint32(0)", fmt.Errorf("character has no keysym mapping: %c", c)
	}
	return keysym, nil
}

func requiresShift(s string) bool {
	_, ok := shiftedKeysyms[s]
	return ok
}
