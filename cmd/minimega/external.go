// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/nbd"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	MIN_QEMU    = []int{1, 6}
	MIN_DNSMASQ = []int{2, 73}
	MIN_OVS     = []int{1, 11}
	// MIN_KERNEL for Overlayfs
	MIN_KERNEL = []int{3, 18}

	// feature requirements
	MIN_QEMU_COPY_PASTE = []int{6, 1}
)

// externalDependencies contains all the external programs that minimega
// invokes. We check for the existence of these on startup and on `check`.
var externalDependencies = map[string]bool{
	"dnsmasq":   true, // used in multiple
	"kvm":       true, // used in multiple
	"mount":     true, // used in multiple
	"dhclient":  true, // used in bridge_cli.go
	"ip":        true, // used in bridge_cli.go
	"scp":       true, // used in deploy.go
	"ssh":       true, // used in deploy.go
	"cp":        true, // used in disk.go
	"qemu-img":  true, // used in disk.go
	"ntfs-3g":   true, // used in disk.go
	"blockdev":  true, // used in disk.go
	"ovs-vsctl": true, // used in external.go
	"taskset":   true, // used in optimize.go
	"tar":       true, // used in cli.go
}

func init() {
	// Add in dependencies from imported packages
	for _, v := range bridge.ExternalDependencies {
		externalDependencies[v] = true
	}

	for _, v := range nbd.ExternalDependencies {
		externalDependencies[v] = true
	}
}

var externalCLIHandlers = []minicli.Handler{
	{ // check
		HelpShort: "check that all external executables dependencies exist",
		HelpLong: `
minimega maintains a list of external packages that it depends on, such as
qemu. Calling check will attempt to find each of these executables in the
available path and check to make sure they meet the minimum version
requirements. Returns errors for all missing executables and all minimum
versions not met.`,
		Patterns: []string{
			"check",
		},
		Call: wrapSimpleCLI(func(_ *Namespace, _ *minicli.Command, _ *minicli.Response) error {
			return checkExternal()
		}),
	},
}

// checkExternal checks for the presence of each of the external processes we
// may call, and error if any aren't in our path.
func checkExternal() error {
	// make sure we're using a new enough kernel
	if err := checkVersion("kernel", MIN_KERNEL, kernelVersion); err != nil {
		return err
	}

	// make sure we have all binaries
	if err := checkDependencies(); err != nil {
		return err
	}

	// everything we want exists, but we have a few minimum versions to check
	if err := checkVersion("dnsmasq", MIN_DNSMASQ, dnsmasqVersion); err != nil {
		return err
	}
	if err := checkVersion("ovs-vsctl", MIN_OVS, ovsVersion); err != nil {
		return err
	}
	if err := checkVersion("qemu", MIN_QEMU, qemuVersion); err != nil {
		return err
	}

	// now check that ovs is actually running...
	if err := bridge.CheckOVS(); err != nil {
		return errors.New("openvswitch does not appear to be running")
	}

	// check kvm module is loaded
	if !lsModule("kvm") {
		// warn since not a hard requirement
		log.Warn("no kvm module detected, is virtualization enabled?")
	}

	// Android runtime is optional and only checked when Android appears configured
	if androidConfiguredAny() {
		for _, ns := range namespaces {
			cfg := ns.vmConfig.AndroidConfig
			if !androidConfigured(cfg) {
				continue
			}

			if err := checkAndroidDependencies(cfg); err != nil {
				log.Warn("android runtime unavailable: %v", err)
				break
			} else {
				log.Info("android runtime available")
				break
			}
		}
	}

	return nil
}

// checkDependencies checks whether each of the processes in
// externalDependencies exists or not
func checkDependencies() error {
	var errs []error

	for name := range externalDependencies {
		path, err := exec.LookPath(name)
		if err == nil {
			log.Info("%v found at: %v", name, path)
		}

		errs = append(errs, err)
	}

	return makeErrSlice(errs)
}

func kernelVersion() ([]int, error) {
	var utsname syscall.Utsname
	if err := syscall.Uname(&utsname); err != nil {
		return nil, fmt.Errorf("check kernel version failed: %v", err)
	}

	// convert []int8 to string so that we can call parseVersion on it
	buf := make([]byte, len(utsname.Release))
	for i, v := range utsname.Release {
		buf[i] = byte(v)
	}

	return parseVersion("kernel", string(buf))
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
	out, err := processWrapper("ovs-vsctl", "-V")
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
	return qemuVersionContext(context.Background())
}

func qemuVersionContext(ctx context.Context) ([]int, error) {
	out, err := processWrapperContext(ctx, "kvm", "-version")
	if err != nil {
		return nil, fmt.Errorf("check qemu version failed: %v", err)
	}

	return parseQemuVersion(out)
}

