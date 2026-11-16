// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/qmp"
)

// fakeQMP serves a minimal QMP socket. handler maps each command to the reply
// object sent back; every executed command name and its arguments are
// recorded.
type fakeQMP struct {
	mu   sync.Mutex
	cmds []map[string]interface{}
}

func (f *fakeQMP) executed(name string) []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var res []map[string]interface{}
	for _, c := range f.cmds {
		if c["execute"] == name {
			res = append(res, c)
		}
	}
	return res
}

// newTestKVM returns a KvmVM wired to a fake QMP server.
func newTestKVM(t *testing.T, handler func(cmd map[string]interface{}) interface{}) (*KvmVM, *fakeQMP) {
	t.Helper()

	dir := t.TempDir()
	l, err := net.Listen("unix", filepath.Join(dir, "qmp"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	f := &fakeQMP{}
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()

		enc, dec := json.NewEncoder(c), json.NewDecoder(c)
		enc.Encode(map[string]interface{}{"QMP": map[string]interface{}{}})
		for {
			var cmd map[string]interface{}
			if dec.Decode(&cmd) != nil {
				return
			}
			if cmd["execute"] == "qmp_capabilities" {
				enc.Encode(map[string]interface{}{"return": map[string]interface{}{}})
				continue
			}
			f.mu.Lock()
			f.cmds = append(f.cmds, cmd)
			f.mu.Unlock()
			enc.Encode(handler(cmd))
		}
	}()

	vm := &KvmVM{BaseVM: &BaseVM{instancePath: dir}, hotplug: map[int]vmHotplug{}}
	vm.Tags = map[string]string{}
	vm.q, err = qmp.Dial(filepath.Join(dir, "qmp"))
	if err != nil {
		t.Fatal(err)
	}

	return vm, f
}

func ret(v interface{}) interface{} { return map[string]interface{}{"return": v} }

func TestQueryMigrateOtherStatuses(t *testing.T) {
	for _, status := range []string{"setup", "cancelling", "cancelled", "pre-switchover", "postcopy-active"} {
		t.Run(status, func(t *testing.T) {
			vm, _ := newTestKVM(t, func(map[string]interface{}) interface{} {
				return ret(map[string]interface{}{"status": status})
			})

			got, pct, err := vm.QueryMigrate() // must not panic
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != status || pct != 0 {
				t.Errorf("got (%q, %v), want (%q, 0)", got, pct, status)
			}
		})
	}
}

func TestSaveMultiDiskNames(t *testing.T) {
	vm, f := newTestKVM(t, func(cmd map[string]interface{}) interface{} {
		switch cmd["execute"] {
		case "query-block":
			var devs []interface{}
			for i := 0; i < 4; i++ {
				devs = append(devs, map[string]interface{}{
					"device":   fmt.Sprintf("drive%d", i),
					"inserted": map[string]interface{}{"file": fmt.Sprintf("%s/9/disk%d", *f_base, i)},
				})
			}
			return ret(devs)
		case "query-block-jobs":
			return ret([]interface{}{})
		}
		return ret(map[string]interface{}{})
	})
	vm.ID = 9
	vm.Disks = DiskConfigs{{}}

	if err := vm.Save("/out/vm.qcow2"); err != nil {
		t.Fatal(err)
	}

	want := []string{"/out/vm.qcow2", "/out/vm.qcow2.1", "/out/vm.qcow2.2", "/out/vm.qcow2.3"}
	cmds := f.executed("drive-backup")
	if len(cmds) != len(want) {
		t.Fatalf("got %d drive-backup commands, want %d", len(cmds), len(want))
	}
	for i, c := range cmds {
		if got := c["arguments"].(map[string]interface{})["target"]; got != want[i] {
			t.Errorf("disk %d saved to %v, want %v", i, got, want[i])
		}
	}
}

func TestAddNICBadBridgeLeavesNoNIC(t *testing.T) {
	vm, f := newTestKVM(t, func(map[string]interface{}) interface{} {
		return ret("")
	})

	err := vm.AddNIC(NetConfig{Bridge: "no-such-bridge", VLAN: 100, MAC: "00:11:22:33:44:55"})
	if err == nil {
		t.Fatal("AddNIC on an unusable bridge succeeded")
	}
	if len(vm.Networks) != 0 {
		t.Errorf("failed AddNIC left %d NIC(s) recorded: %+v", len(vm.Networks), vm.Networks)
	}
	if n := len(f.executed("human-monitor-command")); n != 0 {
		t.Errorf("sent %d QMP commands for a NIC that has no tap", n)
	}
}

// hmpHandler replies to human-monitor-command with out(command-line).
func hmpHandler(out func(line string) string) func(map[string]interface{}) interface{} {
	return func(cmd map[string]interface{}) interface{} {
		line, _ := cmd["arguments"].(map[string]interface{})["command-line"].(string)
		return ret(out(line))
	}
}

func hmpLines(f *fakeQMP) []string {
	var res []string
	for _, c := range f.executed("human-monitor-command") {
		res = append(res, c["arguments"].(map[string]interface{})["command-line"].(string))
	}
	return res
}

func TestHotplugDriveAddFails(t *testing.T) {
	vm, f := newTestKVM(t, hmpHandler(func(line string) string {
		return "Could not open '/nonexistent.img': No such file or directory"
	}))

	if err := vm.Hotplug("/nonexistent.img", "", ""); err == nil {
		t.Error("Hotplug reported success for a drive QEMU could not open")
	}
	if len(vm.hotplug) != 0 {
		t.Errorf("recorded nonexistent device: %v", vm.hotplug)
	}
	for _, l := range hmpLines(f) {
		if strings.HasPrefix(l, "device_add") {
			t.Errorf("sent %q after drive_add failed", l)
		}
	}
}

func TestHotplugDeviceAddFailsRollsBackDrive(t *testing.T) {
	vm, f := newTestKVM(t, hmpHandler(func(line string) string {
		switch {
		case strings.HasPrefix(line, "drive_add"):
			return "OK" // what QEMU prints for a successful if=none drive_add
		case strings.HasPrefix(line, "device_add"):
			return "Bus 'usb-bus.0' not found"
		}
		return ""
	}))

	if err := vm.Hotplug("/img", "", ""); err == nil {
		t.Error("Hotplug reported success when device_add failed")
	}
	if len(vm.hotplug) != 0 {
		t.Errorf("recorded nonexistent device: %v", vm.hotplug)
	}
	lines := hmpLines(f)
	if last := lines[len(lines)-1]; last != "drive_del hotplug0" {
		t.Errorf("drive leaked, last HMP command %q (all: %q)", last, lines)
	}
}

func TestHotplugSuccess(t *testing.T) {
	vm, _ := newTestKVM(t, hmpHandler(func(line string) string {
		if strings.HasPrefix(line, "drive_add") {
			return "OK"
		}
		return ""
	}))

	if err := vm.Hotplug("/img", "", ""); err != nil {
		t.Fatal(err)
	}
	if len(vm.hotplug) != 1 {
		t.Errorf("hotplug = %v, want one device", vm.hotplug)
	}
}

// Concurrent QMP users must each get their own reply back.
func TestQMPConcurrentCommandsGetOwnReplies(t *testing.T) {
	vm, _ := newTestKVM(t, func(cmd map[string]interface{}) interface{} {
		if cmd["execute"] == "query-migrate" {
			return ret(map[string]interface{}{"status": "setup"})
		}
		return ret(map[string]interface{}{"id": cmd["id"]})
	})

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 400; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			if i%2 == 0 {
				status, _, err := vm.QueryMigrate()
				if err != nil || status != "setup" {
					t.Errorf("QueryMigrate got (%q, %v), want setup", status, err)
				}
				return
			}

			out, err := vm.QMPRaw(fmt.Sprintf(`{"execute":"echo","id":%d}`, i))
			if err != nil || !strings.Contains(out, fmt.Sprintf(`"id":%d}`, i)) {
				t.Errorf("QMPRaw %d got %q, %v", i, out, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
}
