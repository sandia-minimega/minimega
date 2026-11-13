// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/vnc"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func TestKVMQemuArgsVNC(t *testing.T) {
	c := NewVMConfig()

	args := c.qemuArgs(7, "/tmp/minimega/7")
	if slices.Contains(args, "-k") {
		t.Errorf("arguments contain -k: %q", args)
	}
	if !hasArgSequence(args, "-vnc", "unix:/tmp/minimega/7/vnc") {
		t.Errorf("missing -vnc unix:/tmp/minimega/7/vnc in %q", args)
	}

	c.QemuOverride = QemuOverrides{{Match: "-vnc unix:", Repl: "-vnc lossy=on,vnc=unix:"}}
	got := c.applyQemuOverrides(args)
	if !hasArgSequence(got, "-vnc", "lossy=on,vnc=unix:/tmp/minimega/7/vnc") {
		t.Errorf("missing -vnc lossy=on,vnc=unix:/tmp/minimega/7/vnc in %q", got)
	}
}

func TestVMConfigQemuOverrideLossyPreview(t *testing.T) {
	ns := &Namespace{Name: "test", vmConfig: NewVMConfig()}

	run := func(input string) *minicli.Response {
		t.Helper()

		c, err := minicli.Compile(input)
		if err != nil {
			t.Fatal(err)
		}

		resp := &minicli.Response{}
		if err := cliVMConfigQemuOverride(ns, c, resp); err != nil {
			t.Fatalf("%v: %v", input, err)
		}
		return resp
	}

	run(`vm config qemu-override "-vnc unix:" "-vnc lossy=on,vnc=unix:"`)
	preview := run("vm config qemu-override").Response

	_, after, ok := strings.Cut(preview, "After overrides:")
	if !ok {
		t.Fatalf("preview has no after section: %q", preview)
	}
	if !strings.Contains(after, "-vnc lossy=on,vnc=unix:vnc ") {
		t.Errorf("preview after overrides has no lossy -vnc argument: %q", after)
	}
	if strings.Contains(preview, " -k ") {
		t.Errorf("preview contains -k: %q", preview)
	}
}

func TestVMInfoVGA(t *testing.T) {
	kvm := &KvmVM{
		BaseVM:    &BaseVM{ID: 1, Name: "kvm-vm", Type: KVM},
		KVMConfig: KVMConfig{Vga: "virtio"},
	}
	container := &ContainerVM{BaseVM: &BaseVM{ID: 2, Name: "container-vm", Type: CONTAINER}}
	ns := &Namespace{Name: "test", VMs: VMs{m: map[int]VM{1: kvm, 2: container}}}

	c, err := minicli.Compile("vm info")
	if err != nil {
		t.Fatal(err)
	}

	resp := &minicli.Response{}
	if err := cliVMInfo(ns, c, resp); err != nil {
		t.Fatal(err)
	}

	col := slices.Index(resp.Header, "vga")
	if col < 0 {
		t.Fatalf("vm info header has no vga column: %q", resp.Header)
	}
	name := slices.Index(resp.Header, "name")

	want := map[string]string{"kvm-vm": "virtio", "container-vm": "N/A"}
	got := map[string]string{}
	for _, row := range resp.Tabular {
		got[row[name]] = row[col]
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("vm info vga = %v, want %v", got, want)
	}
}

// fakeRFBServer accepts connections, completes an RFB 3.3 handshake with no
// security on each, and then discards all client messages.
func fakeRFBServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	serve := func(conn net.Conn) {
		defer conn.Close()

		if _, err := io.WriteString(conn, "RFB 003.003\n"); err != nil {
			return
		}
		if _, err := io.ReadFull(conn, make([]byte, 12)); err != nil {
			return
		}
		if err := binary.Write(conn, binary.BigEndian, uint32(1)); err != nil {
			return
		}
		if _, err := io.ReadFull(conn, make([]byte, 1)); err != nil {
			return
		}

		init := struct {
			Width, Height uint16
			PixelFormat   vnc.PixelFormat
			NameLength    uint32
		}{Width: 640, Height: 480}
		if err := binary.Write(conn, binary.BigEndian, &init); err != nil {
			return
		}

		_, _ = io.Copy(io.Discard, conn)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go serve(conn)
		}
	}()

	return ln.Addr().String()
}

func TestVNCGetStep(t *testing.T) {
	addr := fakeRFBServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatal(err)
	}

	kvm := &KvmVM{BaseVM: &BaseVM{ID: 1, Name: "vm-0", Type: KVM, Host: host}, VNCPort: port}
	ns := &Namespace{Name: "test", Player: vnc.NewPlayer(""), VMs: VMs{m: map[int]VM{1: kvm}}}

	const step = "3600000000000:KeyEvent,true,a"
	fname := filepath.Join(t.TempDir(), "steps.kb")
	if err := os.WriteFile(fname, []byte(step+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ns.Player.Playback("vm-0", addr, fname); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ns.Player.Stop("vm-0") })

	waitForVNC(t, "playback step", func() bool {
		res, _ := ns.Player.GetStep("vm-0")
		return res != ""
	})

	c, err := minicli.Compile("vnc getstep vm-0")
	if err != nil {
		t.Fatal(err)
	}

	resp := &minicli.Response{}
	done := make(chan error, 1)
	go func() { done <- cliVNCPlay(ns, c, resp) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("vnc getstep: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for vnc getstep")
	}

	if want := []string{"name", "step"}; !reflect.DeepEqual(resp.Header, want) {
		t.Errorf("header = %q, want %q", resp.Header, want)
	}
	if want := [][]string{{"vm-0", step}}; !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("tabular = %q, want %q", resp.Tabular, want)
	}
}

