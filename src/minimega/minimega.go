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

// validNameErr can be returned when validName is not met
var validNameErr = errors.New("invalid name; must only include letters, numbers, hyphens and underscores")
