// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package nbd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	ErrNoDeviceAvailable = errors.New("no available nbds found")

	// nbdLock serializes device selection and connection so concurrent
	// callers can't claim the same device.
	nbdLock sync.Mutex
)

const (
	// How many times to retry connecting to a nbd device when all are
	// currently in use.
	maxConnectRetries = 3
)

func Modprobe() error {
	// Load the kernel module
	// This will probably fail unless you are root
	if _, err := processWrapper("modprobe", "nbd", "max_part=10"); err != nil {
		return err
	}

	// It's possible nbd was already loaded but max_part wasn't set
	return Ready()
}

// Ready checks to see if the NBD kernel module has been loaded. If it does not
// find the module, it returns an error. NBD functions should only be used
// after this function returns no error.
func Ready() error {
	// Ensure that the kernel module has been loaded
	out, err := processWrapper("lsmod")
	if err != nil {
		return err
	}

	if !strings.Contains(out, "nbd ") {
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

		// Check whether the nbd has an associated socket or PID to determine
		// if it is available
		_, err_sock := os.Stat(filepath.Join("/var/lock", "qemu-nbd-"+dev))
		_, err_pid := os.Stat(filepath.Join("/sys/block", dev, "pid"))

		if os.IsNotExist(err_sock) && os.IsNotExist(err_pid) {
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
	var err error

	for i := 0; i < maxConnectRetries; i++ {
		var nbdPath string

		// Hold the lock across selection and connection so a concurrent caller
		// can't grab the same device between the check and qemu-nbd -c.
		nbdLock.Lock()

		nbdPath, err = GetDevice()
		if err != nil {
			nbdLock.Unlock()

			if err != ErrNoDeviceAvailable {
				return "", err
			}

			log.Debug("all nbds in use, sleeping before retrying")
			time.Sleep(time.Second * 10)

			continue
		}

		log.Debug("connect nbd: %v -> %v", image, nbdPath)

		out, cerr := processWrapper("qemu-nbd", "-c", nbdPath, image)

		nbdLock.Unlock()

		if cerr == nil {
			return nbdPath, nil
		}

		// The device was likely claimed since the check; try the next one.
		err = fmt.Errorf("unable to connect to nbd: %v", out)
		log.Debug("nbd connect failed, retrying: %v", err)
	}

	return "", err
}

// DisconnectDevice disconnects a given NBD using qemu-nbd.
func DisconnectDevice(dev string) error {
	log.Debug("disconnect nbd: %v", dev)

	// disconnect nbd
	out, err := processWrapper("qemu-nbd", "-d", dev)
	if err != nil {
		return fmt.Errorf("unable to disconnect nbd: %v", out)
	}

	return nil
}
