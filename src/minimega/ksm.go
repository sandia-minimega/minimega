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
	log "minilog"
	"os"
	"strconv"
	"io/ioutil"
	"strings"
)

var (
	ksm_pages_to_scan   int
	ksm_run             int
	ksm_sleep_millisecs int
)

const (
	ksm_path_run             = "/sys/kernel/mm/ksm/run"
	ksm_path_pages_to_scan   = "/sys/kernel/mm/ksm/pages_to_scan"
	ksm_path_sleep_millisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksm_tune_pages_to_scan   = 100000
	ksm_tune_sleep_millisecs = 10
)

func ksm_save() {
	log.Info("saving ksm values")
	ksm_run = ksm_get_int_from_file(ksm_path_run)
	ksm_pages_to_scan = ksm_get_int_from_file(ksm_path_pages_to_scan)
	ksm_sleep_millisecs = ksm_get_int_from_file(ksm_path_sleep_millisecs)
}

func ksm_get_int_from_file(filename string) int {
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

func ksm_enable() {
	log.Info("enabling ksm")
	ksm_write(ksm_path_run, 1)
	ksm_write(ksm_path_pages_to_scan, ksm_tune_pages_to_scan)
	ksm_write(ksm_path_sleep_millisecs, ksm_tune_sleep_millisecs)
}

func ksm_restore() {
	log.Info("restoring ksm values")
	ksm_write(ksm_path_run, ksm_run)
	ksm_write(ksm_path_pages_to_scan, ksm_pages_to_scan)
	ksm_write(ksm_path_sleep_millisecs, ksm_sleep_millisecs)
}

func ksm_write(filename string, value int) {
	file, err := os.Create(filename)
	if err != nil {
		log.Error("%v", err)
		return
	}
	defer file.Close()
	log.Info("writing %v to %v", value, filename)
	file.WriteString(strconv.Itoa(value))
}
