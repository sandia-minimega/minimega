// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"regexp"
)

// validName is used by namespaces to exclude weird characters
var validName = regexp.MustCompile(`^[a-zA-Z0-9-_.]+$`)

// validNameErr can be returned when validName is not met
var validNameErr = errors.New("invalid name; must only include letters, numbers, hyphens and underscores")

// Length limits for VM names. The maximum is from RFC 1035 section 2.3.4.
// Some guest operating systems, such as VyOS and Windows, reject host names
// of 1 character.
const (
	minVMNameLen = 2
	maxVMNameLen = 253
)

// hostLabel matches one host name label of 1 to 63 characters.
const hostLabel = `[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?`

// validHostname matches one or more host name labels separated by dots.
var validHostname = regexp.MustCompile(`^` + hostLabel + `(\.` + hostLabel + `)*$`)

// vmNameRule describes the names that validVMName accepts.
const vmNameRule = "must be a valid host name (RFC 1123) of 2 to 253 characters: dot-separated labels of 1 to 63 letters, digits and hyphens that do not start or end with a hyphen"

// validVMName returns an error unless name is a valid host name, as RFC 952
// and RFC 1123 section 2.1 define it, of 2 to 253 characters. Each
// dot-separated label has 1 to 63 characters, so "a.b" is valid and "a" is
// not.
func validVMName(name string) error {
	if len(name) < minVMNameLen || len(name) > maxVMNameLen {
		return fmt.Errorf("invalid vm name `%v`: length is %v; %v", name, len(name), vmNameRule)
	}

	if !validHostname.MatchString(name) {
		return fmt.Errorf("invalid vm name `%v`: %v", name, vmNameRule)
	}

	return nil
}

// caseConflictReason tells why a VM name conflicts with a name that
// differs only in letter case.
const caseConflictReason = "VM names are host names, which are not case-sensitive (RFC 4343)"

// vmNameKey returns name with ASCII letters in lower case. Two VM names
// conflict when their keys are equal, because host names are not
// case-sensitive (RFC 4343). Other bytes do not change.
func vmNameKey(name string) string {
	b := []byte(name)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}

	return string(b)
}
