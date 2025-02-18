// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/nbd"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// #include "linux/fs.h"
import "C"

const (
	INJECT_COMMAND = iota
	GET_BACKING_IMAGE_COMMAND
)

type DiskInfo struct {
	Format      string
	VirtualSize string
	DiskSize    string
	BackingFile string
}

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

	disk inject window7_miniccc.qc2 files "miniccc":"Program Files/miniccc"

Each argument after the image should be a source and destination pair,
separated by a ':'. If the file paths contain spaces, use double quotes.
Optionally, you may specify a partition (partition 1 will be used by default):

	disk inject window7_miniccc.qc2:2 files "miniccc":"Program Files/miniccc"

You may also specify that there is no partition on the disk, if your filesystem
was directly written to the disk (this is highly unusual):

	disk inject partitionless_disk.qc2:none files /miniccc:/miniccc

You can optionally specify mount arguments to use with inject. Multiple options
should be quoted. For example:

	disk inject foo.qcow2 options "-t fat -o offset=100" files foo:bar

To delete files or directories from an image, specify the delete keyword
before listing the files or directories to delete from the image, separated by
a comma. For example:

	disk inject window7_miniccc.qc2 delete files "Program Files/miniccc.exe"
	disk inject window7_miniccc.qc2 delete files "Users/Administrator/Documents/TestDir"
	disk inject window7_miniccc.qc2 delete files "foo.txt,Temp/bar.zip"

