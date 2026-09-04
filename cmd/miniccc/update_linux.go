//go:build linux

package main

import (
	"os"
	"syscall"
)

func restartUpdated(path, update string) error {
	if err := os.Rename(update, path); err != nil {
		return err
	}
	return syscall.Exec(path, os.Args, os.Environ())
}
