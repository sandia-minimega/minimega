// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// backgroundStopWait limits how long the client waits for killed background
// processes to exit before it restarts.
const backgroundStopWait = 5 * time.Second

// updater stages a miniccc binary that the server sends in chunks and starts
// it when the last chunk arrives.
type updater struct {
	// executable returns the path of the installed client binary.
	executable func() (string, error)
	// restart replaces the installed binary with the staged one and starts it.
	restart func(path, update string) error
	// stop kills the background processes before the restart.
	stop func()

	path string
	tmp  string
	perm os.FileMode
	file *os.File
}

var clientUpdater = &updater{executable: os.Executable, restart: restartUpdated, stop: killBackground}

func handleUpdateFile(f *ron.File) error {
	return clientUpdater.handle(f)
}

// refreshUpdateInfo sets the binary hash and the capabilities that the client
// reports in the handshake.
func refreshUpdateInfo() {
	sum, caps := clientUpdater.probe()

	client.Lock()
	defer client.Unlock()

	client.SHA256 = sum
	client.Capabilities = caps
}

// probe returns the SHA-256 hash of the installed binary and the capabilities
// of the client. The client can update only when it can create a file in the
// folder that holds its binary.
func (u *updater) probe() (string, []string) {
	path, err := u.executable()
	if err != nil {
		log.Warn("miniccc auto-update is not available: %v", err)
		return "", nil
	}

	sum, err := ron.FileSHA256(path)
	if err != nil {
		log.Warn("miniccc auto-update is not available: %v", err)
		return "", nil
	}

	f, err := os.CreateTemp(filepath.Dir(path), ".miniccc-update-check-*")
	if err != nil {
		log.Warn("miniccc auto-update is not available: %v", err)
		return sum, nil
	}
	f.Close()
	os.Remove(f.Name())

	return sum, []string{ron.CapAutoUpdate}
}

// killBackground kills the background processes that the client started and
// waits a short time for them to exit.
func killBackground() {
	client.Lock()
	if len(client.Processes) > 0 {
		log.Warn("miniccc auto-update kills %v background processes", len(client.Processes))
	}
	for _, p := range client.Processes {
		log.Info("killing background process %v: %v", p.PID, p.Command)
		if err := p.process.Kill(); err != nil {
			log.Error("kill background process %v: %v", p.PID, err)
		}
	}
	client.Unlock()

	for deadline := time.Now().Add(backgroundStopWait); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		client.Lock()
		n := len(client.Processes)
		client.Unlock()

		if n == 0 {
			return
		}
	}
}

func abortUpdate() {
	clientUpdater.abort()
}

func (u *updater) handle(f *ron.File) error {
	if f == nil {
		return fmt.Errorf("missing update file")
	}

	if err := u.receive(f); err != nil {
		u.abort()
		return err
	}
	if !f.EOF {
		return nil
	}

	u.file = nil
	u.stop()
	if err := u.restart(u.path, u.tmp); err != nil {
		os.Remove(u.tmp)
		return err
	}
	return nil
}

// receive writes one chunk to the staged file. After the last chunk, it closes
// the staged file and gives it the permissions of the installed binary.
func (u *updater) receive(f *ron.File) error {
	if f.Offset == 0 {
		u.abort()

		path, err := u.executable()
		if err != nil {
			return err
		}
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		u.path = path
		u.tmp = updateTempPath(path)
		u.perm = fi.Mode().Perm()
		if err := os.Remove(u.tmp); err != nil && !os.IsNotExist(err) {
			return err
		}
		u.file, err = os.OpenFile(u.tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
	}

	if u.file == nil {
		return fmt.Errorf("update started at offset %d", f.Offset)
	}
	if _, err := u.file.WriteAt(f.Data, f.Offset); err != nil {
		return err
	}
	if !f.EOF {
		return nil
	}

	if err := u.file.Close(); err != nil {
		return err
	}
	return os.Chmod(u.tmp, u.perm)
}

// abort stops the transfer in progress, if any, and removes its staged file.
func (u *updater) abort() {
	if u.file == nil {
		return
	}
	u.file.Close()
	u.file = nil
	os.Remove(u.tmp)
}

func updateTempPath(path string) string {
	return filepath.Clean(path) + ".update"
}

func updateBackupPath(path string) string {
	return filepath.Clean(path) + ".old"
}

// exitForRestart installs the update and exits with a non-zero code. The
// recovery action of the Windows service then starts the new binary.
func exitForRestart(path, update string, exit func(int)) error {
	if _, err := installUpdate(path, update); err != nil {
		return err
	}

	log.Info("miniccc auto-update installed, exiting so that the service restarts")
	exit(1)
	return nil
}

// installUpdate moves the installed binary to a backup path and moves the
// staged binary to the installed path. The returned function restores the
// backup if the new binary does not start.
func installUpdate(path, update string) (func() error, error) {
	backup := updateBackupPath(path)
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(path, backup); err != nil {
		return nil, err
	}
	if err := os.Rename(update, path); err != nil {
		return nil, errors.Join(err, os.Rename(backup, path))
	}

	return func() error {
		return os.Rename(backup, path)
	}, nil
}
