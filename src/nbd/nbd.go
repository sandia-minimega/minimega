// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package nbd

import (
	"errors"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNoDeviceAvailable = errors.New("no available nbds found")
)

const (
	// How many times to retry connecting to a nbd device when all are
	// currently in use.
	maxConnectRetries = 3
)

// Ready checks to see if the NBD kernel module has been loaded. If it does not
// find the module, it returns an error. NBD functions should only be used
// after this function returns no error.
func Ready() error {
	// Ensure that the kernel module has been loaded
	p := process("lsmod")
	cmd := exec.Command(p)
	result, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	if !strings.Contains(string(result), "nbd ") {
		return errors.New("add module 'nbd'")
	}

	// Warn if nbd wasn't loaded with a max_part parameter
	_, err = os.Stat("/sys/module/nbd/parameters/max_part")
	if err != nil {
		log.Warnln("no max_part parameter set for module nbd")
	}

	return nil
}

// GetDevice returns the first available NBD. If there are no devices
// available, returns ErrNoDeviceAvailable.
func GetDevice() (string, error) {
	// Get a list of all devices
	devFiles, err := ioutil.ReadDir("/dev")
	if err != nil {
		return "", err
	}

	nbdPath := ""

	// Find the first available nbd
	for _, devInfo := range devFiles {
		dev := devInfo.Name()
		// we don't want to include partitions here
		if !strings.Contains(dev, "nbd") || strings.Contains(dev, "p") {
			continue
		}

		// check whether a pid exists for the current nbd
		_, err = os.Stat(filepath.Join("/sys/block", dev, "pid"))
		if err != nil {
			log.Debug("found available nbd: " + dev)
			nbdPath = filepath.Join("/dev", dev)
			break
		} else {
			log.Debug("nbd %v could not be used", dev)
		}
	}

	if nbdPath == "" {
		return "", ErrNoDeviceAvailable
	}

	return nbdPath, nil
}

// ConnectImage exports a image using the NBD protocol using the qemu-nbd. If
// successful, returns the NBD device.
func ConnectImage(image string) (string, error) {
	var nbdPath string
	var err error

	for i := 0; i < maxConnectRetries; i++ {
		nbdPath, err = GetDevice()
		if err != ErrNoDeviceAvailable {
			break
		}

		log.Debug("all nbds in use, sleeping before retrying")
		time.Sleep(time.Second * 10)
	}

	if err != nil {
		return "", err
	}

	// connect it to qemu-nbd
	p := process("qemu-nbd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-c",
			nbdPath,
			image,
		},
		Env: nil,
		Dir: "",
	}
	log.Debug("connecting to nbd with cmd: %v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	log.LogAll(stdout, log.INFO, "qemu-nbd")
	log.LogAll(stderr, log.ERROR, "qemu-nbd")

	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return nbdPath, nil
}

// DisconnectDevice disconnects a given NBD using qemu-nbd.
func DisconnectDevice(dev string) error {
	// disconnect nbd
	p := process("qemu-nbd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-d",
			dev,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "qemu-nbd")
	log.LogAll(stderr, log.ERROR, "qemu-nbd")

	log.Debug("disconnecting nbd with cmd: %v", cmd)
	return cmd.Run()
}