// T19: vnc type uses the paste path. It skips characters that it cannot
// type, and it does not log the text.
func TestVNCType(t *testing.T) {
	var logs strings.Builder
	var logMu sync.Mutex
	log.AddLogger("test:"+t.Name(), writerFunc(func(p []byte) (int, error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logs.Write(p)
	}), log.DEBUG, false)
	t.Cleanup(func() { log.DelLogger("test:" + t.Name()) })

	addr := fakeRFBServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatal(err)
	}

	kvm := &KvmVM{BaseVM: &BaseVM{ID: 1, Name: "vm-0", Type: KVM, Host: host}, VNCPort: port}
	ns := &Namespace{Name: "test", Player: vnc.NewPlayer(""), VMs: VMs{m: map[int]VM{1: kvm}}}
	t.Cleanup(ns.Player.Clear)

	c, err := minicli.Compile("vnc type vm-0 s3cr\u00e9t-pw")
	if err != nil {
		t.Fatal(err)
	}
	if err := cliVNCPlay(ns, c, &minicli.Response{}); err != nil {
		t.Fatalf("vnc type: %v", err)
	}

	waitForVNC(t, "paste end", func() bool { return len(ns.Player.Info()) == 0 })

	logMu.Lock()
	defer logMu.Unlock()
	if strings.Contains(logs.String(), "s3cr") {
		t.Errorf("log has the typed text:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "skipped 1 characters") {
		t.Errorf("log has no skip warning:\n%s", logs.String())
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// testRunningKVM returns a running KvmVM whose Kill sets it to VM_QUIT.
func testRunningKVM(t *testing.T, id int, name, namespace string) *KvmVM {
	t.Helper()

	base := &BaseVM{ID: id, Name: name, Namespace: namespace, Type: KVM, State: VM_RUNNING, kill: make(chan bool)}
	base.UUID = generateUUID()
	base.cond = sync.NewCond(&base.lock)
	base.instancePath = filepath.Join(*f_base, strconv.Itoa(id))

	go func() {
		<-base.kill
		base.lock.Lock()
		defer base.lock.Unlock()
		base.State = VM_QUIT
		base.cond.Broadcast()
	}()

	return &KvmVM{BaseVM: base}
}

// T9: the VNC recordings of a VM stop when the VM is killed and when it is
// flushed, and a VM that is launched again with the same name records into a
// new file. A kill also stops the paste that types and drops the pastes that
// wait.
func TestVNCRecordingsStopOnKillAndFlush(t *testing.T) {
	originalBase := *f_base
	*f_base = t.TempDir()
	t.Cleanup(func() { *f_base = originalBase })

	pasteDir := t.TempDir()
	t.Setenv("TMPDIR", pasteDir)

	addr := fakeRFBServer(t)
	dir := t.TempDir()

	ns := &Namespace{Name: "test", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
	ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
	t.Cleanup(ns.Player.Clear)

	vm := testRunningKVM(t, 1, "vm-0", ns.Name)
	ns.VMs.m[1] = vm

	for i := 0; i < 2; i++ {
		if err := ns.Player.Paste("vm-0", addr, strings.Repeat("a", 500)); err != nil {
			t.Fatal(err)
		}
	}
	if entries, _ := os.ReadDir(pasteDir); len(entries) != 2 {
		t.Fatalf("got %d paste files, want 2", len(entries))
	}

	if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "1.kb")); err != nil {
		t.Fatal(err)
	}
	if err := ns.Recorder.RecordFB("vm-0", addr, filepath.Join(dir, "1.fb")); err != nil {
		t.Fatal(err)
	}

	if err := ns.VMs.Kill("vm-0"); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if info := ns.Recorder.Info(); len(info) != 0 {
		t.Fatalf("recordings after kill: %q", info)
	}
	waitForVNC(t, "paste stop", func() bool {
		entries, _ := os.ReadDir(pasteDir)
		return len(entries) == 0 && len(ns.Player.Info()) == 0
	})

	// a recording that starts after the kill stops on flush
	if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "2.kb")); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(*f_base, "namespaces", ns.Name, vm.UUID)
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alias, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ns.VMs.Flush("vm-0", nil); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if info := ns.Recorder.Info(); len(info) != 0 {
		t.Fatalf("recordings after flush: %q", info)
	}

	ns.VMs.m[2] = testRunningKVM(t, 2, "vm-0", ns.Name)
	newKB := filepath.Join(dir, "3.kb")
	if err := ns.Recorder.RecordKB("vm-0", addr, newKB); err != nil {
		t.Fatalf("record the relaunched VM: %v", err)
	}
	ns.Recorder.Route("vm-0", &vnc.KeyEvent{DownFlag: 1, Key: 'a'})
	if err := ns.Recorder.StopKB("vm-0"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"1.kb", "2.kb"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err != nil || len(b) != 0 {
			t.Errorf("%v = %q, %v; want an empty file", name, b, err)
		}
	}
	if b, err := os.ReadFile(newKB); err != nil || !strings.HasSuffix(string(b), ":KeyEvent,true,a\n") {
		t.Errorf("new kb file = %q, %v; want one KeyEvent line", b, err)
	}
}

