package util

import (
	"os/exec"
)

func ShellCommandExists(cmd string) bool {
	err := exec.Command("which", cmd).Run()
	return err == nil
}
