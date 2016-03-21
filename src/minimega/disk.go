// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"nbd"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	INJECT_COMMAND = iota
	GET_BACKING_IMAGE_COMMAND
)

var diskCLIHandlers = []minicli.Handler{
	{ // disk
		HelpShort: "manipulate qcow disk images image",
		HelpLong: `
Manipulate qcow disk images. Supports creating new images, snapshots of
existing images, and injecting one or more files into an existing image.

Example of creating a new disk:

	disk create qcow2 foo.qcow2 100G

The size argument is the size in bytes, or using optional suffixes "k"
(kilobyte), "M" (megabyte), "G" (gigabyte), "T" (terabyte).

Example of taking a snapshot of a disk:

	disk snapshot windows7.qc2 window7_miniccc.qc2

If the destination name is omitted, a name will be randomly generated and the
snapshot will be stored in the 'files' directory. Snapshots are always created
in the 'files' directory.

To inject files into an image:

	disk inject window7_miniccc.qc2 files "miniccc":"Program Files/miniccc

Each argument after the image should be a source and destination pair,
separated by a ':'. If the file paths contain spaces, use double quotes.
Optionally, you may specify a partition (partition 1 will be used by default):

	disk inject window7_miniccc.qc2:2 files "miniccc":"Program Files/miniccc

You can optionally specify mount arguments to use with inject. Multiple options
should be quoted. For example:

	disk inject foo.qcow2 options "-t fat -o offset=100" files foo:bar

Disk image paths are always relative to the 'files' directory. Users may also
use absolute paths if desired.`,
		Patterns: []string{
			"disk <create,> <qcow2,raw> <image name> <size>",
			"disk <snapshot,> <image> [dst image]",
			"disk <inject,> <image> files <files like /path/to/src:/path/to/dst>...",
			"disk <inject,> <image> options <options> files <files like /path/to/src:/path/to/dst>...",
		},
		Call: wrapSimpleCLI(cliDisk),
	},
}

// diskSnapshot creates a new image, dst, using src as the backing image.
func diskSnapshot(src, dst string) error {
	out, err := processWrapper("qemu-img", "create", "-f", "qcow2", "-b", src, dst)
	if err != nil {
		return fmt.Errorf("%v: %v", out, err)
	}

	return nil
}

// diskCreate creates a new disk image, dst, of given size/format.
func diskCreate(format, dst, size string) error {
	out, err := processWrapper("qemu-img", "create", "-f", format, dst, size)
	if err != nil {
		log.Error("diskCreate: %v", out)
		return err
	}
	return nil
}

// diskInject injects files into a disk image. dst/partition specify the image
// and the partition number, pairs is the dst, src filepaths. options can be
// used to supply mount arguments.
func diskInject(dst, partition string, pairs map[string]string, options []string) error {
	// Load nbd
	if err := nbd.Modprobe(); err != nil {
		return err
	}

	// create a tmp mount point
	mntDir, err := ioutil.TempDir(*f_base, "dstImg")
	if err != nil {
		return err
	}
	log.Debug("temporary mount point: %v", mntDir)

	nbdPath, err := nbd.ConnectImage(dst)
	if err != nil {
		return err
	}
	defer diskInjectCleanup(mntDir, nbdPath)

	time.Sleep(100 * time.Millisecond) // give time to create partitions

	// decide on a partition
	if partition == "" {
		_, err = os.Stat(nbdPath + "p1")
		if err != nil {
			return errors.New("no partitions found")
		}

		_, err = os.Stat(nbdPath + "p2")
		if err == nil {
			return errors.New("please specify a partition; multiple found")
		}

		partition = "1"
	}

	// mount new img
	var path string
	if partition == "none" {
		path = nbdPath
	} else {
		path = nbdPath + "p" + partition
	}

	// we use mount(8), because the mount syscall (mount(2)) requires we
	// populate the fstype field, which we don't know
	args := []string{"mount"}
	if len(options) != 0 {
		args = append(args, options...)
		args = append(args, path, mntDir)
	} else {
		args = []string{"mount", "-w", path, mntDir}
	}
	log.Debug("mount args: %v", args)

	_, err = processWrapper(args...)
	if err != nil {
		// check that ntfs-3g is installed
		_, err = processWrapper("ntfs-3g", "--version")
		if err != nil {
			log.Error("ntfs-3g not found, ntfs images unwriteable")
		}

		// mount with ntfs-3g
		out, err := processWrapper("mount", "-o", "ntfs-3g", path, mntDir)
		if err != nil {
			log.Error("failed to mount partition")
			return fmt.Errorf("%v: %v", out, err)
		}
	}

	// copy files/folders into mntDir
	for dst, src := range pairs {
		dir := filepath.Dir(filepath.Join(mntDir, dst))
		os.MkdirAll(dir, 0775)

		out, err := processWrapper("cp", "-fr", src, filepath.Join(mntDir, dst))
		if err != nil {
			return fmt.Errorf("%v: %v", out, err)
		}
	}

	return nil
}

