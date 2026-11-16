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

// minimega does not set a keymap, so QEMU uses the scancodes of extended key
// events.
func TestKVMQemuArgsNoKeymap(t *testing.T) {
	args := NewVMConfig().qemuArgs(7, "/tmp/minimega/7")
	if slices.Contains(args, "-k") {
		t.Errorf("arguments contain -k: %q", args)
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
// security on each, and then reads the client messages. It keeps the key
// events in keys.
type fakeRFBServer struct {
	addr string

	mu   sync.Mutex
	keys []vnc.KeyEvent
}

func newFakeRFBServer(t *testing.T) *fakeRFBServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	f := &fakeRFBServer{addr: ln.Addr().String()}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go f.serve(conn)
		}
	}()

	return f
}

func (f *fakeRFBServer) serve(conn net.Conn) {
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

	for {
		msg, err := vnc.ReadClientMessage(conn)
		if err != nil {
			return
		}

		if k, ok := msg.(*vnc.KeyEvent); ok {
			f.mu.Lock()
			f.keys = append(f.keys, *k)
			f.mu.Unlock()
		}
	}
}

// typed returns the keys of the key press events that the server got.
func (f *fakeRFBServer) typed() (string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var b strings.Builder
	for _, k := range f.keys {
		if k.DownFlag != 0 {
			b.WriteRune(rune(k.Key))
		}
	}

	return b.String(), len(f.keys)
}

// splitAddr returns the host and the port of addr.
func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatal(err)
	}

	return host, port
}

func TestVNCGetStep(t *testing.T) {
	addr := newFakeRFBServer(t).addr
	host, port := splitAddr(t, addr)

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

// vnc type types the characters that it can type, skips the others, and
// does not log the text.
func TestVNCType(t *testing.T) {
	var logs strings.Builder
	var logMu sync.Mutex
	log.AddLogger("test:"+t.Name(), writerFunc(func(p []byte) (int, error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logs.Write(p)
	}), log.DEBUG, false)
	t.Cleanup(func() { log.DelLogger("test:" + t.Name()) })

	srv := newFakeRFBServer(t)
	host, port := splitAddr(t, srv.addr)

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

	// each typed character is a press and a release
	const want = "s3crt-pw"
	waitForVNC(t, "key events", func() bool {
		_, n := srv.typed()
		return n >= 2*len(want)
	})
	if got, n := srv.typed(); got != want || n != 2*len(want) {
		t.Errorf("server got %d key events that type %q, want %d that type %q", n, got, 2*len(want), want)
	}

	logMu.Lock()
	defer logMu.Unlock()
	if strings.Contains(logs.String(), "s3cr") {
		t.Errorf("log has the typed text:\n%s", logs.String())
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// testRunningKVM returns a running KvmVM.
func testRunningKVM(id int, name, namespace string) *KvmVM {
	base := &BaseVM{ID: id, Name: name, Namespace: namespace, Type: KVM, State: VM_RUNNING, kill: make(chan bool)}
	base.UUID = generateUUID()
	base.cond = sync.NewCond(&base.lock)
	base.instancePath = filepath.Join(*f_base, strconv.Itoa(id))

	return &KvmVM{BaseVM: base}
}

// When QEMU exits, the VNC recordings of the VM stop, so that a VM that is
// launched again with the same name can record. The paste that types stops,
// and the pastes that wait are dropped.
func TestVNCRecordingsStopOnGuestExit(t *testing.T) {
	originalBase := *f_base
	*f_base = t.TempDir()
	t.Cleanup(func() { *f_base = originalBase })

	pasteDir := t.TempDir()
	t.Setenv("TMPDIR", pasteDir)

	addr := newFakeRFBServer(t).addr
	dir := t.TempDir()

	ns := &Namespace{Name: "test", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
	ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
	t.Cleanup(ns.Recorder.Clear)
	t.Cleanup(ns.Player.Clear)

	vm := testRunningKVM(1, "vm-0", ns.Name)
	ns.VMs.hookExit(vm)
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

	// each paste takes seconds to type, so both are still there at the exit
	for i := 0; i < 2; i++ {
		if err := ns.Player.Paste("vm-0", addr, strings.Repeat("a", 500)); err != nil {
			t.Fatal(err)
		}
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
	waitForVNC(t, "paste stop", func() bool {
		entries, _ := os.ReadDir(pasteDir)
		return len(entries) == 0 && len(ns.Player.Info()) == 0
	})
}

// testFlushableKVM returns a KvmVM in the given state, with an instance path
// and a namespace alias, so that a flush can remove it.
func testFlushableKVM(t *testing.T, id int, name, namespace string, state VMState) *KvmVM {
	t.Helper()

	vm := testRunningKVM(id, name, namespace)
	vm.State = state
	if err := os.MkdirAll(vm.instancePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vm.createInstancePathAlias(); err != nil {
		t.Fatal(err)
	}

	return vm
}

// A flush stops the VNC recordings of a KVM VM in the ERROR state whose QEMU
// still runs. A later QEMU exit of the flushed VM does not stop the
// recordings of a new VM with the same name.
func TestVNCRecordingsStopOnFlush(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "Flush"
		if all {
			name = "FlushAll"
		}

		t.Run(name, func(t *testing.T) {
			originalBase := *f_base
			*f_base = t.TempDir()
			t.Cleanup(func() { *f_base = originalBase })

			addr := newFakeRFBServer(t).addr
			dir := t.TempDir()

			ns := &Namespace{Name: "test", Recorder: vnc.NewRecorder(), Player: vnc.NewPlayer("")}
			ns.VMs = VMs{m: map[int]VM{}, stopVNC: ns.stopVMRecordings}
			t.Cleanup(ns.Recorder.Clear)
			t.Cleanup(ns.Player.Clear)

			flushed := testFlushableKVM(t, 1, "vm-0", ns.Name, VM_ERROR)
			ns.VMs.hookExit(flushed)
			ns.VMs.m[1] = flushed

			// the QEMU process of the flushed VM
			cmd := exec.Command("sleep", "60")
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cmd.Process.Kill() })
			wait := make(chan bool)
			flushed.waitForExit(cmd.Process, wait)

			if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "1.kb")); err != nil {
				t.Fatal(err)
			}

			var err error
			if all {
				err = ns.VMs.FlushAll(nil)
			} else {
				err = ns.VMs.Flush("vm-0", nil)
			}
			if err != nil {
				t.Fatalf("flush: %v", err)
			}
			if len(ns.VMs.m) != 0 {
				t.Fatalf("VMs after the flush: %v", ns.VMs.m)
			}
			if info := ns.Recorder.Info(); len(info) != 0 {
				t.Fatalf("recordings after the flush: %q", info)
			}

			vm := testRunningKVM(2, "vm-0", ns.Name)
			ns.VMs.hookExit(vm)
			ns.VMs.m[2] = vm
			if err := ns.Recorder.RecordKB("vm-0", addr, filepath.Join(dir, "2.kb")); err != nil {
				t.Fatalf("record after the flush: %v", err)
			}

			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-wait:
			case <-time.After(10 * time.Second):
				t.Fatal("timed out waiting for the process exit")
			}

			if info := ns.Recorder.Info(); len(info) != 1 {
				t.Errorf("recordings after the QEMU exit of the flushed VM: %q, want the recording of the new VM", info)
			}
		})
	}
}

