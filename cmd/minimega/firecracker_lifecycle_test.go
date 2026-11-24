// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func testFCConfig() VMConfig {
	cfg := NewVMConfig()
	cfg.KernelPath = "vmlinux"
	return cfg
}

func testNICs(n int) NetConfigs {
	var res NetConfigs
	for i := 0; i < n; i++ {
		res = append(res, NetConfig{VLAN: 100 + i})
	}
	return res
}

func testDisks(n int) DiskConfigs {
	var res DiskConfigs
	for i := 0; i < n; i++ {
		res = append(res, DiskConfig{Path: fmt.Sprintf("disk%d.raw", i)})
	}
	return res
}

func TestFCValidateConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		edit func(*VMConfig)
		want string // substring of the error; empty for no error
	}{
		{name: "defaults", edit: func(*VMConfig) {}},
		{name: "no kernel", edit: func(c *VMConfig) { c.KernelPath = "" }, want: "need a kernel"},
		{name: "1 vcpu", edit: func(c *VMConfig) { c.VCPUs = 1 }},
		{name: "32 vcpus", edit: func(c *VMConfig) { c.VCPUs = 32 }},
		{name: "0 vcpus", edit: func(c *VMConfig) { c.VCPUs = 0 }, want: "1 to 32 vCPUs, not 0"},
		{name: "33 vcpus", edit: func(c *VMConfig) { c.VCPUs = 33 }, want: "1 to 32 vCPUs, not 33"},
		{name: "cdrom", edit: func(c *VMConfig) { c.CdromPath = "x.iso" }, want: "CD-ROM"},
		{name: "serial ports", edit: func(c *VMConfig) { c.SerialPorts = 1 }, want: "serial ports"},
		{name: "virtio ports", edit: func(c *VMConfig) { c.VirtioPorts = "foo" }, want: "virtio ports"},
		{name: "migrate", edit: func(c *VMConfig) { c.MigratePath = "x.state" }, want: "use firecracker-snapshot"},
		{name: "qinq", edit: func(c *VMConfig) { c.Networks = NetConfigs{{VLAN: 1, QinQ: true}} }, want: "QinQ: NIC 0"},
		{name: "bond", edit: func(c *VMConfig) { c.Bonds = BondConfigs{{Name: "b0"}} }, want: "bonds"},
		{
			name: "inject without snapshot",
			edit: func(c *VMConfig) {
				c.Snapshot = false
				c.Injects = FCInjects{{Partition: 1, Src: "a", Dst: "/a"}}
			},
			want: "snapshot true",
		},
		{
			name: "inject with snapshot",
			edit: func(c *VMConfig) { c.Injects = FCInjects{{Partition: 1, Src: "a", Dst: "/a"}} },
		},
		{
			name: "inject with ro disk 0",
			edit: func(c *VMConfig) {
				c.Disks = DiskConfigs{{Path: "root.squashfs", ReadOnly: true}}
				c.Injects = FCInjects{{Partition: 1, Src: "a", Dst: "/a"}}
			},
			want: "vm config inject needs a writable disk 0: disk 0 has option ro",
		},
		{
			name: "inject with ro disk 1",
			edit: func(c *VMConfig) {
				c.Disks = DiskConfigs{{Path: "data.raw"}, {Path: "root.squashfs", ReadOnly: true}}
				c.Injects = FCInjects{{Partition: 1, Src: "a", Dst: "/a"}}
			},
		},
		{
			name: "ro disk without snapshot",
			edit: func(c *VMConfig) {
				c.Snapshot = false
				c.Disks = DiskConfigs{{Path: "root.squashfs", ReadOnly: true}}
			},
		},
		{name: "scratch", edit: func(c *VMConfig) { c.FirecrackerScratch = "64M" }},
		{name: "bad scratch", edit: func(c *VMConfig) { c.FirecrackerScratch = "12x" }, want: "firecracker-scratch: invalid size"},
		{
			name: "17 devices",
			edit: func(c *VMConfig) { c.Disks, c.Networks = testDisks(1), testNICs(14) },
		},
		{
			name: "18 devices with scratch",
			edit: func(c *VMConfig) {
				c.Disks, c.Networks = testDisks(1), testNICs(14)
				c.FirecrackerScratch = "1G"
			},
			want: "VM web1 needs 18 devices (1 disk, scratch, 14 NICs, vsock, entropy); the limit is 17 without firecracker-pci",
		},
		{
			name: "18 devices",
			edit: func(c *VMConfig) { c.Disks, c.Networks = testDisks(2), testNICs(14) },
			want: "VM web1 needs 18 devices (2 disks, 14 NICs, vsock, entropy); the limit is 17 without firecracker-pci",
		},
		{
			name: "31 devices with pci",
			edit: func(c *VMConfig) {
				c.FirecrackerPCI = true
				c.Disks, c.Networks = testDisks(1), testNICs(28)
			},
		},
		{
			name: "32 devices with pci",
			edit: func(c *VMConfig) {
				c.FirecrackerPCI = true
				c.Disks, c.Networks = testDisks(2), testNICs(28)
			},
			want: "needs 32 devices (2 disks, 28 NICs, vsock, entropy); the limit is 31 with firecracker-pci",
		},
		{
			name: "two errors",
			edit: func(c *VMConfig) { c.KernelPath, c.CdromPath = "", "x.iso" },
			want: "need a kernel: set vm config kernel; firecracker VMs do not support a CD-ROM",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testFCConfig()
			test.edit(&cfg)

			_, err := fcValidateConfig("web1", cfg)
			switch {
			case test.want == "" && err != nil:
				t.Errorf("fcValidateConfig() = %v, want no error", err)
			case test.want != "" && err == nil:
				t.Errorf("fcValidateConfig() = nil, want error with %q", test.want)
			case test.want != "" && !strings.Contains(err.Error(), test.want):
				t.Errorf("fcValidateConfig() = %v, want error with %q", err, test.want)
			}
		})
	}
}

