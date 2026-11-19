// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var (
	i440fxVersioned = regexp.MustCompile(`^pc-i440fx-(\d+)\.(\d+)$`)
	aliasOf         = regexp.MustCompile(`\(alias of ([^)]+)\)`)
)

// OlderMachine returns a versioned pc-i440fx machine type from the output of
// "qemu -M help" that a VM does not get by default. It returns the highest
// version that is not the default machine type, not the target of an alias
// such as "pc", and not deprecated. Versions older than 2.4 are not used:
// they send no machine name in a migration stream.
func OlderMachine(help string) (string, error) {
	skip := map[string]bool{}
	type candidate struct {
		name         string
		major, minor int
	}
	var all []candidate

	for _, line := range strings.Split(help, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if m := aliasOf.FindStringSubmatch(line); m != nil {
			skip[m[1]] = true
			continue
		}
		if strings.Contains(line, "(default)") || strings.Contains(line, "(deprecated)") {
			skip[fields[0]] = true
			continue
		}
		m := i440fxVersioned.FindStringSubmatch(fields[0])
		if m == nil {
			continue
		}
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		if major < 2 || (major == 2 && minor < 4) {
			continue
		}
		all = append(all, candidate{fields[0], major, minor})
	}

	var best *candidate
	for i, c := range all {
		if skip[c.name] {
			continue
		}
		if best == nil || c.major > best.major || (c.major == best.major && c.minor > best.minor) {
			best = &all[i]
		}
	}
	if best == nil {
		return "", errors.New("no versioned pc-i440fx machine type other than the default")
	}

	return best.name, nil
}
