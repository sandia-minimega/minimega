package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

var updateState struct {
	path string
	tmp  string
	perm os.FileMode
	file *os.File
}

func handleUpdateFile(f *ron.File) error {
	if f == nil {
		return fmt.Errorf("missing update file")
	}

	if f.Offset == 0 {
		if updateState.file != nil {
			updateState.file.Close()
		}

		path, err := os.Executable()
		if err != nil {
			return err
		}
		updateState.path = path
		updateState.tmp = updateTempPath(path)
		updateState.perm = f.Perm
		if err := os.Remove(updateState.tmp); err != nil && !os.IsNotExist(err) {
			return err
		}
		updateState.file, err = os.OpenFile(updateState.tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Perm)
		if err != nil {
			return err
		}
	}

	if updateState.file == nil {
		return fmt.Errorf("update started at offset %d", f.Offset)
	}
	if _, err := updateState.file.WriteAt(f.Data, f.Offset); err != nil {
		return err
	}
	if !f.EOF {
		return nil
	}

	if err := updateState.file.Close(); err != nil {
		return err
	}
	updateState.file = nil
	if err := os.Chmod(updateState.tmp, updateState.perm); err != nil {
		return err
	}
	return restartUpdated(updateState.path, updateState.tmp)
}

func updateTempPath(path string) string {
	return filepath.Clean(path) + ".update"
}