func TestFCValidateConfigWarnings(t *testing.T) {
	if w, err := fcValidateConfig("web1", testFCConfig()); err != nil || len(w) != 0 {
		t.Fatalf("fcValidateConfig(defaults) = %q, %v, want no warnings", w, err)
	}

	tests := []struct {
		name string
		edit func(*VMConfig)
		want string
	}{
		{"qemu", func(c *VMConfig) { c.QemuPath = "qemu-system-x86_64" }, "vm config qemu"},
		{"cpu", func(c *VMConfig) { c.CPU = "Haswell" }, "vm config cpu"},
		{"sockets", func(c *VMConfig) { c.Sockets = 2 }, "vm config sockets"},
		{"cores", func(c *VMConfig) { c.Cores = 2 }, "vm config cores"},
		{"threads", func(c *VMConfig) { c.Threads = 2 }, "vm config threads"},
		{"machine", func(c *VMConfig) { c.Machine = "q35" }, "vm config machine"},
		{"baremetal", func(c *VMConfig) { c.Baremetal = true }, "vm config baremetal"},
		{"baremetal driver", func(c *VMConfig) { c.BaremetalNetworkDriver = "x" }, "vm config baremetal-network-driver"},
		{"vga", func(c *VMConfig) { c.Vga = "qxl" }, "vm config vga"},
		{"usb", func(c *VMConfig) { c.UsbUseXHCI = false }, "vm config usb-use-xhci"},
		{"tpm", func(c *VMConfig) { c.TpmSocketPath = "/tmp/tpm" }, "vm config tpm-socket"},
		{"copy paste", func(c *VMConfig) { c.BidirectionalCopyPaste = true }, "vm config bidirectional-copy-paste"},
		{"qemu append", func(c *VMConfig) { c.QemuAppend = []string{"-vga", "qxl"} }, "vm config qemu-append"},
		{"qemu override", func(c *VMConfig) { c.QemuOverride = QemuOverrides{{Match: "a", Repl: "b"}} }, "vm config qemu-override"},
		{
			"disk interface",
			func(c *VMConfig) { c.Disks = DiskConfigs{{Path: "a.raw", Interface: "ide"}} },
			"ignoring interface ide of disk 0",
		},
		{
			"nic driver",
			func(c *VMConfig) { c.Networks = NetConfigs{{VLAN: 1, Driver: "rtl8139"}} },
			"ignoring driver rtl8139 of NIC 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testFCConfig()
			test.edit(&cfg)

			w, err := fcValidateConfig("web1", cfg)
			if err != nil {
				t.Fatalf("fcValidateConfig() error = %v", err)
			}
			if len(w) != 1 || !strings.Contains(w[0], test.want) || !strings.HasPrefix(w[0], "VM web1: ") {
				t.Errorf("fcValidateConfig() warnings = %q, want one with %q", w, test.want)
			}
		})
	}

	for _, quiet := range []string{"virtio", ""} {
		cfg := testFCConfig()
		cfg.Disks = DiskConfigs{{Path: "a.raw", Interface: quiet}}
		cfg.Networks = NetConfigs{{VLAN: 1, Driver: DefaultKVMDriver}, {VLAN: 2, Driver: "virtio-net-pci"}, {VLAN: 3}}
		if w, _ := fcValidateConfig("web1", cfg); len(w) != 0 {
			t.Errorf("fcValidateConfig(interface %q) warnings = %q, want none", quiet, w)
		}
	}
}

