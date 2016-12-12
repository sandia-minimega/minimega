// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
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
	affinityCPUSets    map[string][]*KvmVM
	hugepagesMountPath string
)

const (
	ksmPathRun            = "/sys/kernel/mm/ksm/run"
	ksmPathPagesToScan    = "/sys/kernel/mm/ksm/pages_to_scan"
	ksmPathSleepMillisecs = "/sys/kernel/mm/ksm/sleep_millisecs"
	ksmTunePagesToScan    = 100000
	ksmTuneSleepMillisecs = 10
)

var optimizeCLIHandlers = []minicli.Handler{
	{ // optimize
		HelpShort: "enable or disable several virtualization optimizations",
		HelpLong: `
Enable or disable several virtualization optimizations, including Kernel
Samepage Merging, CPU affinity for VMs, and the use of hugepages.

To enable/disable Kernel Samepage Merging (KSM):
	optimize ksm [true,false]

To enable hugepage support:
	optimize hugepages </path/to/hugepages_mount>

To disable hugepage support:
	clear optimize hugepages

To enable/disable CPU affinity support:
	optimize affinity [true,false]

To set a CPU set filter for the affinity scheduler, for example (to use only
CPUs 1, 2-20):
	optimize affinity filter [1,2-20]

To clear a CPU set filter:
	clear optimize affinity filter

To view current CPU affinity mappings:
	optimize affinity

To disable all optimizations see "clear optimize".`,
		Patterns: []string{
			"optimize",
			"optimize <affinity,> <filter,> <filter>",
			"optimize <affinity,> [true,false]",
			"optimize <hugepages,> [path]",
			"optimize <ksm,> [true,false]",
		},
		Call: wrapSimpleCLI(cliOptimize),
	},
	{ // clear optimize
		HelpShort: "reset virtualization optimization state",
		HelpLong: `
Resets state for virtualization optimizations. See "help optimize" for more
information.`,
		Patterns: []string{
			"clear optimize",
			"clear optimize <affinity,> [filter,]",
			"clear optimize <hugepages,>",
			"clear optimize <ksm,>",
		},
		Call: wrapSimpleCLI(cliOptimizeClear),
	},
}

func init() {
	affinityClearFilter()
}

func cliOptimize(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["ksm"] {
		if len(c.BoolArgs) == 1 {
			// Must want to print ksm status
			resp.Response = fmt.Sprintf("%v", ksmEnabled)
		} else if c.BoolArgs["true"] {
			// Must want to update ksm status to true
			ksmEnable()
		} else {
			// Must want to update ksm status to false
			ksmDisable()
		}

		return nil
	} else if c.BoolArgs["hugepages"] {
		if len(c.BoolArgs) == 1 {
			// Must want to print hugepage path
			resp.Response = fmt.Sprintf("%v", hugepagesMountPath)
		} else {
			hugepagesMountPath = c.StringArgs["path"]
		}

		return nil
	} else if c.BoolArgs["affinity"] {
		if len(c.BoolArgs) == 1 {
			// Must want to print affinity status
			resp.Header = []string{"cpu", "vms"}
			resp.Tabular = [][]string{}

			var cpus []string
			for k, _ := range affinityCPUSets {
				cpus = append(cpus, k)
			}

			sort.Strings(cpus)

			for _, cpu := range cpus {
				var ids []int
				for _, vm := range affinityCPUSets[cpu] {
					ids = append(ids, vm.GetID())
				}
				resp.Tabular = append(resp.Tabular, []string{
					cpu,
					fmt.Sprintf("%v", ids)})
			}
		} else if c.BoolArgs["filter"] {
			r, err := ranges.NewRange("", 0, runtime.NumCPU()-1)
			if err != nil {
				return fmt.Errorf("cpu affinity ranges: %v", err)
			}

			cpus, err := r.SplitRange(c.StringArgs["filter"])
			if err != nil {
				return fmt.Errorf("cannot expand CPU range: %v", err)
			}

			affinityCPUSets = make(map[string][]*KvmVM)
			for _, v := range cpus {
				affinityCPUSets[v] = []*KvmVM{}
			}

			if affinityEnabled {
				affinityEnable()
			}
		} else if c.BoolArgs["true"] && !affinityEnabled {
			// Enabling affinity
			affinityEnable()
		} else if c.BoolArgs["false"] && affinityEnabled {
			// Disabling affinity
			affinityDisable()
		}

		return nil
	}

	// Summary of optimizations
	out, err := optimizeStatus()
	if err == nil {
		resp.Response = out
	}

	return err
}

