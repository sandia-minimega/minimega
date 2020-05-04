package util

import (
	"os/exec"
)

// ShellCommandExists tests whether or not the given executable exists in the
// current path. Internally it shells out to the `which` command.
func ShellCommandExists(cmd string) bool {
	err := exec.Command("which", cmd).Run()
	return err == nil
}