func TestFCDeviceCount(t *testing.T) {
	tests := []struct {
		name        string
		disks, nics int
		backchannel bool
		entropy     bool
		pci         bool
		n, limit    int
		detail      string
	}{
		{"defaults", 0, 0, true, true, false, 2, 17, "0 disks, 0 NICs, vsock, entropy"},
		{"singular", 1, 1, false, false, false, 2, 17, "1 disk, 1 NIC"},
		{"no vsock", 2, 15, false, true, false, 18, 17, "2 disks, 15 NICs, entropy"},
		{"web1", 2, 15, true, true, false, 19, 17, "2 disks, 15 NICs, vsock, entropy"},
		{"pci", 1, 3, true, false, true, 5, 31, "1 disk, 3 NICs, vsock"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := NewVMConfig()
			cfg.Disks, cfg.Networks = testDisks(test.disks), testNICs(test.nics)
			cfg.Backchannel = test.backchannel
			cfg.FirecrackerEntropy = test.entropy
			cfg.FirecrackerPCI = test.pci

			n, limit, detail := fcDeviceCount(cfg)
			if n != test.n || limit != test.limit || detail != test.detail {
				t.Errorf("fcDeviceCount() = %v, %v, %q, want %v, %v, %q", n, limit, detail, test.n, test.limit, test.detail)
			}
		})
	}
}

func TestFCDeviceCountScratch(t *testing.T) {
	cfg := NewVMConfig()
	cfg.Disks, cfg.Networks = testDisks(1), testNICs(2)
	cfg.Backchannel, cfg.FirecrackerEntropy = true, false

	for scratch, want := range map[string]struct {
		n      int
		detail string
	}{
		"":    {4, "1 disk, 2 NICs, vsock"},
		"0":   {4, "1 disk, 2 NICs, vsock"},
		"64M": {5, "1 disk, scratch, 2 NICs, vsock"},
	} {
		cfg.FirecrackerScratch = scratch
		if n, _, detail := fcDeviceCount(cfg); n != want.n || detail != want.detail {
			t.Errorf("fcDeviceCount() with scratch %q = %v, %q, want %v, %q", scratch, n, detail, want.n, want.detail)
		}
	}
}

func TestFCBootArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		disks    int
		wantArgs string
		wantRoot int
		wantErr  string
	}{
		{
			name:     "empty",
			disks:    1,
			wantArgs: "console=ttyS0 reboot=k panic=1",
			wantRoot: 0,
		},
		{
			name:     "no disks",
			wantArgs: "console=ttyS0 reboot=k panic=1",
			wantRoot: -1,
		},
		{
			name:     "root adds rw",
			args:     []string{"root=/dev/vda1"},
			disks:    1,
			wantArgs: "root=/dev/vda1 console=ttyS0 reboot=k panic=1 rw",
			wantRoot: -1,
		},
		{
			name:     "root keeps ro",
			args:     []string{"root=/dev/vda1 ro"},
			disks:    1,
			wantArgs: "root=/dev/vda1 ro console=ttyS0 reboot=k panic=1",
			wantRoot: -1,
		},
		{
			name:     "root keeps rw",
			args:     []string{"root=/dev/vda1", "rw"},
			disks:    2,
			wantArgs: "root=/dev/vda1 rw console=ttyS0 reboot=k panic=1",
			wantRoot: -1,
		},
		{
			name:     "user values kept",
			args:     []string{"console=tty0", "panic=10", "reboot=k", "quiet"},
			disks:    1,
			wantArgs: "console=tty0 panic=10 reboot=k quiet",
			wantRoot: 0,
		},
		{
			name:    "reboot=t",
			args:    []string{"reboot=t"},
			disks:   1,
			wantErr: "reboot=t",
		},
		{
			name:    "bare reboot",
			args:    []string{"reboot"},
			disks:   1,
			wantErr: "need reboot=k",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, root, err := fcBootArgs(test.args, test.disks, 3, false)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("fcBootArgs() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("fcBootArgs() error = %v", err)
			}
			if args != test.wantArgs || root != test.wantRoot {
				t.Errorf("fcBootArgs() = %q, %v, want %q, %v", args, root, test.wantArgs, test.wantRoot)
			}
		})
	}
}

func TestFCBootArgsLength(t *testing.T) {
	const prefix = "console=ttyS0 reboot=k panic=1 x="

	tests := []struct {
		name    string
		devices int
		pci     bool
		extra   int
	}{
		{"mmio", 4, false, 8 + 4*30},
		{"pci", 4, true, 4 * 30},
		{"no devices", 0, true, 0},
	}

	for _, test := range tests {
		for _, total := range []int{2047, 2048} {
			t.Run(fmt.Sprintf("%v/%d", test.name, total), func(t *testing.T) {
				arg := prefix + strings.Repeat("a", total-test.extra-len(prefix))

				args, _, err := fcBootArgs([]string{arg}, 1, test.devices, test.pci)
				if total < 2048 {
					if err != nil {
						t.Fatalf("fcBootArgs() error = %v", err)
					}
					if len(args)+test.extra != total {
						t.Errorf("len(args) + extra = %d, want %d", len(args)+test.extra, total)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "2048 bytes") {
					t.Errorf("fcBootArgs() error = %v, want an error that names 2048 bytes", err)
				}
			})
		}
	}
}

func TestFCMakeConsoleFIFO(t *testing.T) {
	old := fcChown
	defer func() { fcChown = old }()

	var gotPath string
	var gotUID, gotGID int
	fcChown = func(p string, uid, gid int) error {
		gotPath, gotUID, gotGID = p, uid, gid
		return nil
	}

	path := filepath.Join(t.TempDir(), "console.fifo")
	if err := os.WriteFile(path, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := fcMakeConsoleFIFO(path, 2000000007); err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOTSUP) {
			t.Skipf("mkfifo not supported here: %v", err)
		}
		t.Fatal(err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 || fi.Mode().Perm() != 0600 {
		t.Errorf("mode = %v, want named pipe 0600", fi.Mode())
	}
	if gotPath != path || gotUID != 2000000007 || gotGID != 2000000007 {
		t.Errorf("chown(%q, %v, %v), want (%q, 2000000007, 2000000007)", gotPath, gotUID, gotGID, path)
	}

	fcChown = func(string, int, int) error { return errors.New("denied") }
	if err := fcMakeConsoleFIFO(path, 1); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("fcMakeConsoleFIFO() with failing chown = %v, want error", err)
	}
}

func waitFileContent(t *testing.T, path, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := os.ReadFile(path)
		if string(got) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%v = %q, want %q", path, got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestFCConsoleRegularFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "serial.out")
	log := filepath.Join(dir, "console.log")

	if err := os.WriteFile(log, []byte("old boot\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("line one\nline two\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var c fcConsole
	if err := c.Start(src, log, fcConsoleMaxBytes); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(src, log, fcConsoleMaxBytes); err == nil {
		t.Error("second Start() = nil, want error")
	}

	waitFileContent(t, log, "line one\nline two\n")
	c.Stop()
	c.Stop()

	waitFileContent(t, log+".1", "old boot\n")

	// A second Start rotates the log again.
	if err := os.WriteFile(src, []byte("boot two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(src, log, fcConsoleMaxBytes); err != nil {
		t.Fatal(err)
	}
	waitFileContent(t, log, "boot two\n")
	c.Stop()
	waitFileContent(t, log+".1", "line one\nline two\n")
}

func TestFCConsoleTruncation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int64
		want  string
	}{
		{"under limit", "abc\n", 10, "abc\n"},
		{"at limit", "abcd", 4, "abcd"},
		{"over limit", "abcdef\n", 4, "abcd\n[console log truncated at 4 bytes]\n"},
		{"over limit at newline", "abc\ndef\n", 4, "abc\n[console log truncated at 4 bytes]\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			fcCopyConsole(&out, &oneByteReader{s: test.input}, test.max)
			if got := out.String(); got != test.want {
				t.Errorf("fcCopyConsole() wrote %q, want %q", got, test.want)
			}
		})
	}
}

// oneByteReader returns one byte for each Read, so that truncation happens
// across reads.
type oneByteReader struct {
	s string
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.s) == 0 {
		return 0, errors.New("EOF")
	}
	p[0] = r.s[0]
	r.s = r.s[1:]
	return 1, nil
}

