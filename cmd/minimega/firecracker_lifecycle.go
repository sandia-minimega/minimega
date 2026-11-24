// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	fcMaxVCPUs = 32

	fcMaxDevicesMMIO = 17
	fcMaxDevicesPCI  = 31

	// fcMaxCmdline is the x86_64 kernel command line size, with the NUL.
	fcMaxCmdline = 2048
	// fcCmdlinePCIOff is the size of the " pci=off" that Firecracker adds.
	fcCmdlinePCIOff = 8
	// fcCmdlinePerDevice is about the size of one virtio_mmio.device= entry.
	fcCmdlinePerDevice = 30

	// fcMaxHostname is the size limit of a Linux host name.
	fcMaxHostname = 64

	// fcConsoleMaxBytes is the default size limit of console.log.
	fcConsoleMaxBytes = 16 << 20

	// fcLogMaxBytes is the size limit of firecracker.log.
	fcLogMaxBytes = 4 << 20

	// fcDrainTimeout is how long Stop waits for a reader to empty its pipe.
	fcDrainTimeout = 500 * time.Millisecond

	fcBeaconMessage = "the miniature beacons of gondor have been lit, it's a minifire!"
)

// fcValidateConfig checks that cfg can launch as Firecracker VM name. It
// returns one warning for each setting that Firecracker ignores.
func fcValidateConfig(name string, cfg VMConfig) (warnings []string, err error) {
	base, kvm, fc := cfg.BaseConfig, cfg.KVMConfig, cfg.FirecrackerConfig

	var errs []string
	fail := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if kvm.KernelPath == "" {
		fail("firecracker VMs need a kernel: set vm config kernel")
	}
	if base.VCPUs < 1 || base.VCPUs > fcMaxVCPUs {
		fail("firecracker VMs need 1 to %d vCPUs, not %d", fcMaxVCPUs, base.VCPUs)
	}
	if kvm.CdromPath != "" {
		fail("firecracker VMs do not support a CD-ROM: clear vm config cdrom")
	}
	if kvm.SerialPorts > 0 {
		fail("firecracker VMs do not support serial ports: clear vm config serial-ports")
	}
	if kvm.VirtioPorts != "" {
		fail("firecracker VMs do not support virtio ports: clear vm config virtio-ports")
	}
	if kvm.MigratePath != "" {
		fail("firecracker VMs do not support vm config state: use firecracker-snapshot")
	}
	for i, nic := range base.Networks {
		if nic.QinQ {
			fail("firecracker VMs do not support QinQ: NIC %d", i)
		}
	}
	if len(base.Bonds) > 0 {
		fail("firecracker VMs do not support bonds")
	}
	if err := fcCheckInjects(base.Snapshot, fc.Injects); err != nil {
		fail("%v", err)
	}
	if len(fc.Injects) > 0 && len(kvm.Disks) > 0 && kvm.Disks[0].ReadOnly {
		fail("vm config inject needs a writable disk 0: disk 0 has option ro")
	}
	if _, err := fcParseSize(fc.FirecrackerScratch); err != nil {
		fail("firecracker-scratch: %v", err)
	}
	if fc.FirecrackerClone && fc.FirecrackerSnapshotPath != "" {
		if !base.Backchannel {
			fail("firecracker-clone needs vm config backchannel true: minimega sets the identity of a clone through cc")
		}
		if len(name) > fcMaxHostname {
			fail("firecracker-clone sets the VM name as the guest host name, but VM name %v is %d bytes; the limit is %d bytes", name, len(name), fcMaxHostname)
		}
	}

	if n, limit, detail := fcDeviceCount(cfg); n > limit {
		with := "without"
		if fc.FirecrackerPCI {
			with = "with"
		}
		fail("VM %v needs %d devices (%v); the limit is %d %v firecracker-pci", name, n, detail, limit, with)
	}

	ignored := []struct {
		set bool
		key string
	}{
		{kvm.QemuPath != "kvm", "qemu"},
		{kvm.CPU != "host", "cpu"},
		{kvm.Sockets != 0, "sockets"},
		{kvm.Cores != 0, "cores"},
		{kvm.Threads != 0, "threads"},
		{kvm.Machine != "", "machine"},
		{kvm.Baremetal, "baremetal"},
		{kvm.BaremetalNetworkDriver != "", "baremetal-network-driver"},
		{kvm.Vga != "std", "vga"},
		{!kvm.UsbUseXHCI, "usb-use-xhci"},
		{kvm.TpmSocketPath != "", "tpm-socket"},
		{kvm.BidirectionalCopyPaste, "bidirectional-copy-paste"},
		{len(kvm.QemuAppend) > 0, "qemu-append"},
		{len(kvm.QemuOverride) > 0, "qemu-override"},
	}
	for _, v := range ignored {
		if v.set {
			warnings = append(warnings, fmt.Sprintf("VM %v: firecracker ignores vm config %v", name, v.key))
		}
	}

	for i, d := range kvm.Disks {
		if d.Interface != "" && d.Interface != "virtio" {
			warnings = append(warnings, fmt.Sprintf("VM %v: firecracker always uses virtio-blk: ignoring interface %v of disk %d", name, d.Interface, i))
		}
	}
	if fc.FirecrackerClone && fc.FirecrackerSnapshotPath == "" {
		warnings = append(warnings, fmt.Sprintf("VM %v: firecracker-clone has no effect without firecracker-snapshot", name))
	}
	for i, nic := range base.Networks {
		if nic.Driver != "" && nic.Driver != DefaultKVMDriver && !strings.HasPrefix(nic.Driver, "virtio") {
			warnings = append(warnings, fmt.Sprintf("VM %v: firecracker always uses virtio-net: ignoring driver %v of NIC %d", name, nic.Driver, i))
		}
	}

	if len(errs) > 0 {
		return warnings, errors.New(strings.Join(errs, "; "))
	}

	return warnings, nil
}