// parseQemuVersion parses the output of `kvm -version`, for example
// "QEMU emulator version 11.1.3 (Debian 1:11.1.3+ds-1)".
func parseQemuVersion(out string) ([]int, error) {
	f := strings.Fields(out)
	if len(f) < 4 {
		return nil, fmt.Errorf("cannot parse qemu version: %v", out)
	}

	return parseVersion("qemu", f[3])
}

// hostQemuProbeTimeout limits how long `host` waits for `kvm -version`.
const hostQemuProbeTimeout = 5 * time.Second

func hostQemuProbe() ([]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hostQemuProbeTimeout)
	defer cancel()

	return qemuVersionContext(ctx)
}

// versionCache keeps the first successful result of probe as a dotted version.
// Calls that overlap a running probe wait for it and share its result, also
// when it fails. A call that starts after a failed probe runs probe again.
type versionCache struct {
	mu      sync.Mutex
	version string
	call    *versionCall
	probe   func() ([]int, error)
	// joined, if set, runs when a call waits for the running probe
	joined func()
}

// versionCall is a running probe. done closes when version is set.
type versionCall struct {
	done    chan struct{}
	version string
}

func (c *versionCache) get() string {
	c.mu.Lock()
	if c.version != "" {
		defer c.mu.Unlock()
		return c.version
	}

	if call := c.call; call != nil {
		if c.joined != nil {
			c.joined()
		}
		c.mu.Unlock()
		<-call.done
		return call.version
	}

	call := &versionCall{done: make(chan struct{})}
	c.call = call
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.version = call.version
		c.call = nil
		c.mu.Unlock()
		close(call.done)
	}()

	v, err := c.probe()
	if err != nil {
		log.Debug("unable to determine version: %v", err)
		return ""
	}

	call.version = printVersion(v)
	return call.version
}

var hostQemu = &versionCache{probe: hostQemuProbe}

// hostQemuVersion returns the dotted QEMU version from `kvm -version`, or an
// empty string when the probe fails or times out.
func hostQemuVersion() string {
	return hostQemu.get()
}

// parseVersion parses a version string like 1.2.3, returning a slice of ints
func parseVersion(name, version string) ([]int, error) {
	var res []int

	for _, v := range strings.Split(version, ".") {
		i, err := strconv.Atoi(v)
		if err != nil {
			// if the string contains non-numeric characters, trim those and
			// then immediately return the result, if we have a valid number
			for i, r := range v {
				if r < '0' || r > '9' {
					v = v[:i]
					break
				}
			}

			if i, err := strconv.Atoi(v); err == nil {
				res = append(res, i)
				return res, nil
			}

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

	got := printVersion(version)
	want := printVersion(min)

	log.Info("%v version: %v, minimum: %v", name, got, want)

	for i := range min {
		if i >= len(version) || version[i] < min[i] {
			// minimum version was more specific (e.g. 1.1.1 against 1.1) or
			// minimum version is greater in the current index => fail
			return fmt.Errorf("%v version does not meet minimum: %v < %v", name, got, want)
		} else if version[i] > min[i] {
			// version exceeds minimum
			break
		}
	}

	// must match or exceed
	return nil
}

// checks that qemu has the chardev `required`
func checkQemuChardev(required string) error {
	out, err := processWrapper("kvm", "-chardev", "help")
	if err != nil {
		return fmt.Errorf("check qemu chardev failed: %v", err)
	}

	fields := strings.Split(out, "\n")
	for _, f := range fields {
		if strings.TrimSpace(f) == required {
			return nil
		}
	}
	return fmt.Errorf("qemu does not have required chardev: %v", required)
}

// lsModule returns true if the specified module is in the `lsmod` output
func lsModule(s string) bool {
	log.Info("checking for kernel module: %v", s)

	out, err := processWrapper("lsmod")
	if err != nil {
		log.Warn("unable to check lsmod for %v: %v", s, err)
		return false
	}

	return strings.Contains(out, s)
}

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
func processWrapper(args ...string) (string, error) {
	return processWrapperContext(context.Background(), args...)
}

// processWrapperContext is processWrapper with a context. When ctx ends,
// processWrapperContext kills the process and returns an error.
func processWrapperContext(ctx context.Context, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty argument list")
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if ctx.Done() != nil {
		// a child that keeps the output pipe open must not block the return
		cmd.WaitDelay = time.Second
	}
	out, err := cmd.CombinedOutput()
	stop := time.Now()
	log.Debug("cmd %v completed in %v", args[0], stop.Sub(start))

	return string(out), err
}

func process(s string) (string, error) {
	p, err := exec.LookPath(s)
	if err == exec.ErrNotFound {
		// add executable name to error
		return "", fmt.Errorf("executable not found in $PATH: %v", s)
	}

	return p, err
}
