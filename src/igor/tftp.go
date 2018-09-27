// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type TFTPBackend struct {
}

func NewTFTPBackend() Backend {
	return &TFTPBackend{}
}

func (b *TFTPBackend) Install(r Reservation) error {
	// Manual file installation happens now
	// create appropriate pxe config file in igorConfig.TFTPRoot+/pxelinux.cfg/igor/
	masterfile, err := os.Create(r.Filename())
	if err != nil {
		return fmt.Errorf("failed to create %v -- %v", r.Filename(), err)
	}
	defer masterfile.Close()

	masterfile.WriteString(fmt.Sprintf("default %s\n\n", r.ResName))
	masterfile.WriteString(fmt.Sprintf("label %s\n", r.ResName))
	masterfile.WriteString(fmt.Sprintf("kernel /igor/%s-kernel\n", r.KernelHash))
	masterfile.WriteString(fmt.Sprintf("append initrd=/igor/%s-initrd %s\n", r.InitrdHash, r.KernelArgs))

	// create individual PXE boot configs i.e. igorConfig.TFTPRoot+/pxelinux.cfg/AC10001B by copying config created above
	for _, pxename := range r.PXENames {
		masterfile.Seek(0, 0)

		fname := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", pxename)
		f, err := os.Create(fname)
		if err != nil {
			return fmt.Errorf("failed to create %v -- %v", fname, err)
		}

		io.Copy(f, masterfile)
		f.Close()
	}

	return nil
}

func (b *TFTPBackend) Uninstall(r Reservation) error {
	// Delete all the PXE files in the reservation
	for _, pxename := range r.PXENames {
		// TODO: check error?
		os.Remove(filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", pxename))
	}

	return nil
}

func (b *TFTPBackend) Power(hosts []string, on bool) error {
	command := igorConfig.PowerOffCommand
	if on {
		command = igorConfig.PowerOnCommand
	}

	if command == "" {
		return errors.New("power configuration missing")
	}

	runner := DefaultRunner(func(host string) error {
		cmd := strings.Split(fmt.Sprintf(command, host), " ")
		_, err := processWrapper(cmd...)
		return err
	})

	return runner.RunAll(hosts)
}