// fcDeviceCount returns the number of devices that cfg needs, the device
// limit, and a list of the devices.
func fcDeviceCount(cfg VMConfig) (n, limit int, detail string) {
	disks := len(cfg.KVMConfig.Disks)
	nics := len(cfg.BaseConfig.Networks)

	parts := []string{fcPlural(disks, "disk")}
	n = disks + nics

	if size, _ := fcParseSize(cfg.FirecrackerConfig.FirecrackerScratch); size > 0 {
		n++
		parts = append(parts, "scratch")
	}
	parts = append(parts, fcPlural(nics, "NIC"))

	if cfg.BaseConfig.Backchannel {
		n++
		parts = append(parts, "vsock")
	}
	if cfg.FirecrackerConfig.FirecrackerEntropy {
		n++
		parts = append(parts, "entropy")
	}

	limit = fcMaxDevicesMMIO
	if cfg.FirecrackerConfig.FirecrackerPCI {
		limit = fcMaxDevicesPCI
	}

	return n, limit, strings.Join(parts, ", ")
}

func fcPlural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %v", noun)
	}
	return fmt.Sprintf("%d %vs", n, noun)
}

// fcBootArgs returns the boot_args for a VM with numDisks disks and
// numDevices devices, and the index of the disk that is the root device (-1
// for none).
func fcBootArgs(appendArgs []string, numDisks, numDevices int, pci bool) (args string, rootIndex int, err error) {
	fields := strings.Fields(strings.Join(appendArgs, " "))

	keys := map[string]bool{}
	var mode bool
	for _, f := range fields {
		k, v, _ := strings.Cut(f, "=")
		if k == "reboot" && v != "k" {
			return "", 0, fmt.Errorf("unsupported kernel argument %v: firecracker VMs need reboot=k", f)
		}
		if f == "ro" || f == "rw" {
			mode = true
		}
		keys[k] = true
	}

	for _, d := range []string{"console=ttyS0", "reboot=k", "panic=1"} {
		if k, _, _ := strings.Cut(d, "="); !keys[k] {
			fields = append(fields, d)
		}
	}

	rootIndex = 0
	if keys["root"] {
		rootIndex = -1
		if !mode {
			fields = append(fields, "rw")
		}
	} else if numDisks == 0 {
		rootIndex = -1
	}

	args = strings.Join(fields, " ")

	extra := fcCmdlinePerDevice * numDevices
	if !pci {
		extra += fcCmdlinePCIOff
	}
	if total := len(args) + extra; total >= fcMaxCmdline {
		return "", 0, fmt.Errorf("kernel command line is %d bytes, with about %d bytes that firecracker adds; the limit is %d bytes", total, extra, fcMaxCmdline-1)
	}

	return args, rootIndex, nil
}

// fcMakeConsoleFIFO creates a named pipe that the VMM, which runs as uid,
// writes the serial console or its log to.
func fcMakeConsoleFIFO(path string, uid int) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := unix.Mkfifo(path, 0600); err != nil {
		return fmt.Errorf("mkfifo %v: %w", path, err)
	}

	if err := fcChown(path, uid, uid); err != nil {
		return fmt.Errorf("chown %v: %v", path, err)
	}

	return nil
}

// fcConsole copies the serial console or the VMM log of a VM from a named
// pipe to a log file with a size limit.
type fcConsole struct {
	// Label names the output in the truncation line. Empty is "console log".
	Label string

	mu   sync.Mutex
	in   *os.File
	done chan struct{}
}

