// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"os/exec"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var externalProcesses = map[string]string{
	"mount":       "mount",
	"qemu-nbd":    "qemu-nbd",
	"qemu-img":    "qemu-img",
	"umount":      "umount",
	"dd":          "dd",
	"mkfs":        "mkfs.ext3",
	"extlinux":    "extlinux",
	"cp":          "cp",
	"sfdisk":      "sfdisk",
	"debootstrap": "debootstrap",
	"chroot":      "chroot",
	"bash":        "bash",
	"mksquashfs":  "mksquashfs",
	"genisoimage": "genisoimage",
}

// check for the presence of each of the external processes we may call,
// and error if any aren't in our path
func externalCheck() {
	for _, i := range externalProcesses {
		path, err := exec.LookPath(i)
		if err != nil {
			e := fmt.Sprintf("%v not found", i)
			log.Errorln(e)
		} else {
			log.Info("%v found at: %v", i, path)
		}
	}
}

func process(p string) string {
	path, err := exec.LookPath(externalProcesses[p])
	if err != nil {
		log.Fatalln(err)
		return ""
	}
	return path
}
