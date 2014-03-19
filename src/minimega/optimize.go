// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

var (
	ksmPagesToScan    int
	ksmRun            int
	ksmSleepMillisecs int
	ksmEnabled        bool
	hugepagesEnabled  bool
	affinityEnabled   bool
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
		log.Debugln("enabling ksm")
		ksmWrite(ksmPathRun, 1)
		ksmWrite(ksmPathPagesToScan, ksmTunePagesToScan)
		ksmWrite(ksmPathSleepMillisecs, ksmTuneSleepMillisecs)
		ksmEnabled = true
	}
}

func ksmDisable() {
	if ksmEnabled {
		log.Debugln("restoring ksm values")
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

func clearOptimize() {
	ksmDisable()
}

func optimizeCLI(c cliCommand) cliResponse {
	// must be in the form of
	// 	optimize ksm [true,false]
	//	optimize hugepages [true,false]
	//	optimize affinity [true,false]
	switch len(c.Args) {
	case 0: // summary of all optimizations
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "Subsystem\tEnabled\n")
		fmt.Fprintf(w, "KSM\t%v\n", ksmEnabled)
		fmt.Fprintf(w, "hugepages\t%v\n", hugepagesEnabled)
		fmt.Fprintf(w, "CPU affinity\t%v\n", affinityEnabled)
		w.Flush()
		return cliResponse{
			Response: o.String(),
		}
	case 1: // must be ksm, hugepages, affinity
		switch c.Args[0] {
		case "ksm":
			return cliResponse{
				Response: fmt.Sprintf("%v", ksmEnabled),
			}
		case "hugepages":
			return cliResponse{
				Response: fmt.Sprintf("%v", hugepagesEnabled),
			}
		case "affinity":
			return cliResponse{
				Response: fmt.Sprintf("%v", affinityEnabled),
			}
		default:
			return cliResponse{
				Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
			}
		}
	case 2: // must be ksm, hugepages, affinity, with [true,false]
		var set bool
		switch strings.ToLower(c.Args[1]) {
		case "true":
			set = true
		case "false":
			set = false
		default:
			return cliResponse{
				Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
			}
		}

		switch c.Args[0] {
		case "ksm":
			if set {
				ksmEnable()
			} else {
				ksmDisable()
			}
		case "hugepages":
		case "affinity":
		default:
			return cliResponse{
				Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
			}
		}
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
		}
	}
	return cliResponse{}
}
