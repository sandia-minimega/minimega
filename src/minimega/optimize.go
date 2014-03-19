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
	"os/exec"
	"ranges"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

var (
	ksmPagesToScan     int
	ksmRun             int
	ksmSleepMillisecs  int
	ksmEnabled         bool
	affinityEnabled    bool
	affinityCPUSets    map[string][]*vmInfo
	hugepagesMountPath string
)

const (
	ksmPathRun            = "/sys/kernel/mm/ksm/run"
	ksmPathPagesToScan    = "/sys/kernel/mm/ksm/pages_to_scan"
	ksmPathSleepMillisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksmTunePagesToScan    = 100000
	ksmTuneSleepMillisecs = 10
)

func init() {
	affinityClearFilter()
}

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
	hugepagesMountPath = ""
	affinityDisable()
	affinityClearFilter()
}

func optimizeCLI(c cliCommand) cliResponse {
	// must be in the form of
	// 	optimize ksm [true,false]
	//	optimize hugepages <path>
	//	optimize affinity [true,false]
	switch len(c.Args) {
	case 0: // summary of all optimizations
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "Subsystem\tEnabled\n")
		fmt.Fprintf(w, "KSM\t%v\n", ksmEnabled)

		hugepagesEnabled := "false"
		if hugepagesMountPath != "" {
			hugepagesEnabled = fmt.Sprintf("true [%v]", hugepagesMountPath)
		}
		fmt.Fprintf(w, "hugepages\t%v\n", hugepagesEnabled)

		r, err := ranges.NewRange("", 0, runtime.NumCPU()-1)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("cpu affinity ranges: %v", err),
			}
		}

		var cpus []string
		for k, _ := range affinityCPUSets {
			cpus = append(cpus, k)
		}
		cpuRange, err := r.UnsplitRange(cpus)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("cannot compress CPU range: %v", err),
			}
		}

		if affinityEnabled {
			fmt.Fprintf(w, "CPU affinity\ttrue with cpus %v\n", cpuRange)
		} else {
			fmt.Fprintf(w, "CPU affinity\tfalse\n")
		}

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
				Response: fmt.Sprintf("%v", hugepagesMountPath),
			}
		case "affinity":
			var o bytes.Buffer
			w := new(tabwriter.Writer)
			w.Init(&o, 5, 0, 1, ' ', 0)
			fmt.Fprintf(w, "CPU\tVMs\n")

			var cpus []string
			for k, _ := range affinityCPUSets {
				cpus = append(cpus, k)
			}

			sort.Strings(cpus)

			for _, cpu := range cpus {
				var ids []int
				for _, vm := range affinityCPUSets[cpu] {
					ids = append(ids, vm.Id)
				}
				fmt.Fprintf(w, "%v\t%v\n", cpu, ids)
			}

			w.Flush()
			return cliResponse{
				Response: o.String(),
			}
		default:
			return cliResponse{
				Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
			}
		}
	case 2: // must be ksm, hugepages, affininy
		switch c.Args[0] {
		case "ksm":
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

			if set {
				ksmEnable()
			} else {
				ksmDisable()
			}
		case "hugepages":
			if c.Args[1] == `""` {
				hugepagesMountPath = ""
			} else {
				hugepagesMountPath = c.Args[1]
			}
		case "affinity":
			// must be:
			//	[true,false]
			switch strings.ToLower(c.Args[1]) {
			case "true":
				if !affinityEnabled {
					affinityEnable()
				}
			case "false":
				if affinityEnabled {
					affinityDisable()
				}
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
	case 3:
		// must be:
		//	affinity filter [...]
		//	affinity filter clear
		if c.Args[0] != "affinity" || c.Args[1] != "filter" {
			return cliResponse{
				Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
			}
		}

		if c.Args[2] == "clear" {
			affinityClearFilter()
			return cliResponse{}
		}

		r, err := ranges.NewRange("", 0, runtime.NumCPU()-1)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("cpu affinity ranges: %v", err),
			}
		}
		cpus, err := r.SplitRange(c.Args[2])
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("cannot expand CPU range: %v", err),
			}
		}

		affinityCPUSets = make(map[string][]*vmInfo)
		for _, v := range cpus {
			affinityCPUSets[v] = []*vmInfo{}
		}

		if affinityEnabled {
			affinityEnable()
		}
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command %v %v", c.Command, strings.Join(c.Args, " ")),
		}
	}
	return cliResponse{}
}

func affinityEnable() error {
	affinityEnabled = true
	for _, v := range vms.vms {
		cpu := affinitySelectCPU(v)
		err := v.AffinitySet(cpu)
		if err != nil {
			return err
		}
	}
	return nil
}

func affinityDisable() error {
	affinityEnabled = false
	for _, v := range vms.vms {
		affinityUnselectCPU(v)
		err := v.AffinityUnset()
		if err != nil {
			return err
		}
	}
	return nil
}

func affinityClearFilter() {
	cpu := runtime.NumCPU()
	affinityCPUSets = make(map[string][]*vmInfo)
	for i := 0; i < cpu; i++ {
		v := fmt.Sprintf("%v", i)
		affinityCPUSets[v] = []*vmInfo{}
	}
	if affinityEnabled {
		affinityEnable()
	}
}

func affinitySelectCPU(vm *vmInfo) string {
	// find a key with the fewest number of entries, add vm to it and
	// return the key
	var key string
	for k, v := range affinityCPUSets {
		if key == "" {
			key = k
			continue
		}
		if len(v) < len(affinityCPUSets[key]) {
			key = k
		}
	}
	if key == "" {
		log.Fatalln("could not find a valid CPU set!")
	}
	affinityCPUSets[key] = append(affinityCPUSets[key], vm)
	return key
}

func affinityUnselectCPU(vm *vmInfo) {
	// find and remove vm from its cpuset
	for k, v := range affinityCPUSets {
		for i, j := range v {
			if j.Id == vm.Id {
				if len(v) == 1 {
					affinityCPUSets[k] = []*vmInfo{}
				} else if i == 0 {
					affinityCPUSets[k] = v[1:]
				} else if i == len(v)-1 {
					affinityCPUSets[k] = v[:len(v)-1]
				} else {
					affinityCPUSets[k] = append(affinityCPUSets[k][:i], affinityCPUSets[k][i+1:]...)
				}
				return
			}
		}
	}
	log.Fatal("could not find vm %v in CPU set", vm.Id)
}

func (vm *vmInfo) CheckAffinity() {
	if affinityEnabled {
		cpu := affinitySelectCPU(vm)
		err := vm.AffinitySet(cpu)
		if err != nil {
			log.Errorln(err)
		}
	}
}

func (vm *vmInfo) AffinitySet(cpu string) error {
	log.Debugln("affinitySet")

	p := process("taskset")
	args := []string{p, "-a", "-p", fmt.Sprintf("%v", cpu), fmt.Sprintf("%v", vm.PID)}
	cmd := exec.Command(args[0], args[1:]...)
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	cmd.Stdout = &sOut
	cmd.Stderr = &sErr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%v : stdout: %v, stderr: %v", err, sOut.String(), sErr.String())
	}
	return nil
}

func (vm *vmInfo) AffinityUnset() error {
	log.Debugln("affinityUnset")

	p := process("taskset")
	args := []string{p, "-p", "0xffffffffffffffff", fmt.Sprintf("%v", vm.PID)}
	cmd := exec.Command(args[0], args[1:]...)
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	cmd.Stdout = &sOut
	cmd.Stderr = &sErr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%v : stdout: %v, stderr: %v", err, sOut.String(), sErr.String())
	}
	return nil
}
