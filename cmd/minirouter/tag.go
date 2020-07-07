package main

import (
	"fmt"
	"os/exec"
)

// set a tag in the upstream minimega. We don't log events as the logging
// subsystem uses this as well.
func tag(k, v string) error {
	if *f_miniccc == "" {
		return fmt.Errorf("no miniccc client set")
	}

	return exec.Command(*f_miniccc, "-tag", k, v).Run()
}
