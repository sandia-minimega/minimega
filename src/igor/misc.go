// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	log "minilog"
	"os"
	"path/filepath"
)

// install src into dir, using the hash as the file name. Returns the hash or
// an error.
func install(src, dir, suffix string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// hash the file
	hash := sha1.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("unable to hash file %v: %v", src, err)
	}

	fname := hex.EncodeToString(hash.Sum(nil))

	dst := filepath.Join(dir, fname+suffix)

	// copy the file if it doesn't already exist
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		// need to go back to the beginning of the file since we already read
		// it once to do the hashing
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}

		f2, err := os.Create(dst)
		if err != nil {
			return "", err
		}
		defer f2.Close()

		if _, err := io.Copy(f2, f); err != nil {
			return "", fmt.Errorf("unable to install %v: %v", src, err)
		}
	} else if err != nil {
		// strange...
		return "", err
	} else {
		log.Info("file with identical hash %v already exists, skipping install of %v.", fname, src)
	}

	return fname, nil
}

// purgeFiles removes the KernelHash/InitrdHash if they are not used by any
// other reservations.
func purgeFiles(r Reservation) error {
	// If no other reservations are using them, delete the kernel and/or initrd
	var kfound, ifound bool
	for _, r2 := range Reservations {
		if r2.KernelHash == r.KernelHash {
			kfound = true
		}
		if r2.InitrdHash == r.InitrdHash {
			ifound = true
		}
	}

	if !kfound {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", r.KernelHash+"-kernel")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	if !ifound {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", r.InitrdHash+"-initrd")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	return nil
}