// A KVM VM that Launch creates in a namespace from NewNamespace stops its
// VNC recordings when QEMU exits.
func TestVNCRecordingsStopOnLaunchedVMExit(t *testing.T) {
	originalBase, originalIOMBase := *f_base, *f_iomBase
	*f_base = t.TempDir()
	*f_iomBase = filepath.Join(*f_base, "files")
	t.Cleanup(func() { *f_base, *f_iomBase = originalBase, originalIOMBase })

	ns := NewNamespace(DefaultNamespace)
	t.Cleanup(ns.ccServer.Destroy)
	t.Cleanup(ns.Recorder.Clear)
	t.Cleanup(ns.Player.Clear)

	// QEMU does not start, so the launch fails and adds the VM in the ERROR
	// state
	config := NewVMConfig()
	config.QemuPath = filepath.Join(t.TempDir(), "qemu")
	q := &QueuedVMs{Names: []string{"vm-l"}, VMType: KVM, VMConfig: config}

	var errs []error
	for err := range ns.VMs.Launch("", ns.Name, q) {
		errs = append(errs, err)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "unable to start qemu") {
		t.Fatalf("launch errors = %v, want one start error", errs)
	}

	ns.VMs.mu.Lock()
	var vm *KvmVM
	for _, v := range ns.VMs.m {
		vm, _ = v.(*KvmVM)
	}
	ns.VMs.mu.Unlock()
	if vm == nil {
		t.Fatal("launch added no KVM VM")
	}

	addr := newFakeRFBServer(t).addr
	if err := ns.Recorder.RecordKB("vm-l", addr, filepath.Join(t.TempDir(), "1.kb")); err != nil {
		t.Fatal(err)
	}

	// a start of the VM waits for the new QEMU process
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

	addr := newFakeRFBServer(t).addr
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
