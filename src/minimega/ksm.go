// minimega
//
// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// routines to save/restore and throttle the ksm state
package main

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"strconv"
	"strings"
)

var (
	ksmPagesToScan    int
	ksmRun            int
	ksmSleepMillisecs int
	ksmEnabled        bool
)

const (
	ksmPathRun            = "/sys/kernel/mm/ksm/run"
	ksmPathPagesToScan    = "/sys/kernel/mm/ksm/pages_to_scan"
	ksmPathSleepMillisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksmTunePagesToScan    = 100000
	ksmTuneSleepMillisecs = 10
)

func ksmSave() {
	log.Infoln("saving ksm values")
	ksmRun = ksmGetIntFromFile(ksmPathRun)
	ksmPagesToScan = ksmGetIntFromFile(ksmPathPagesToScan)
	ksmSleepMillisecs = ksmGetIntFromFile(ksmPathSleepMillisecs)
}

func ksmGetIntFromFile(filename string) int {
	buffer, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalln(err)
	}
	b := strings.TrimSpace(string(buffer))
	log.Info("read: %v", b)
	run, err := strconv.Atoi(b)
	if err != nil {
		log.Fatalln(err)
	}
	log.Info("got %v from %v", int(run), filename)
	return int(run)
}

func ksmEnable() {
	if !ksmEnabled {
		ksmSave()
		log.Infoln("enabling ksm")
		ksmWrite(ksmPathRun, 1)
		ksmWrite(ksmPathPagesToScan, ksmTunePagesToScan)
		ksmWrite(ksmPathSleepMillisecs, ksmTuneSleepMillisecs)
		ksmEnabled = true
	}
}

func ksmDisable() {
	if ksmEnabled {
		log.Infoln("restoring ksm values")
		ksmWrite(ksmPathRun, ksmRun)
		ksmWrite(ksmPathPagesToScan, ksmPagesToScan)
		ksmWrite(ksmPathSleepMillisecs, ksmSleepMillisecs)
		ksmEnabled = false
	}
}

func ksmWrite(filename string, value int) {
	file, err := os.Create(filename)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer file.Close()
	log.Info("writing %v to %v", value, filename)
	file.WriteString(strconv.Itoa(value))
}

func ksmCLI(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		r := "disabled"
		if ksmEnabled {
			r = "enabled"
		}
		return cliResponse{
			Response: fmt.Sprintf("%v", r),
		}
	case 1:
		switch strings.ToLower(c.Args[0]) {
		case "disable":
			ksmDisable()
		case "enable":
			ksmEnable()
		default:
			return cliResponse{
				Error: "valid arguments are [enable, disable]",
			}
		}
	default:
		return cliResponse{
			Error: "ksm takes one argument",
		}
	}
	return cliResponse{}
}
