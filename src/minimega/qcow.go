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

type injectPair struct {
	src string
	dst string
}

type injectData struct {
	err       error
	srcImg    string
	dstImg    string
	partition string
	nbdPath   string
	pairs     []injectPair
}

var qcowCLIHandlers = []minicli.Handler{
	{ // vm inject
		HelpShort: "inject files into a qcow image",
		HelpLong: `
Create a backed snapshot of a qcow2 image and injects one or more files into
the new snapshot.

	srcimg - the name of the qcow to use as the backing image file. Optionally,
	specify a partition in which the files should be injected. Separated from
	the path to the image by a ':'. Defaults to one if no partition specified.

	dstimg - The optional name of the snapshot image. This should be a name
	only, if any extra path is specified, an error is thrown. This file will be
	created at 'base'/files. A filename will be generated if this optional
	parameter is omitted.

	files - src and destination file pairs. Each pair should contain two
	elements, the source and the destination, separated by a ':'. If the file
	paths contain spaces, use double quotes.

For example:

	vm inject dst dst.qc2 src src.qc2 "my file":"Program Files/my file"`,
		Patterns: []string{
			"vm inject src <srcimg> <files like /path/to/src:/path/to/dst>...",
			"vm inject dst <dstimg> src <srcimg> <files like /path/to/src:/path/to/dst>...",
		},
		Call: wrapSimpleCLI(cliVmInject),
	},
}

func init() {
	registerHandlers("qcow", qcowCLIHandlers)
}

//Parse the source-file:destination pairs
func (inject *injectData) parseInjectPairs(c *minicli.Command) {
	if inject.err != nil {
		return
	}

	inject.pairs = []injectPair{}

	// parse inject pairs
	for _, arg := range c.ListArgs["files"] {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			inject.err = errors.New("malformed command; expected src:dst pairs")
			return
		}

		inject.pairs = append(inject.pairs, injectPair{src: parts[0], dst: parts[1]})
		log.Debug("inject pair: %v, %v", parts[0], parts[1])
	}
}

//Parse the command line to get the arguments for vm_inject
func parseInject(c *minicli.Command) *injectData {
	inject := &injectData{}

	// parse source image
	srcPair := strings.Split(c.StringArgs["srcimg"], ":")
	inject.srcImg, inject.err = filepath.Abs(srcPair[0])

	if inject.err != nil {
		return inject
	}
	if len(srcPair) == 2 {
		inject.partition = srcPair[1]
	}

	log.Debug("source image: %v, partition %v", inject.srcImg, inject.partition)

	// parse destination image
	if strings.Contains(c.StringArgs["dstimg"], "/") {
		inject.err = errors.New("dst image must filename without path")
	} else if c.StringArgs["dstimg"] != "" {
		inject.dstImg = path.Join(*f_iomBase, c.StringArgs["dstimg"])
	}

	inject.parseInjectPairs(c)

	return inject
}

func (inject *injectData) run() (string, error) {
	if inject.err != nil {
		return "", inject.err
	}

	// create the new img
	p := process("qemu-img")
	cmd := exec.Command(p, "create", "-f", "qcow2", "-b", inject.srcImg, inject.dstImg)

	log.Debug("creating sub image with: %v", cmd)

	result, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v\n%v", string(result[:]), err)
	}

	// create a tmp mount point
	mntDir, err := ioutil.TempDir(*f_base, "dstImg")
	if err != nil {
		return "", err
	}
	log.Debug("temporary mount point: %v", mntDir)

	inject.nbdPath, err = nbd.ConnectImage(inject.dstImg)
	if err != nil {
		return "", err
	}
	defer vmInjectCleanup(mntDir, inject.nbdPath)

	time.Sleep(100 * time.Millisecond) // give time to create partitions

	// decide on a partition
	if inject.partition == "" {
		_, err = os.Stat(inject.nbdPath + "p1")
		if err != nil {
			return "", errors.New("no partitions found")
		}

		_, err = os.Stat(inject.nbdPath + "p2")
		if err == nil {
			return "", errors.New("please specify a partition; multiple found")
		}

		inject.partition = "1"
	}

	// mount new img
	p = process("mount")
	cmd = exec.Command(p, "-w", inject.nbdPath+"p"+inject.partition, mntDir)
	result, err = cmd.CombinedOutput()
	if err != nil {
		// check that ntfs-3g is installed
		p = process("ntfs-3g")
		cmd = exec.Command(p, "--version")
		_, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("ntfs-3g not found, ntfs images unwriteable")
		}

		// mount with ntfs-3g
		p = process("mount")
		cmd = exec.Command(p, "-o", "ntfs-3g", inject.nbdPath+"p"+inject.partition, mntDir)
		result, err = cmd.CombinedOutput()
		if err != nil {
			log.Error("failed to mount partition")
			return "", fmt.Errorf("%v\n%v", string(result[:]), err)
		}
	}

	// copy files/folders into mntDir
	p = process("cp")
	for _, pair := range inject.pairs {
		dir := filepath.Dir(filepath.Join(mntDir, pair.dst))
		os.MkdirAll(dir, 0775)
		cmd = exec.Command(p, "-fr", pair.src, mntDir+"/"+pair.dst)
		result, err = cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%v\n%v", string(result[:]), err)
		}
	}

	return inject.dstImg, nil
}

// Unmount, disconnect nbd, and remove mount directory
func vmInjectCleanup(mntDir, nbdPath string) {
	log.Debug("cleaning up vm inject: %s %s", mntDir, nbdPath)

	p := process("umount")
	cmd := exec.Command(p, mntDir)
	err := cmd.Run()
	if err != nil {
		log.Error("injectCleanup: %v", err)
	}

	err = nbd.DisconnectDevice(nbdPath)
	if err != nil {
		log.Error("qemu nbd disconnect: %v", err)
		log.Warn("minimega was unable to disconnect %v", nbdPath)
	}

	p = process("rm")
	cmd = exec.Command(p, "-r", mntDir)
	err = cmd.Run()
	if err != nil {
		log.Error("rm mount dir: %v", err)
	}
}

// Inject files into a qcow
// Alternatively, this function can also return a qcow2 backing file's name
func cliVmInject(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Load nbd
	err := nbd.Modprobe()
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	inject := parseInject(c)
	if inject.dstImg == "" {
		// Create new snapshot for image
		dstImg, err := ioutil.TempFile(*f_iomBase, "snapshot")
		if err != nil {
			inject.err = errors.New("could not create a dst image")
		} else {
			inject.dstImg = dstImg.Name()
		}
	}
	log.Debug("destination image: %v", inject.dstImg)

	resp.Response, err = inject.run()
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