// Start rotates logPath to logPath.1, and copies fifo to a new logPath until
// Stop. It writes at most maxBytes of console output, then drains fifo.
func (c *fcConsole) Start(fifo, logPath string, maxBytes int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.in != nil {
		return errors.New("console reader is already running")
	}

	if err := os.Rename(logPath, logPath+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rotate %v: %v", logPath, err)
	}

	out, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	// O_RDWR keeps a writer open, so a VMM that closes the pipe gives no EOF.
	in, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		out.Close()
		return err
	}

	label := c.Label
	if label == "" {
		label = "console log"
	}

	done := make(chan struct{})
	c.in, c.done = in, done

	go func() {
		defer close(done)
		defer out.Close()

		fcCopyCapped(out, in, maxBytes, label)
	}()

	return nil
}

// Stop waits until the pipe is empty, closes it, and waits for the copy to
// end.
func (c *fcConsole) Stop() {
	c.mu.Lock()
	in, done := c.in, c.done
	c.in, c.done = nil, nil
	c.mu.Unlock()

	if in == nil {
		return
	}

	fcDrain(in, done, fcDrainTimeout)
	in.Close()
	<-done
}

// fcDrain returns when in has no unread bytes, done is closed, or the timeout
// passes.
func fcDrain(in *os.File, done <-chan struct{}, timeout time.Duration) {
	rc, err := in.SyscallConn()
	if err != nil {
		return
	}

	deadline := time.Now().Add(timeout)
	for {
		// TIOCINQ is FIONREAD on Linux.
		var n int
		if err := rc.Control(func(fd uintptr) {
			n, _ = unix.IoctlGetInt(int(fd), unix.TIOCINQ)
		}); err != nil || n <= 0 || time.Now().After(deadline) {
			return
		}

		select {
		case <-done:
			return
		case <-time.After(time.Millisecond):
		}
	}
}

// fcCopyConsole is fcCopyCapped for the serial console.
func fcCopyConsole(w io.Writer, r io.Reader, maxBytes int64) {
	fcCopyCapped(w, r, maxBytes, "console log")
}

// fcCopyCapped copies r to w until r fails. After maxBytes, it writes one
// truncation line that names label and discards the rest.
func fcCopyCapped(w io.Writer, r io.Reader, maxBytes int64, label string) {
	buf := make([]byte, 32<<10)

	var written int64
	var stopped bool
	last := byte('\n')

	for {
		n, err := r.Read(buf)
		if n > 0 && !stopped {
			p := buf[:n]
			if rem := maxBytes - written; int64(len(p)) > rem {
				p = p[:rem]
				stopped = true
			}

			if len(p) > 0 {
				if _, err := w.Write(p); err != nil {
					stopped = true
				}
				written += int64(len(p))
				last = p[len(p)-1]
			}

			if stopped {
				sep := ""
				if last != '\n' {
					sep = "\n"
				}
				fmt.Fprintf(w, "%v[%v truncated at %d bytes]\n", sep, label, maxBytes)
			}
		}

		if err != nil {
			return
		}
	}
}

// fcTail returns the last n bytes of the file at path.
func fcTail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if n <= 0 {
		return []byte{}, nil
	}

	if off := fi.Size() - n; off > 0 {
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
	}

	return io.ReadAll(f)
}

// fcClassifyExit returns the state of a VM after its VMM exits, and an error
// message for VM_ERROR. killed is true when vm kill sent the signal.
func fcClassifyExit(state *os.ProcessState, waitErr error, killed bool, consoleTail []byte, fcLogTail []byte) (VMState, string) {
	if killed {
		return VM_QUIT, ""
	}

	if state == nil {
		if waitErr == nil {
			waitErr = errors.New("no exit status")
		}
		return VM_ERROR, fmt.Sprintf("wait for firecracker: %v", waitErr)
	}

	if state.Exited() && state.ExitCode() == 0 {
		if bytes.Contains(consoleTail, []byte("Kernel panic")) {
			return VM_ERROR, "guest kernel panic"
		}
		return VM_QUIT, ""
	}

	msg := fmt.Sprintf("firecracker exited with %v", state)
	if lines := fcLastLines(fcLogTail, 5); len(lines) > 0 {
		msg += ": " + strings.Join(lines, "; ")
	}

	return VM_ERROR, msg
}

// fcLastLines returns the last n lines of b that are not empty.
func fcLastLines(b []byte, n int) []string {
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines
}

func fcLaunchedMessage(n int) string {
	if n == 1 {
		return "minifire: 1 microVM lit"
	}
	return fmt.Sprintf("minifire: %d microVMs lit", n)
}

func fcIsBeaconName(name string) bool {
	return strings.EqualFold(name, "minifire")
}