func cliOptimizeClear(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["affinity"] && c.BoolArgs["filter"] {
		// Reset affinity filter
		affinityClearFilter()
	} else if c.BoolArgs["affinity"] {
		// Reset affinity (disable)
		affinityDisable()
	} else if c.BoolArgs["hugepages"] {
		// Reset hugepages (disable)
		hugepagesMountPath = ""
	} else if c.BoolArgs["ksm"] {
		ksmDisable()
	} else {
		clearOptimize()
	}

	return nil
}

// TODO: Rewrite this to use Header/Tabular.
func optimizeStatus() (string, error) {
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
		return "", fmt.Errorf("cpu affinity ranges: %v", err)
	}

	var cpus []string
	for k, _ := range affinityCPUSets {
		cpus = append(cpus, k)
	}
	cpuRange, err := r.UnsplitRange(cpus)
	if err != nil {
		return "", fmt.Errorf("cannot compress CPU range: %v", err)
	}

	if affinityEnabled {
		fmt.Fprintf(w, "CPU affinity\ttrue with cpus %v\n", cpuRange)
	} else {
		fmt.Fprintf(w, "CPU affinity\tfalse\n")
	}

	w.Flush()
	return o.String(), nil
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
		log.Errorln(err)
		teardown()
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

func affinityEnable() error {
	affinityEnabled = true
	for _, vm := range vms.FindKvmVMs() {
		cpu := affinitySelectCPU(vm)
		err := vm.AffinitySet(cpu)
		if err != nil {
			return err
		}
	}
	return nil
}

func affinityDisable() error {
	affinityEnabled = false
	for _, vm := range vms.FindKvmVMs() {
		affinityUnselectCPU(vm)
		err := vm.AffinityUnset()
		if err != nil {
			return err
		}
	}
	return nil
}

func affinityClearFilter() {
	cpu := runtime.NumCPU()
	affinityCPUSets = make(map[string][]*KvmVM)
	for i := 0; i < cpu; i++ {
		v := fmt.Sprintf("%v", i)
		affinityCPUSets[v] = []*KvmVM{}
	}
	if affinityEnabled {
		affinityEnable()
	}
}

func affinitySelectCPU(vm *KvmVM) string {
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

func affinityUnselectCPU(vm *KvmVM) {
	// find and remove vm from its cpuset
	for k, v := range affinityCPUSets {
		for i, j := range v {
			if j.GetID() == vm.GetID() {
				if len(v) == 1 {
					affinityCPUSets[k] = []*KvmVM{}
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
	log.Fatal("could not find vm %v in CPU set", vm.GetID())
}

func (vm *KvmVM) CheckAffinity() {
	if affinityEnabled {
		cpu := affinitySelectCPU(vm)
		err := vm.AffinitySet(cpu)
		if err != nil {
			log.Error("AffinitySet: %v", err)
		}
	}
}

func (vm *KvmVM) AffinitySet(cpu string) error {
	log.Debugln("affinitySet")

	out, err := processWrapper("taskset", "-a", "-p", fmt.Sprintf("%v", cpu), fmt.Sprintf("%v", vm.pid))
	if err != nil {
		return fmt.Errorf("%v: %v", err, out)
	}
	return nil
}

func (vm *KvmVM) AffinityUnset() error {
	log.Debugln("affinityUnset")

	out, err := processWrapper("taskset", "-p", "0xffffffffffffffff", fmt.Sprintf("%v", vm.pid))
	if err != nil {
		return fmt.Errorf("%v: %v", err, out)
	}
	return nil
}
