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
)

const (
	ksmPathRun            = "/sys/kernel/mm/ksm/run"
	ksmPathPagesToScan    = "/sys/kernel/mm/ksm/pages_to_scan"
	ksmPathSleepMillisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksmTunePagesToScan    = 100000
	ksmTuneSleepMillisecs = 10
)

func ksmSave() {
	log.Info("saving ksm values")
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
	log.Info("enabling ksm")
	ksmWrite(ksmPathRun, 1)
	ksmWrite(ksmPathPagesToScan, ksmTunePagesToScan)
	ksmWrite(ksmPathSleepMillisecs, ksmTuneSleepMillisecs)
}

func ksmRestore() {
	log.Info("restoring ksm values")
	ksmWrite(ksmPathRun, ksmRun)
	ksmWrite(ksmPathPagesToScan, ksmPagesToScan)
	ksmWrite(ksmPathSleepMillisecs, ksmSleepMillisecs)
}

func ksmWrite(filename string, value int) {
	file, err := os.Create(filename)
	if err != nil {
		log.Error("%v", err)
		return
	}
	defer file.Close()
	log.Info("writing %v to %v", value, filename)
	file.WriteString(strconv.Itoa(value))
}
