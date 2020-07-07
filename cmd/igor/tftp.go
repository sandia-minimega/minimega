// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type TFTPBackend struct {
}

func NewTFTPBackend() Backend {
	return &TFTPBackend{}
}

func (b *TFTPBackend) Install(r *Reservation) error {
	// Manual file installation happens now
	// create appropriate pxe config file in igor.TFTPRoot+/pxelinux.cfg/igor/
	masterfile, err := os.Create(r.Filename())
	if err != nil {
		return fmt.Errorf("failed to create %v -- %v", r.Filename(), err)
	}
	defer masterfile.Close()

	masterfile.WriteString(fmt.Sprintf("default %s\n\n", r.Name))
	masterfile.WriteString(fmt.Sprintf("label %s\n", r.Name))
	masterfile.WriteString(fmt.Sprintf("kernel /igor/%s-kernel\n", r.KernelHash))
	masterfile.WriteString(fmt.Sprintf("append initrd=/igor/%s-initrd %s\n", r.InitrdHash, r.KernelArgs))

	// create individual PXE boot configs i.e. igor.TFTPRoot+/pxelinux.cfg/AC10001B by copying config created above
	for _, pxename := range r.PXENames {
		masterfile.Seek(0, 0)

		fname := filepath.Join(igor.TFTPRoot, "pxelinux.cfg", pxename)
		f, err := os.Create(fname)
		if err != nil {
			return fmt.Errorf("failed to create %v -- %v", fname, err)
		}

		io.Copy(f, masterfile)
		f.Close()
	}

	return nil
}

func (b *TFTPBackend) Uninstall(r *Reservation) error {
	// Delete all the PXE files in the reservation
	for _, pxename := range r.PXENames {
		// TODO: check error?
		os.Remove(filepath.Join(igor.TFTPRoot, "pxelinux.cfg", pxename))
	}

	return nil
}