// TestVNCRecordingsStopOnFlushAll checks that FlushAll, which "vm flush" with
// no target and "clear namespace" call, stops a recording of a VM that quit.
func TestVNCRecordingsStopOnFlushAll(t *testing.T) {
	originalBase := *f_base
	*f_base = t.TempDir()
	t.Cleanup(func() { *f_base = originalBase })

	addr := fakeRFBServer(t)
	dir := t.TempDir()

	ns := &Namespace{Name: "test", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
	ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
	t.Cleanup(ns.Player.Clear)

	vm := testRunningKVM(t, 1, "vm-0", ns.Name)
	ns.VMs.m[1] = vm

	if err := ns.VMs.Kill("vm-0"); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// a recording that starts after the kill stops on FlushAll
	if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "1.kb")); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(*f_base, "namespaces", ns.Name, vm.UUID)
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alias, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ns.VMs.FlushAll(nil); err != nil {
		t.Fatalf("flush all: %v", err)
	}
	if info := ns.Recorder.Info(); len(info) != 0 {
		t.Fatalf("recordings after flush all: %q", info)
	}
	if len(ns.VMs.m) != 0 {
		t.Fatalf("VMs after flush all: %v", len(ns.VMs.m))
	}
}

// The VNC recordings of a VM stop when the guest shuts down, so that a VM
// that is launched again with the same name can record.
func TestVNCRecordingsStopOnGuestExit(t *testing.T) {
	originalBase := *f_base
	*f_base = t.TempDir()
	t.Cleanup(func() { *f_base = originalBase })

	addr := fakeRFBServer(t)
	dir := t.TempDir()

	ns := &Namespace{Name: "test", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
	ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
	t.Cleanup(ns.Recorder.Clear)

	vm := testRunningKVM(t, 1, "vm-0", ns.Name)
	vm.onExit = ns.VMs.stopVNC
	ns.VMs.m[1] = vm
	if err := os.MkdirAll(vm.instancePath, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "1.kb")); err != nil {
		t.Fatal(err)
	}
	if err := ns.Recorder.RecordFB("vm-0", addr, filepath.Join(dir, "1.fb")); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan bool)
	vm.waitForExit(cmd.Process, wait)

	select {
	case <-wait:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the process exit")
	}

	if s := vm.GetState(); s != VM_QUIT {
		t.Errorf("state = %v, want %v", s, VM_QUIT)
	}
	if info := ns.Recorder.Info(); len(info) != 0 {
		t.Fatalf("recordings after the guest exit: %q", info)
	}
	if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "2.kb")); err != nil {
		t.Fatalf("record after the guest exit: %v", err)
	}
}

// A VM that -recover finds also stops its VNC recordings when QEMU exits.
func TestVNCRecordingsStopOnRecoveredVMExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("recovery reads /proc")
	}

	originalBase := *f_base
	*f_base = t.TempDir()
	t.Cleanup(func() { *f_base = originalBase })

	const name, id = "vm-r", "7"
	instance := filepath.Join(*f_base, id)
	if err := os.MkdirAll(instance, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{"name": name, "config": "", "state": "QUIT"} {
		if err := os.WriteFile(filepath.Join(instance, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ns := &Namespace{Name: "recovered", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
	ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
	t.Cleanup(ns.Recorder.Clear)

	link := filepath.Join(*f_base, "namespaces", ns.Name, generateUUID())
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(instance, link); err != nil {
		t.Fatal(err)
	}

	namespaceLock.Lock()
	namespaces[ns.Name] = ns
	namespaceLock.Unlock()
	t.Cleanup(func() {
		namespaceLock.Lock()
		delete(namespaces, ns.Name)
		namespaceLock.Unlock()
	})

	if err := recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}

	vm, ok := ns.VMs.m[7].(*KvmVM)
	if !ok {
		t.Fatalf("recovered VMs = %v, want KVM VM 7", ns.VMs.m)
	}

	addr := fakeRFBServer(t)
	if err := ns.Recorder.RecordKB(name, addr, filepath.Join(t.TempDir(), "1.kb")); err != nil {
		t.Fatal(err)
	}

	// a relaunch of the recovered VM waits for the new QEMU process
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan bool)
	vm.waitForExit(cmd.Process, wait)

	select {
	case <-wait:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the process exit")
	}

	if info := ns.Recorder.Info(); len(info) != 0 {
		t.Fatalf("recordings after the guest exit: %q", info)
	}
}

func waitForVNC(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
