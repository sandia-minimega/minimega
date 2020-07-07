// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// DiskConfig contains all the disk-related config for a disk.
type DiskConfig struct {
	Path         string
	SnapshotPath string
	Interface    string
	Cache        string

	// Raw string that we used when creating this disk config will be
	// reparsed if we ever clone the VM that has this config.
	Raw string
}

type DiskConfigs []DiskConfig

// ParseDiskConfig processes the input specifying the disk image path, interface,
// and cache mode and udpates the vm config accordingly.
func ParseDiskConfig(spec string, snapshot bool) (*DiskConfig, error) {
	// example: /data/minimega/images/linux.qcow2,virtio,writeback
	f := strings.Split(spec, ",")

	// path, interface, cache
	var p, i, c string

	switch len(f) {
	case 1:
		// path
		p = f[0]
	case 2:
		if isCache(f[1]) {
			// path, cache
			p, c = f[0], f[1]
		} else if isInterface(f[1]) {
			// path, interface
			p, i = f[0], f[1]
		} else {
			return nil, errors.New("malformed diskspec")
		}
	case 3:
		if isInterface(f[1]) && isCache(f[2]) {
			// path, interface, cache
			p, i, c = f[0], f[1], f[2]
		} else {
			return nil, errors.New("malformed diskspec")
		}
	default:
		return nil, errors.New("malformed diskspec")
	}

	log.Info(`got path="%v", interface="%v", cache="%v"`, p, i, c)

	p = checkPath(p)

	return &DiskConfig{
		Path:      p,
		Cache:     c,
		Interface: i,
	}, nil
}

// String representation of DiskConfig, should be able to parse back into a
// DiskConfig.
func (c DiskConfig) String() string {
	parts := []string{}

	parts = append(parts, c.Path)

	if c.Interface != "" {
		parts = append(parts, c.Interface)
	}

	if c.Cache != "" {
		parts = append(parts, c.Cache)
	}

	return strings.Join(parts, ",")
}

func (c DiskConfigs) String() string {
	parts := []string{}
	for _, n := range c {
		parts = append(parts, n.String())
	}

	return strings.Join(parts, " ")
}

func (c DiskConfigs) WriteConfig(w io.Writer) error {
	if len(c) > 0 {
		_, err := fmt.Fprintf(w, "vm config disk %v\n", c)
		return err
	}

	return nil
}

// disk interface cache mode is a hypervisor-independant feature
func isCache(c string) bool {
	// supported QEMU disk cache modes from the man page
	validCaches := map[string]bool{"none": true, "writeback": true, "unsafe": true, "directsync": true, "writethrough": true}

	return validCaches[c]
}

func isInterface(i string) bool {
	// supported QEMU disk interfaces from the man page
	// AND our custom "ahci" that means we set up the QEMU args in a different way later
	validInterfaces := map[string]bool{"ahci": true, "ide": true, "scsi": true, "sd": true, "mtd": true, "floppy": true, "pflash": true, "virtio": true}

	return validInterfaces[i]
}