Disk image paths are always relative to the 'files' directory. Users may also
use absolute paths if desired. The backing images for snapshots should always
be in the files directory.`,
		Patterns: []string{
			"disk <create,> <qcow2,raw> <image name> <size>",
			"disk <snapshot,> <image> [dst image]",
			"disk <inject,> <image> files <files like /path/to/src:/path/to/dst>...",
			"disk <inject,> <image> <delete,> files <files like /path/to/src,/path/to/src>...",
			"disk <inject,> <image> <options,> <options> files <files like /path/to/src:/path/to/dst>",
			"disk <inject,> <image> <options,> <options> <delete,> files <files like /path/to/src,/path/to/src>",
			"disk <info,> <image>",
		},
		Call: wrapSimpleCLI(cliDisk),
	},
}

// diskSnapshot creates a new image, dst, using src as the backing image.
func diskSnapshot(src, dst string) error {
	if !strings.HasPrefix(src, *f_iomBase) {
		log.Warn("minimega expects backing images to be in the files directory")
	}

	info, err := diskInfo(src)
	if err != nil {
		return fmt.Errorf("[image %s] error getting info: %v", src, err)
	}

	out, err := processWrapper("qemu-img", "create", "-f", "qcow2", "-b", src, "-F", info.Format, dst)
	if err != nil {
		return fmt.Errorf("[image %s] %v: %v", src, out, err)
	}

	return nil
}

// diskInfo return information about the disk.
func diskInfo(image string) (DiskInfo, error) {
	info := DiskInfo{}

	out, err := processWrapper("qemu-img", "info", image)
	if err != nil {
		return info, fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	regex := regexp.MustCompile(`.*\(actual path: (.*)\)`)

	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "file format":
			info.Format = parts[1]
		case "virtual size":
			info.VirtualSize = parts[1]
		case "disk size":
			info.DiskSize = parts[1]
		case "backing file":
			// In come cases, `qemu-img info` includes the actual absolute path for
			// the backing image. We want to use that, if present.
			if match := regex.FindStringSubmatch(parts[1]); match != nil {
				info.BackingFile = match[1]
			} else {
				info.BackingFile = parts[1]
			}
		}
	}

	return info, nil
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

// diskInject injects files into or deletes files from a disk image.
// dst/partition specify the image and the partition number. for injecting
// files, pairs is the dst/src filepaths. for deleting files, paths is the
// comma-separated list of filepaths to delete. options can be used to supply
// mount arguments.
func diskInject(dst, partition string, pairs map[string]string, options []string, delete bool, paths []string) error {
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
	defer func() {
		if err := os.Remove(mntDir); err != nil {
			log.Error("rm mount dir failed: %v", err)
		}
	}()

	nbdPath, err := nbd.ConnectImage(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := nbd.DisconnectDevice(nbdPath); err != nil {
			log.Error("nbd disconnect failed: %v", err)
		}
	}()

	path := nbdPath

	f, err := os.Open(nbdPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// decide whether to mount partition or raw disk
	if partition != "none" {
		// keep rereading partitions and waiting for them to show up for a bit
		timeoutTime := time.Now().Add(5 * time.Second)
		for i := 1; ; i++ {
			if time.Now().After(timeoutTime) {
				return fmt.Errorf("[image %s] no partitions found on image", dst)
			}

			// tell kernel to reread partitions
			syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), C.BLKRRPART, 0)

			_, err = os.Stat(nbdPath + "p1")
			if err == nil {
				log.Info("partitions detected after %d attempt(s)", i)
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		// default to first partition if there is only one partition
		if partition == "" {
			_, err = os.Stat(nbdPath + "p2")
			if err == nil {
				return fmt.Errorf("[image %s] please specify a partition; multiple found", dst)
			}

			partition = "1"
		}

		path = nbdPath + "p" + partition

		// check desired partition exists
		for i := 1; i <= 5; i++ {
			_, err = os.Stat(path)
			if err != nil {
				err = fmt.Errorf("[image %s] desired partition %s not found", dst, partition)

				time.Sleep(time.Duration(i*100) * time.Millisecond)
				continue
			}

			log.Info("desired partition %s found in image %s", partition, dst)
			break
		}

		if err != nil {
			return err
		}
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
			return fmt.Errorf("[image %s] %v: %v", dst, out, err)
		}
	}
	defer func() {
		if err := syscall.Unmount(mntDir, 0); err != nil {
			log.Error("unmount failed: %v", err)
		}
	}()

	if delete {
		// delete the file paths from mntDir.
		for _, path := range paths {
			mntPath := filepath.Join(mntDir, path)
			if _, err := os.Stat(mntPath); os.IsNotExist(err) {
				log.Warn("[image %s] path does not exist to delete: %v", dst, path)
			} else {
				err := os.RemoveAll(mntPath)
				if err != nil {
					return fmt.Errorf("[image %s] error deleting '%s': %v", dst, path, err)
				}
			}
		}
	} else {
		// copy files/folders into mntDir
		for target, source := range pairs {
			dir := filepath.Dir(filepath.Join(mntDir, target))
			os.MkdirAll(dir, 0775)

			out, err := processWrapper("cp", "-fr", source, filepath.Join(mntDir, target))
			if err != nil {
				return fmt.Errorf("[image %s] %v: %v", dst, out, err)
			}
		}
	}

	// explicitly flush buffers
	out, err := processWrapper("blockdev", "--flushbufs", path)
	if err != nil {
		return fmt.Errorf("[image %s] unable to flush: %v %v", dst, out, err)
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

// parseFiles parses the files argument passed into the 'disk inject ...'
// command. if options are used, the files argument gets turned into a
// minicli.Command StringArg, otherwise it gets turned into a ListArg.
// this function returns either a paths string slice if 'delete' is part
// of the original command, or a pairs string map.
func parseFiles(files interface{}, delete bool) (map[string]string, []string, error) {
	var pairs map[string]string
	var paths []string
	var err error
	switch v := files.(type) {
	case []string:
		if delete {
			if sliceContainsString(v, ",") {
				paths = strings.Split(v[0], ",")
			} else {
				paths = []string{v[0]} // single file
			}
		} else {
			pairs, err = parseInjectPairs(v)
		}
	case string:
		if delete {
			if strings.Contains(v, ",") {
				paths = strings.Split(v, ",")
			} else {
				paths = []string{v} // single file
			}
		} else {
			pairs, err = parseInjectPairs([]string{v})
		}
	default:
		return nil, nil, errors.New("error parsing files: unknown type")
	}

	return pairs, paths, err
}

func cliDisk(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := filepath.Clean(c.StringArgs["image"])

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(image) {
		image = path.Join(*f_iomBase, image)
	}
	log.Debug("image: %v", image)

	if c.BoolArgs["snapshot"] {
		dst := c.StringArgs["dst"]

		if dst == "" {
			f, err := ioutil.TempFile(*f_iomBase, "snapshot")
			if err != nil {
				return errors.New("could not create a dst image")
			}

			dst = f.Name()
			resp.Response = dst
		} else if strings.Contains(dst, "/") {
			return errors.New("dst image must filename without path")
		} else {
			dst = path.Join(*f_iomBase, dst)
		}

		log.Debug("destination image: %v", dst)

		return diskSnapshot(image, dst)
	} else if c.BoolArgs["inject"] {
		var partition string

		if strings.Contains(image, ":") {
			parts := strings.Split(image, ":")
			if len(parts) != 2 {
				return errors.New("found way too many ':'s, expected <path/to/image>:<partition>")
			}

			image, partition = parts[0], parts[1]
		}

		delete := strings.Contains(c.Original, " delete files ")

		options := fieldsQuoteEscape("\"", c.StringArgs["options"])
		log.Debug("got options: %v", options)

		var files interface{}
		if _, ok := c.StringArgs["options"]; !ok {
			files = c.ListArgs["files"]
		} else {
			files = c.StringArgs["files"]
		}

		pairs, paths, err := parseFiles(files, delete)
		if err != nil {
			return err
		}

		return diskInject(image, partition, pairs, options, delete, paths)
	} else if c.BoolArgs["create"] {
		size := c.StringArgs["size"]

		format := "raw"
		if _, ok := c.BoolArgs["qcow2"]; ok {
			format = "qcow2"
		}

		return diskCreate(format, image, size)
	} else if c.BoolArgs["info"] {
		info, err := diskInfo(image)
		if err != nil {
			return err
		}

		resp.Header = []string{"image", "format", "virtualsize", "disksize", "backingfile"}
		resp.Tabular = append(resp.Tabular, []string{
			image, info.Format, info.VirtualSize, info.DiskSize, info.BackingFile,
		})

		return nil
	}

	return unreachable()
}
