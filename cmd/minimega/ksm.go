// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	// ksmEnabled is set to true when KSM is enabled by minimega
	ksmEnabled bool
	// these variables save the old values when we enable KSM and are restored
	// when we disable KSM.
	ksmRun            int
	ksmPagesToScan    int
	ksmSleepMillisecs int
)

const (
	ksmPathRun            = "/sys/kernel/mm/ksm/run"
	ksmPathPagesToScan    = "/sys/kernel/mm/ksm/pages_to_scan"
	ksmPathSleepMillisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksmTunePagesToScan    = 100000
	ksmTuneSleepMillisecs = 10
)

func ksmEnable() error {
	if ksmEnabled {
		return errors.New("ksm is already enabled")
	}

	log.Info("saving current KSM values")

	// save the current values
	var err error
	ksmRun, err = readInt(ksmPathRun)
	if err != nil {
		return err
	}
	ksmPagesToScan, err = readInt(ksmPathPagesToScan)
	if err != nil {
		return err
	}
	ksmSleepMillisecs, err = readInt(ksmPathSleepMillisecs)
	if err != nil {
		return err
	}

	// enable KSM
	log.Info("enabling ksm")

	if err := writeInt(ksmPathRun, 1); err != nil {
		return err
	}
	if err := writeInt(ksmPathPagesToScan, ksmTunePagesToScan); err != nil {
		return err
	}
	if err := writeInt(ksmPathSleepMillisecs, ksmTuneSleepMillisecs); err != nil {
		return err
	}

	ksmEnabled = true
	return nil
}

// ksmDisable writes back the values that we saved in ksmEnable
func ksmDisable() error {
	if !ksmEnabled {
		return errors.New("ksm is not enabled")
	}

	log.Info("restoring ksm values")
	if err := writeInt(ksmPathRun, ksmRun); err != nil {
		return err
	}
	if err := writeInt(ksmPathPagesToScan, ksmPagesToScan); err != nil {
		return err
	}
	if err := writeInt(ksmPathSleepMillisecs, ksmSleepMillisecs); err != nil {
		return err
	}

	ksmEnabled = false
	return nil
}
