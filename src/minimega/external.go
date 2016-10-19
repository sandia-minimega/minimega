// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	MIN_QEMU    = []int{1, 6}
	MIN_DNSMASQ = []int{2, 73}
	MIN_OVS     = []int{1, 11}
)

// externalProcessesLock mediates access to customExternalProcesses.
var externalProcessesLock sync.Mutex

// defaultExternalProcesses is the default mapping between a command and the
// actual binary name. This should *never* be modified. If the user needs to
// update customExternalProcesses.
var defaultExternalProcesses = map[string]string{
	"qemu":     "kvm",
	"ip":       "ip",
	"ovs":      "ovs-vsctl",
	"dnsmasq":  "dnsmasq",
	"kill":     "kill",
	"dhcp":     "dhclient",
	"openflow": "ovs-ofctl",
	"mount":    "mount",
	"umount":   "umount",
	"mkdosfs":  "mkdosfs",
	"qemu-nbd": "qemu-nbd",
	"rm":       "rm",
	"qemu-img": "qemu-img",
	"cp":       "cp",
	"taskset":  "taskset",
	"lsmod":    "lsmod",
	"ntfs-3g":  "ntfs-3g",
	"scp":      "scp",
	"ssh":      "ssh",
	"hostname": "hostname",
	"tc":       "tc",
}

// customExternalProcesses contains user-specified mappings between command
// names. This mapping is checked first before using defaultExternalProcesses
// to resolve a command.
var customExternalProcesses = map[string]string{}

var externalCLIHandlers = []minicli.Handler{
	{ // check
		HelpShort: "check that all external executables dependencies exist",
		HelpLong: `
minimega maintains a list of external packages that it depends on, such as
qemu. Calling check will attempt to find each of these executables in the
avaiable path and check to make sure they meet the minimum version
requirements. Returns errors for all missing executables and all minimum
versions not met.`,
		Patterns: []string{
			"check",
		},
		Call: wrapSimpleCLI(cliCheckExternal),
	},
}

func cliCheckExternal(c *minicli.Command, resp *minicli.Response) error {
	if err := checkExternal(); err != nil {
		return err
	}

	return nil
}

// checkExternal checks for the presence of each of the external processes we
// may call, and error if any aren't in our path.
func checkExternal() error {
	// make sure we have all binaries first
	if err := checkProcesses(); err != nil {
		return err
	}

	// everything we want exists, but we have a few minimum versions to check
	if err := checkVersion("dnsmasq", MIN_DNSMASQ, dnsmasqVersion); err != nil {
		return err
	}
	if err := checkVersion("ovs", MIN_OVS, ovsVersion); err != nil {
		return err
	}
	if err := checkVersion("qemu", MIN_QEMU, qemuVersion); err != nil {
		return err
	}

	return nil
}

// checkProcesses checks each of the processes in defaultExternalProcesses exists
func checkProcesses() error {
	externalProcessesLock.Lock()
	defer externalProcessesLock.Unlock()

	var errs []string
	for name, proc := range defaultExternalProcesses {
		if alt, ok := customExternalProcesses[name]; ok {
			proc = alt
		}

		path, err := exec.LookPath(proc)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			log.Info("%v found at: %v", proc, path)
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}

	return nil
}

func dnsmasqVersion() ([]int, error) {
	out, err := processWrapper("dnsmasq", "-v")
	if err != nil {
		return nil, fmt.Errorf("check dnsmasq version failed: %v", err)
	}

	f := strings.Fields(out)
	if len(f) < 3 {
		return nil, fmt.Errorf("cannot parse dnsmasq version: %v", out)
	}

	return parseVersion("dnsmasq", f[2])
}

func ovsVersion() ([]int, error) {
	out, err := processWrapper("ovs", "-V")
	if err != nil {
		return nil, fmt.Errorf("check ovs version failed: %v", err)
	}

	f := strings.Fields(out)
	if len(f) < 4 {
		return nil, fmt.Errorf("cannot parse ovs version: %v", out)
	}

	return parseVersion("ovs", f[3])
}

func qemuVersion() ([]int, error) {
	out, err := processWrapper("qemu", "-version")
	if err != nil {
		return nil, fmt.Errorf("check qemu version failed: %v", err)
	}

	f := strings.Fields(out)
	if len(f) < 4 {
		return nil, fmt.Errorf("cannot parse qemu version: %v", out)
	}

	return parseVersion("qemu", f[3])
}

// parseVersion parses a version string like 1.2.3, returning a slice of ints
func parseVersion(name, version string) ([]int, error) {
	var res []int

	for _, v := range strings.Split(version, ".") {
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("cannot parse %v version: %v", name, version)
		}

		res = append(res, i)
	}

	return res, nil
}

// printVersion joins a slice of ints with dots to produce a version string
func printVersion(version []int) string {
	var res []string
	for _, v := range version {
		res = append(res, strconv.Itoa(v))
	}

	return strings.Join(res, ".")
}

// checkVersion compares the return value of versionFn against min, returning
// an error if the version is less than min or versionFn failed.
func checkVersion(name string, min []int, versionFn func() ([]int, error)) error {
	version, err := versionFn()
	if err != nil {
		return err
	}

	log.Debug("%v version: %v", name, printVersion(version))

	for i := range min {
		if i >= len(version) || version[i] < min[i] {
			// minimum version was more specific (e.g. 1.1.1 against 1.1) or
			// minimum version is greater in the current index => fail
			got := printVersion(version)
			want := printVersion(min)
			return fmt.Errorf("%v version does not meet minimum: %v < %v", name, got, want)
		} else if version[i] > min[i] {
			// version exceeds minimum
			break
		}
	}

	// must match or exceed
	return nil
}

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
// Users that need runtime control of processes should use os/exec directly.
func processWrapper(args ...string) (string, error) {
	a := append([]string{}, args...)
	if len(a) == 0 {
		return "", fmt.Errorf("empty argument list")
	}
	p := process(a[0])
	if p == "" {
		return "", fmt.Errorf("cannot find process %v", args[0])
	}

	a[0] = p
	var ea []string
	if len(a) > 1 {
		ea = a[1:]
	}

	start := time.Now()
	out, err := exec.Command(p, ea...).CombinedOutput()
	stop := time.Now()
	log.Debug("cmd %v completed in %v", p, stop.Sub(start))
	return string(out), err
}

func process(p string) string {
	externalProcessesLock.Lock()
	defer externalProcessesLock.Unlock()

	name, ok := customExternalProcesses[p]
	if !ok {
		name = defaultExternalProcesses[p]
	}

	path, err := exec.LookPath(name)
	if err != nil {
		log.Error("process: %v", err)
		return ""
	}
	return path
}