func TestFCConsoleFIFO(t *testing.T) {
	old := fcChown
	defer func() { fcChown = old }()
	fcChown = func(string, int, int) error { return nil }

	dir := t.TempDir()
	fifo := filepath.Join(dir, "console.fifo")
	log := filepath.Join(dir, "console.log")

	if err := fcMakeConsoleFIFO(fifo, os.Getuid()); err != nil {
		t.Skipf("mkfifo not supported here: %v", err)
	}

	var c fcConsole
	if err := c.Start(fifo, log, 32); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	write := func(s string) {
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(s); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// The reader keeps running after a writer closes the pipe.
	write("first\n")
	waitFileContent(t, log, "first\n")
	write("second\n")
	waitFileContent(t, log, "first\nsecond\n")

	write(strings.Repeat("x", 40))
	want := "first\nsecond\n" + strings.Repeat("x", 32-13) + "\n[console log truncated at 32 bytes]\n"
	waitFileContent(t, log, want)

	// Output after the limit is drained, not written.
	write("more\n")
	time.Sleep(50 * time.Millisecond)
	waitFileContent(t, log, want)

	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return")
	}
}

func TestFCLogFIFO(t *testing.T) {
	old := fcChown
	defer func() { fcChown = old }()
	fcChown = func(string, int, int) error { return nil }

	dir := t.TempDir()
	fifo := filepath.Join(dir, "vmm.fifo")
	log := filepath.Join(dir, "firecracker.log")

	if err := fcMakeConsoleFIFO(fifo, os.Getuid()); err != nil {
		t.Skipf("mkfifo not supported here: %v", err)
	}
	if err := os.WriteFile(log, []byte("old launch\n"), 0600); err != nil {
		t.Fatal(err)
	}

	write := func(s string) {
		t.Helper()
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(s); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}

	c := &fcConsole{Label: "firecracker log"}
	if err := c.Start(fifo, log, 16); err != nil {
		t.Fatal(err)
	}
	waitFileContent(t, log+".1", "old launch\n")

	write(strings.Repeat("y", 20))
	capped := strings.Repeat("y", 16) + "\n[firecracker log truncated at 16 bytes]\n"
	waitFileContent(t, log, capped)
	c.Stop()

	// The next Start rotates the log, and Stop copies what the pipe holds.
	if err := c.Start(fifo, log, fcLogMaxBytes); err != nil {
		t.Fatal(err)
	}
	waitFileContent(t, log+".1", capped)

	data := strings.Repeat("0123456789abcdef", 3<<10)
	write(data)
	c.Stop()

	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != data {
		t.Errorf("firecracker.log has %d bytes after Stop(), want %d", len(got), len(data))
	}
}

func TestFCTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		n    int64
		want string
	}{
		{0, ""},
		{3, "789"},
		{10, "0123456789"},
		{100, "0123456789"},
	} {
		got, err := fcTail(path, test.n)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != test.want {
			t.Errorf("fcTail(%v) = %q, want %q", test.n, got, test.want)
		}
	}

	if _, err := fcTail(filepath.Join(t.TempDir(), "missing"), 10); err == nil {
		t.Error("fcTail(missing) = nil error, want error")
	}
}

func testProcessState(t *testing.T, script string) (*os.ProcessState, error) {
	t.Helper()

	cmd := exec.Command("sh", "-c", script)
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("run %q: %v", script, err)
	}

	return cmd.ProcessState, err
}

func TestFCClassifyExit(t *testing.T) {
	ok, _ := testProcessState(t, "true")
	exit3, exit3Err := testProcessState(t, "exit 3")
	signaled, _ := testProcessState(t, "kill -9 $$")

	fcLog := []byte("l1\nl2\nl3\nl4\nl5\nl6\n\n")
	panicTail := []byte("[ 1.0] Kernel panic - not syncing: VFS: Unable to mount root fs\n")

	tests := []struct {
		name      string
		state     *os.ProcessState
		waitErr   error
		killed    bool
		console   []byte
		wantState VMState
		wantMsg   string
	}{
		{name: "reboot", state: ok, wantState: VM_QUIT},
		{name: "panic", state: ok, console: panicTail, wantState: VM_ERROR, wantMsg: "guest kernel panic"},
		{name: "killed", state: signaled, killed: true, wantState: VM_QUIT},
		{name: "killed with panic", state: signaled, killed: true, console: panicTail, wantState: VM_QUIT},
		{
			name: "exit 3", state: exit3, waitErr: exit3Err, wantState: VM_ERROR,
			wantMsg: "firecracker exited with exit status 3: l2; l3; l4; l5; l6",
		},
		{
			name: "signal", state: signaled, wantState: VM_ERROR,
			wantMsg: "firecracker exited with signal: killed: l2; l3; l4; l5; l6",
		},
		{name: "wait error", waitErr: errors.New("no child"), wantState: VM_ERROR, wantMsg: "wait for firecracker: no child"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, msg := fcClassifyExit(test.state, test.waitErr, test.killed, test.console, fcLog)
			if state != test.wantState || msg != test.wantMsg {
				t.Errorf("fcClassifyExit() = %v, %q, want %v, %q", state, msg, test.wantState, test.wantMsg)
			}
		})
	}

	if _, msg := fcClassifyExit(exit3, nil, false, nil, nil); msg != "firecracker exited with exit status 3" {
		t.Errorf("fcClassifyExit(no log) msg = %q", msg)
	}
}

func TestFCEasterEggs(t *testing.T) {
	if got := fcLaunchedMessage(1); got != "minifire: 1 microVM lit" {
		t.Errorf("fcLaunchedMessage(1) = %q", got)
	}
	if got := fcLaunchedMessage(100); got != "minifire: 100 microVMs lit" {
		t.Errorf("fcLaunchedMessage(100) = %q", got)
	}

	for name, want := range map[string]bool{"minifire": true, "MiniFire": true, "minifire1": false, "fire": false, "": false} {
		if got := fcIsBeaconName(name); got != want {
			t.Errorf("fcIsBeaconName(%q) = %v, want %v", name, got, want)
		}
	}

	if fcBeaconMessage != "the miniature beacons of gondor have been lit, it's a minifire!" {
		t.Errorf("fcBeaconMessage = %q", fcBeaconMessage)
	}
}