// parseInjectPairs parses a list of strings containing src:dst pairs into a
// map of where the dst is the key and src is the value. We build the map this
// way so that one source file can be written to multiple destinations and so
// that we can detect and return an error if the user tries to inject two files
// with the same destination.
func parseInjectPairs(files []string) (map[string]string, error) {
	pairs := map[string]string{}

	// parse inject pairs
	for _, arg := range files {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			return nil, errors.New("malformed command; expected src:dst pairs")
		}

		if pairs[parts[1]] != "" {
			return nil, fmt.Errorf("destination appears twice: `%v`", parts[1])
		}

		pairs[parts[1]] = parts[0]
		log.Debug("inject pair: %v, %v", parts[0], parts[1])
	}

	return pairs, nil
}

// diskInjectCleanup handles unmounting, disconnecting nbd, and removing mount
// directory after diskInject.
func diskInjectCleanup(mntDir, nbdPath string) {
	log.Debug("cleaning up vm inject: %s %s", mntDir, nbdPath)

	err := syscall.Unmount(mntDir, 0)
	if err != nil {
		log.Error("injectCleanup: %v", err)
	}

	if err := nbd.DisconnectDevice(nbdPath); err != nil {
		log.Error("qemu nbd disconnect: %v", err)
		log.Warn("minimega was unable to disconnect %v", nbdPath)
	}

	err = os.Remove(mntDir)
	if err != nil {
		log.Error("rm mount dir: %v", err)
	}
}

func cliDisk(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	image := c.StringArgs["image"]

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(image) {
		image = path.Join(*f_iomBase, image)
	}
	log.Debug("image: %v", image)

	if c.BoolArgs["snapshot"] {
		dst := c.StringArgs["dst"]

		if dst == "" {
			if f, err := ioutil.TempFile(*f_iomBase, "snapshot"); err != nil {
				resp.Error = "could not create a dst image"
				return resp
			} else {
				dst = f.Name()
				resp.Response = dst
			}
		} else if strings.Contains(dst, "/") {
			resp.Error = "dst image must filename without path"
			return resp
		} else {
			dst = path.Join(*f_iomBase, dst)
		}
		log.Debug("destination image: %v", dst)

		if err := diskSnapshot(image, dst); err != nil {
			resp.Error = err.Error()
		}
	} else if c.BoolArgs["inject"] {
		partition := "1"

		if strings.Contains(image, ":") {
			parts := strings.Split(image, ":")
			if len(parts) != 2 {
				resp.Error = "found way too many ':'s, expected <path/to/image>:<partition>"
				return resp
			}

			image, partition = parts[0], parts[1]
		}

		options := fieldsQuoteEscape("\"", c.StringArgs["options"])
		log.Debug("got options: %v", options)

		pairs, err := parseInjectPairs(c.ListArgs["files"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		if err := diskInject(image, partition, pairs, options); err != nil {
			resp.Error = err.Error()
		}
	} else if c.BoolArgs["create"] {
		size := c.StringArgs["size"]

		format := "raw"
		if _, ok := c.BoolArgs["qcow2"]; ok {
			format = "qcow2"
		}

		if err := diskCreate(format, image, size); err != nil {
			resp.Error = err.Error()
		}
	} else {
		// boo, should be unreachable
	}

	return resp
}
