// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"regexp"
)

// validName is used by VMs and namespaces to exclude weird characters
var validName = regexp.MustCompile(`^[a-zA-Z0-9-_.]+$`)

// validNameErr can be returned when isValidName is not met
var validNameErr = errors.New(`invalid name; must only include letters, numbers, hyphens, underscores, and periods, and must not be "." or ".."`)

// isValidName reports whether name is an acceptable VM or namespace name. In
// addition to matching validName, the name must not be "." or "..". Namespace
// and VM names are used as directory names (see ron.NewServer, BaseVM.Flush,
// and Namespace.Save), and either of those would resolve to a directory other
// than the one intended.
func isValidName(name string) bool {
	return name != "." && name != ".." && validName.MatchString(name)
}
