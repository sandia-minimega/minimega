// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package vnc

import "fmt"

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
