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
	"os/exec"
	"path"
	"path/filepath"
	"strings"
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
Manipulate qcow disk images. Supports creating snapshots of existing images and
injecting one or more files into an existing image.

Example of taking a snapshot of a disk:

	disk snapshot windows7.qc2 window7_miniccc.qc2

If the destination name is omitted, a name will be randomly generated and the
snapshot will be stored in the 'file' directory.

To inject files into an image:

	disk inject window7_miniccc.qc2 "miniccc":"Program Files/miniccc

Each argument after the image should be a source and destination pair,
separated by a ':'. If the file paths contain spaces, use double quotes. Optionally,
you may specify a partition (partition 1 will be used by default):

	disk inject window7_miniccc.qc2:2 "miniccc":"Program Files/miniccc

You can optionally specify mount arguments to use with inject. Multiple options should be quoted. For example:

	disk inject foo.qcow2 "-t fat -o offset=100" foo:bar`,
		Patterns: []string{
			"disk <snapshot,> <src image> [dst image]",
			"disk <inject,> <image> <options> <files like /path/to/src:/path/to/dst>...",
			"disk <inject,> <image> <files like /path/to/src:/path/to/dst>...",
		},
		Call: wrapSimpleCLI(cliDisk),
	},
}

func diskSnapshot(src, dst string) error {
	// create the new img
	p := process("qemu-img")
	cmd := exec.Command(p, "create", "-f", "qcow2", "-b", src, dst)

	log.Debug("creating snapshot with: %v", cmd)

	result, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%v", string(result), err)
	}

	return nil
}

func diskInject(dst, partition string, pairs map[string]string, option string) error {
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

	options := strings.Fields(option)

	// mount new img
	p := process("mount")
	var cmd *exec.Cmd
	if len(options) != 0 {
		args := append([]string{"-w"}, options...)
		args = append(args, nbdPath+"p"+partition, mntDir)
		cmd = exec.Command(p, args...)
	} else {
		cmd = exec.Command(p, "-w", nbdPath+"p"+partition, mntDir)
	}
	if result, err := cmd.CombinedOutput(); err != nil {
		// check that ntfs-3g is installed
		p = process("ntfs-3g")
		cmd = exec.Command(p, "--version")
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("ntfs-3g not found, ntfs images unwriteable")
		}

		// mount with ntfs-3g
		p = process("mount")
		cmd = exec.Command(p, "-o", "ntfs-3g", nbdPath+"p"+partition, mntDir)
		result, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("failed to mount partition")
			return fmt.Errorf("%v\n%v", string(result[:]), err)
		}
	}

	// copy files/folders into mntDir
	p = process("cp")
	for dst, src := range pairs {
		dir := filepath.Dir(filepath.Join(mntDir, dst))
		os.MkdirAll(dir, 0775)

		cmd := exec.Command(p, "-fr", src, mntDir+"/"+dst)
		if result, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v\n%v", string(result[:]), err)
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

	p := process("umount")
	cmd := exec.Command(p, mntDir)
	if err := cmd.Run(); err != nil {
		log.Error("injectCleanup: %v", err)
	}

	if err := nbd.DisconnectDevice(nbdPath); err != nil {
		log.Error("qemu nbd disconnect: %v", err)
		log.Warn("minimega was unable to disconnect %v", nbdPath)
	}

	p = process("rm")
	cmd = exec.Command(p, "-r", mntDir)
	if err := cmd.Run(); err != nil {
		log.Error("rm mount dir: %v", err)
	}
}

func cliDisk(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["snapshot"] {
		var err error
		src, dst := c.StringArgs["src"], c.StringArgs["dst"]

		src, err = filepath.Abs(src)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

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

		if err := diskSnapshot(src, dst); err != nil {
			resp.Error = err.Error()
		}
	} else if c.BoolArgs["inject"] {
		image, partition := c.StringArgs["image"], "1"

		if strings.Contains(image, ":") {
			parts := strings.Split(image, ":")
			if len(parts) != 2 {
				resp.Error = "found way too many ':'s, expected <path/to/image>:<partition>"
				return resp
			}

			image, partition = parts[0], parts[1]
		}

		image, err := filepath.Abs(image)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		pairs, err := parseInjectPairs(c.ListArgs["files"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		option := c.StringArgs["option"]

		if err := diskInject(image, partition, pairs, option); err != nil {
			resp.Error = err.Error()
		}
	} else {
		// boo, should be unreachable
	}

	return resp
}
