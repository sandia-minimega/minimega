// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/iomeshage"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestCCAutoUpdateGetterParses(t *testing.T) {
	c, err := minicli.Compile("cc auto-update")
	if err != nil {
		t.Fatalf("cc auto-update does not parse: %v", err)
	}
	if !c.BoolArgs["auto-update"] {
		t.Fatalf("cc auto-update matched the wrong pattern: %v", c.BoolArgs)
	}
}

func TestCCAutoUpdateConcurrentRequests(t *testing.T) {
	ns := &Namespace{Name: "test", ccServer: &ron.Server{}}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(enable bool) {
			defer wg.Done()

			arg := "false"
			if enable {
				arg = "true"
			}
			c := &minicli.Command{BoolArgs: map[string]bool{"auto-update": true, arg: true}}
			if err := cliCCAutoUpdate(ns, c, &minicli.Response{}); err != nil {
				t.Error(err)
			}

			resp := &minicli.Response{}
			if err := cliCCAutoUpdate(ns, &minicli.Command{BoolArgs: map[string]bool{"auto-update": true}}, resp); err != nil {
				t.Error(err)
			}
		}(i%2 == 0)
	}
	wg.Wait()

	resp := &minicli.Response{}
	if err := cliCCAutoUpdate(ns, &minicli.Command{BoolArgs: map[string]bool{"auto-update": true}}, resp); err != nil {
		t.Fatal(err)
	}
	if want := strconv.FormatBool(ns.ccServer.AutoUpdate()); resp.Response != want {
		t.Fatalf("cc auto-update = %q, want %q from the server", resp.Response, want)
	}
}

// addTestNamespace registers ns and removes it when the test ends.
func addTestNamespace(t *testing.T, ns *Namespace) {
	t.Helper()

	namespaceLock.Lock()
	namespaces[ns.Name] = ns
	namespaceLock.Unlock()

	t.Cleanup(func() {
		namespaceLock.Lock()
		delete(namespaces, ns.Name)
		namespaceLock.Unlock()
	})
}

// useEmptyMesh sets iom to a mesh that has no files and restores it when the
// test ends.
func useEmptyMesh(t *testing.T) {
	t.Helper()

	empty, err := iomeshage.New(t.TempDir(), &meshage.Node{}, "", false)
	if err != nil {
		t.Fatal(err)
	}

	old := iom
	iom = empty
	t.Cleanup(func() { iom = old })
}

func TestCCAutoUpdateRecordsNamespaceSetting(t *testing.T) {
	ns := &Namespace{Name: "cc-auto-update-record", Hosts: map[string]bool{hostname: true}, ccServer: &ron.Server{}}
	addTestNamespace(t, ns)
	useEmptyMesh(t)

	run := func(cmd string, wantErr bool, want int32) {
		t.Helper()

		c := minicli.MustCompile(cmd)
		c.SetNamespace(ns.Name)

		respChan := make(chan minicli.Responses, 1)
		cliCCBroadcast(c, respChan)

		failed := false
		for _, resp := range <-respChan {
			failed = failed || resp.Error != ""
		}
		if failed != wantErr {
			t.Errorf("%q failed = %v, want %v", cmd, failed, wantErr)
		}
		if got := ns.ccAutoUpdate.Load(); got != want {
			t.Errorf("namespace cc auto-update = %v after %q, want %v", got, cmd, want)
		}
	}

	// the mesh has no miniccc binaries, so enabling fails on the local host
	run("cc auto-update true", true, ccAutoUpdateUnset)
	if ns.ccServer.AutoUpdate() {
		t.Fatal("cc auto-update true enabled auto-update without binaries")
	}
	run("cc auto-update false", false, ccAutoUpdateFalse)
	run("cc auto-update true", true, ccAutoUpdateFalse)

	iom = nil
	run("cc auto-update true", false, ccAutoUpdateTrue)
}

func TestNamespaceSendsCCAutoUpdateToNewHosts(t *testing.T) {
	ns := &Namespace{Name: "cc-auto-update-hosts", Hosts: map[string]bool{}, ccServer: &ron.Server{}}
	addTestNamespace(t, ns)

	ns.ccServer.SetAutoUpdate(true)
	if err := ns.sendCCAutoUpdate([]string{hostname}); err != nil {
		t.Fatal(err)
	}
	if !ns.ccServer.AutoUpdate() {
		t.Fatal("sendCCAutoUpdate changed auto-update while the namespace setting is unset")
	}

	for _, want := range []bool{false, true, false} {
		setting := ccAutoUpdateFalse
		if want {
			setting = ccAutoUpdateTrue
		}
		ns.ccAutoUpdate.Store(setting)

		if err := ns.sendCCAutoUpdate([]string{hostname}); err != nil {
			t.Fatal(err)
		}
		if got := ns.ccServer.AutoUpdate(); got != want {
			t.Fatalf("auto-update on the new local host = %v, want %v", got, want)
		}
	}

	cmds := namespaceHostCommands(ns, []string{"peer"}, minicli.MustCompile("cc auto-update false"))
	want := `mesh send "peer" namespace "cc-auto-update-hosts" cc auto-update false`
	if len(cmds) != 1 || cmds[0].Original != want || cmds[0].Source != ns.Name {
		t.Fatalf("remote commands = %v, want %q with source %q", cmds, want, ns.Name)
	}
}

func TestFetchCCUpdateFilesUsesMesh(t *testing.T) {
	mesh := map[string][]string{
		"miniccc":        {"miniccc"},
		"miniccc-*":      {"miniccc-linux-arm64", "miniccc-linux-arm64", "miniccc-notes.txt"},
		"ns/miniccc.exe": {"ns/miniccc.exe"},
		"ns/miniccc-*":   {"ns/miniccc-windows-amd64.exe", "ns/sub/miniccc"},
	}

	var got []string
	get := func(file string) error {
		got = append(got, file)
		return nil
	}
	list := func(pattern string) ([]string, error) { return mesh[pattern], nil }

	if err := fetchCCUpdateFiles("ns", list, get); err != nil {
		t.Fatal(err)
	}

	sort.Strings(got)
	want := []string{"miniccc", "miniccc-linux-arm64", "ns/miniccc-windows-amd64.exe", "ns/miniccc.exe"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetched %v, want %v", got, want)
	}

	fail := func(string) error { return errors.New("no route") }
	if err := fetchCCUpdateFiles("", list, fail); err == nil {
		t.Fatal("fetchCCUpdateFiles ignored a mesh error")
	}

	none := func(string) ([]string, error) { return nil, nil }
	if err := fetchCCUpdateFiles("ns", none, get); err == nil {
		t.Fatal("fetchCCUpdateFiles did not fail when the mesh has no binaries")
	}

	timeout := func(string) ([]string, error) { return nil, errors.New("timeout") }
	if err := fetchCCUpdateFiles("ns", timeout, get); err == nil {
		t.Fatal("fetchCCUpdateFiles ignored a mesh search error")
	}
}

func TestCCClientsShowsCapabilities(t *testing.T) {
	s, err := ron.NewServer(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.UseVMs = false

	sock := filepath.Join(t.TempDir(), "cc")
	if err := s.ListenUnix(sock); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.CloseUnix(sock) })

	connect := func(cl *ron.Client) {
		t.Helper()

		conn, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })

		if _, err := io.WriteString(conn, "RON"); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(conn, make([]byte, 3)); err != nil {
			t.Fatal(err)
		}
		go io.Copy(io.Discard, conn)

		m := ron.Message{Type: ron.MESSAGE_CLIENT, UUID: cl.UUID, Client: cl, Version: "v1"}
		if err := gob.NewEncoder(conn).Encode(&m); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 200 && !s.HasClient(cl.UUID); i++ {
			time.Sleep(10 * time.Millisecond)
		}
	}

	connect(&ron.Client{UUID: "new", Hostname: "new", Capabilities: []string{ron.CapAutoUpdate, "other"}})
	connect(&ron.Client{UUID: "old", Hostname: "old"})

	resp := &minicli.Response{}
	if err := cliCCClients(&Namespace{ccServer: s}, &minicli.Command{}, resp); err != nil {
		t.Fatal(err)
	}

	last := len(resp.Header) - 1
	if resp.Header[last] != "capabilities" {
		t.Fatalf("last column = %q, want capabilities", resp.Header[last])
	}

	got := map[string]string{}
	for _, row := range resp.Tabular {
		got[row[0]] = row[last]
	}
	want := map[string]string{"new": strings.Join([]string{ron.CapAutoUpdate, "other"}, ","), "old": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
}

func TestCCRTunnelDoesNotRequireVM(t *testing.T) {
	ccServer, err := ron.NewServer(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ccServer.Destroy()

	c, err := minicli.Compile("cc rtunnel 4444 localhost 80")
	if err != nil {
		t.Fatal(err)
	}

	if !c.BoolArgs["rtunnel"] {
		t.Fatalf("rtunnel not set: %v", c.BoolArgs)
	}
	for k, want := range map[string]string{"src": "4444", "host": "localhost", "dst": "80"} {
		if got := c.StringArgs[k]; got != want {
			t.Errorf("StringArgs[%q] = %q, want %q", k, got, want)
		}
	}

	// with no clients connected, rtunnel reports that instead of a missing VM
	ns := &Namespace{Name: "test", ccServer: ccServer}
	err = cliCC(ns, c, &minicli.Response{})
	if err == nil || err.Error() != "no clients connected" {
		t.Fatalf("cc rtunnel = %v, want no clients connected", err)
	}
}

func TestCCRTunnelUsesNamespaceFilter(t *testing.T) {
	ccServer, err := ron.NewServer(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ccServer.UseVMs = false
	t.Cleanup(func() {
		waitFor(t, "clients to disconnect", func() bool { return ccServer.Clients() == 0 })
		ccServer.Destroy()
	})

	path := filepath.Join(t.TempDir(), "cc")
	if err := ccServer.ListenUnix(path); err != nil {
		t.Fatal(err)
	}

	// the windows client never starts its tunnel, so rtunnel fails if it
	// does not apply the filter
	dialCCClient(t, path, &ron.Client{UUID: "linux-client", OS: "linux"}, true)
	dialCCClient(t, path, &ron.Client{UUID: "windows-client", OS: "windows"}, false)

	waitFor(t, "windows-client to connect", func() bool { return ccServer.HasClient("windows-client") })
	waitFor(t, "linux-client tunnel", func() bool {
		_, err := ccServer.ListForwards("linux-client")
		return err == nil
	})

	ns := &Namespace{Name: "test", ccServer: ccServer}

	rtunnel := func(src int, host string, dst int) error {
		c, err := minicli.Compile(fmt.Sprintf("cc rtunnel %d %s %d", src, host, dst))
		if err != nil {
			t.Fatal(err)
		}

		return cliCC(ns, c, &minicli.Response{})
	}

	ns.ccFilter = &ron.Filter{OS: "plan9"}
	if err := rtunnel(freeTCPPort(t), "127.0.0.1", 80); err == nil || err.Error() != "no clients match filter: os=plan9" {
		t.Fatalf("cc rtunnel with os=plan9 = %v, want no clients match filter", err)
	}

	dest, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()

	served := make(chan error, 1)
	go func() {
		conn, err := dest.Accept()
		if err != nil {
			served <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, len("hello"))
		if _, err := io.ReadFull(conn, buf); err != nil {
			served <- err
			return
		}
		if string(buf) != "hello" {
			served <- fmt.Errorf("destination got %q, want %q", buf, "hello")
			return
		}

		_, err = conn.Write([]byte("world"))
		served <- err
	}()

	ns.ccFilter = &ron.Filter{OS: "linux"}
	src := freeTCPPort(t)
	if err := rtunnel(src, "127.0.0.1", dest.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatalf("cc rtunnel with os=linux: %v", err)
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", src))
	if err != nil {
		t.Fatalf("dial reverse tunnel: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len("world"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Fatalf("client got %q, want %q", buf, "world")
	}

	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

func TestCCRTunnelListPatterns(t *testing.T) {
	if err := minicli.Validate(); err != nil {
		t.Fatal(err)
	}

	for input, vm := range map[string]string{
		"cc rtunnel list":           "",
		"cc rtunnel list all":       "all",
		"cc rtunnel list server-vm": "server-vm",
	} {
		c, err := minicli.Compile(input)
		if err != nil {
			t.Fatalf("Compile(%q): %v", input, err)
		}

		if !c.BoolArgs["rtunnel"] || !c.BoolArgs["list"] {
			t.Errorf("%q: BoolArgs = %v, want rtunnel and list", input, c.BoolArgs)
		}
		if got := c.StringArgs["vm"]; got != vm {
			t.Errorf("%q: StringArgs[vm] = %q, want %q", input, got, vm)
		}
		if _, ok := c.StringArgs["src"]; ok {
			t.Errorf("%q: StringArgs = %v, want no src", input, c.StringArgs)
		}
	}

	c, err := minicli.Compile("cc rtunnel 4444 list 80")
	if err != nil {
		t.Fatal(err)
	}
	if c.BoolArgs["list"] || c.StringArgs["host"] != "list" {
		t.Fatalf("cc rtunnel 4444 list 80: BoolArgs = %v, StringArgs = %v, want host list", c.BoolArgs, c.StringArgs)
	}
}

func TestCCRTunnelList(t *testing.T) {
	ccServer, err := ron.NewServer(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ccServer.UseVMs = false
	t.Cleanup(func() {
		waitFor(t, "clients to disconnect", func() bool { return ccServer.Clients() == 0 })
		ccServer.Destroy()
	})

	path := filepath.Join(t.TempDir(), "cc")
	if err := ccServer.ListenUnix(path); err != nil {
		t.Fatal(err)
	}

	// the windows client never starts its tunnel, so it has no reverse tunnels
	linux := dialCCClient(t, path, &ron.Client{UUID: "linux-client", OS: "linux"}, true)
	dialCCClient(t, path, &ron.Client{UUID: "windows-client", OS: "windows"}, false)

	waitFor(t, "windows-client to connect", func() bool { return ccServer.HasClient("windows-client") })
	waitFor(t, "linux-client tunnel", func() bool {
		_, err := ccServer.ListForwards("linux-client")
		return err == nil
	})

	ns := &Namespace{
		Name:     "test",
		ccServer: ccServer,
		VMs: VMs{m: map[int]VM{
			1: &KvmVM{BaseVM: &BaseVM{ID: 1, Name: "linux-vm", BaseConfig: BaseConfig{UUID: "linux-client"}}},
			2: &KvmVM{BaseVM: &BaseVM{ID: 2, Name: "windows-vm", BaseConfig: BaseConfig{UUID: "windows-client"}}},
		}},
	}

	run := func(input string) (*minicli.Response, error) {
		c, err := minicli.Compile(input)
		if err != nil {
			t.Fatal(err)
		}

		resp := &minicli.Response{}
		return resp, cliCC(ns, c, resp)
	}

	header := []string{"vm", "src port", "dst", "dst port"}

	resp, err := run("cc rtunnel list")
	if err != nil {
		t.Fatalf("cc rtunnel list: %v", err)
	}
	if !reflect.DeepEqual(resp.Header, header) || len(resp.Tabular) != 0 {
		t.Fatalf("cc rtunnel list before rtunnel = %v %v, want header %v and no rows", resp.Header, resp.Tabular, header)
	}

	lo, hi := freeTCPPort(t), freeTCPPort(t)
	for hi == lo {
		hi = freeTCPPort(t)
	}
	if hi < lo {
		lo, hi = hi, lo
	}

	// windows-client fails without its tunnel, but linux-client opens one
	if _, err := run(fmt.Sprintf("cc rtunnel %d 127.0.0.1 80", hi)); err == nil || err.Error() != "windows-client: tunnel has not been initialized" {
		t.Fatalf("cc rtunnel without filter = %v, want an error for windows-client", err)
	}

	ns.ccFilter = &ron.Filter{OS: "linux"}
	if _, err := run(fmt.Sprintf("cc rtunnel %d localhost 8080", lo)); err != nil {
		t.Fatalf("cc rtunnel with os=linux: %v", err)
	}

	want := [][]string{
		{"linux-vm", strconv.Itoa(lo), "localhost", "8080"},
		{"linux-vm", strconv.Itoa(hi), "127.0.0.1", "80"},
	}

	for _, input := range []string{"cc rtunnel list", "cc rtunnel list all", "cc rtunnel list linux-vm"} {
		resp, err := run(input)
		if err != nil {
			t.Fatalf("%v: %v", input, err)
		}
		if !reflect.DeepEqual(resp.Header, header) || !reflect.DeepEqual(resp.Tabular, want) {
			t.Errorf("%v = %v %v, want %v %v", input, resp.Header, resp.Tabular, header, want)
		}
	}

	resp, err = run("cc rtunnel list windows-vm")
	if err != nil || len(resp.Tabular) != 0 {
		t.Fatalf("cc rtunnel list windows-vm = %v, %v, want no rows", resp.Tabular, err)
	}

	if _, err := run("cc rtunnel list missing-vm"); err == nil || !isVMNotFound(err.Error()) {
		t.Fatalf("cc rtunnel list missing-vm = %v, want vm not found", err)
	}

	linux.Close()
	waitFor(t, "linux-client to disconnect", func() bool { return !ccServer.HasClient("linux-client") })

	resp, err = run("cc rtunnel list")
	if err != nil || len(resp.Tabular) != 0 {
		t.Fatalf("cc rtunnel list after disconnect = %v, %v, want no rows", resp.Tabular, err)
	}

	if _, err := run("cc rtunnel list linux-vm"); err == nil || err.Error() != "no such client: linux-client" {
		t.Fatalf("cc rtunnel list linux-vm after disconnect = %v, want no such client", err)
	}
}

// dialCCClient connects to the cc server at path as client. When tunnel is
// true, it also serves minitunnel traffic the same way miniccc does. Close the
// returned connection to disconnect the client.
func dialCCClient(t *testing.T, path string, client *ron.Client, tunnel bool) net.Conn {
	t.Helper()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}

	local, remote := net.Pipe()
	t.Cleanup(func() {
		conn.Close()
		local.Close()
		remote.Close()
	})

	if _, err := io.WriteString(conn, "RON"); err != nil {
		t.Fatal(err)
	}
	magic := make([]byte, len("RON"))
	if _, err := io.ReadFull(conn, magic); err != nil {
		t.Fatal(err)
	}

	enc, dec := gob.NewEncoder(conn), gob.NewDecoder(conn)

	hello := &ron.Message{Type: ron.MESSAGE_CLIENT, UUID: client.UUID, Version: "v1", Client: client}
	if err := enc.Encode(hello); err != nil {
		t.Fatal(err)
	}

	var ack ron.Message
	if err := dec.Decode(&ack); err != nil {
		t.Fatal(err)
	}

	var encLock sync.Mutex
	send := func(m *ron.Message) error {
		encLock.Lock()
		defer encLock.Unlock()

		return enc.Encode(m)
	}

	if tunnel {
		go minitunnel.ListenAndServe(local)
		go ron.Trunk(remote, client.UUID, send)
	}

	go func() {
		for {
			var m ron.Message
			if err := dec.Decode(&m); err != nil {
				return
			}
			if tunnel && m.Type == ron.MESSAGE_TUNNEL {
				if _, err := remote.Write(m.Tunnel); err != nil {
					return
				}
			}
		}
	}()

	return conn
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMergeCCRTunnelResponses(t *testing.T) {
	ok := &minicli.Response{Host: "a"}
	noMatch := &minicli.Response{Host: "b", Error: "no clients match filter: os=linux"}
	noConn := &minicli.Response{Host: "c", Error: "no clients connected"}
	failed := &minicli.Response{Host: "d", Error: "client uuid-1: timeout"}

	for _, tc := range []struct {
		name string
		in   minicli.Responses
		want minicli.Responses
	}{
		{"success and no match", minicli.Responses{ok, noMatch}, minicli.Responses{ok}},
		{"all no match", minicli.Responses{noMatch, noConn}, minicli.Responses{noMatch}},
		{"failure and no match", minicli.Responses{noConn, failed}, minicli.Responses{failed}},
		{"success and failure", minicli.Responses{ok, failed}, minicli.Responses{ok, failed}},
	} {
		if got := mergeCCRTunnelResponses(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: mergeCCRTunnelResponses = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCCCLIWrapper(t *testing.T) {
	wrapper := func(name string) minicli.CLIFunc {
		return func(_ *minicli.Command, respChan chan<- minicli.Responses) {
			respChan <- minicli.Responses{{Response: name}}
		}
	}
	broadcast, vmTarget, rtunnel := wrapper("broadcast"), wrapper("vmTarget"), wrapper("rtunnel")

	for input, want := range map[string]string{
		"cc rtunnel 4444 localhost 80": "rtunnel",
		"cc rtunnel list":              "broadcast",
		"cc rtunnel list all":          "broadcast",
		"cc rtunnel list server-vm":    "vmTarget",
		"cc tunnel list all":           "broadcast",
		"cc clients":                   "broadcast",
	} {
		c, err := minicli.Compile(input)
		if err != nil {
			t.Fatalf("Compile(%q): %v", input, err)
		}

		respChan := make(chan minicli.Responses, 1)
		ccCLIWrapper(c, broadcast, vmTarget, rtunnel)(c, respChan)

		if got := (<-respChan)[0].Response; got != want {
			t.Errorf("%q: ccCLIWrapper = %v, want %v", input, got, want)
		}
	}
}

func TestCCCommandPreProcess(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *ron.Command
		wantErr bool
	}{
		{
			name: "no options",
			args: []string{"cat", "foo"},
			want: &ron.Command{Command: []string{"cat", "foo"}},
		},
		{
			name: "stdio",
			args: []string{"stdin=in", "stdout=out", "stderr=err", "cat"},
			want: &ron.Command{Stdin: "in", Stdout: "out", Stderr: "err", Command: []string{"cat"}},
		},
		{
			name: "session",
			args: []string{"session=user", "notepad.exe"},
			want: &ron.Command{Session: ron.SessionUser, Command: []string{"notepad.exe"}},
		},
		{
			name: "session with stdio",
			args: []string{"stdout=out", "session=elevated", "stdin=in", "stderr=err", "cat"},
			want: &ron.Command{
				Stdin: "in", Stdout: "out", Stderr: "err",
				Session: ron.SessionElevated,
				Command: []string{"cat"},
			},
		},
		{
			name:    "invalid session",
			args:    []string{"session=root", "cat"},
			wantErr: true,
		},
		{
			name:    "options without a command",
			args:    []string{"stdout=out", "session=user"},
			wantErr: true,
		},
		{
			name: "unknown key starts the command",
			args: []string{"FOO=bar", "session=user"},
			want: &ron.Command{Command: []string{"FOO=bar", "session=user"}},
		},
		{
			name: "options after the command are arguments",
			args: []string{"echo", "session=user"},
			want: &ron.Command{Command: []string{"echo", "session=user"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ccCommandPreProcess(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCCSendExpand(t *testing.T) {
	errTimeout := errors.New("timeout")

	cases := []struct {
		name     string
		subpath  string
		pattern  string
		info     map[string][]string
		errs     map[string]error
		noSearch bool
		want     []ccSendFile
		wantErr  error
	}{
		{
			name:    "namespace match has namespace-relative send name",
			subpath: "blue",
			pattern: "*.txt",
			info:    map[string][]string{"blue/*.txt": {"blue/one.txt"}},
			want:    []ccSendFile{{mesh: "blue/one.txt", send: "one.txt"}},
		},
		{
			name:    "global match keeps its directory",
			subpath: "red",
			pattern: "blue/*.txt",
			info:    map[string][]string{"blue/*.txt": {"blue/one.txt"}},
			want:    []ccSendFile{{mesh: "blue/one.txt", send: "blue/one.txt"}},
		},
		{
			name:    "files below matched directories are not sent",
			pattern: "*",
			info: map[string][]string{"*": {
				"one.txt",
				"blue/one.txt",
				"miniccc_responses/1/uuid/stdout",
			}},
			want: []ccSendFile{{mesh: "one.txt", send: "one.txt"}},
		},
		{
			name:    "namespace files below matched directories are not sent",
			subpath: "blue",
			pattern: "*",
			info: map[string][]string{"blue/*": {
				"blue/one.txt",
				"blue/miniccc_responses/1/uuid/stdout",
			}},
			want: []ccSendFile{{mesh: "blue/one.txt", send: "one.txt"}},
		},
		{
			name:    "namespace search error does not fall back",
			subpath: "blue",
			pattern: "*.txt",
			info:    map[string][]string{"*.txt": {"global.txt"}},
			errs:    map[string]error{"blue/*.txt": errTimeout},
			wantErr: errTimeout,
		},
		{
			name:    "empty namespace search falls back",
			subpath: "blue",
			pattern: "*.txt",
			info:    map[string][]string{"*.txt": {"global.txt"}},
			want:    []ccSendFile{{mesh: "global.txt", send: "global.txt"}},
		},
		{
			name:    "files on many nodes are sent once",
			pattern: "*.txt",
			info:    map[string][]string{"*.txt": {"two.txt", "one.txt", "two.txt", "one.txt"}},
			want: []ccSendFile{
				{mesh: "one.txt", send: "one.txt"},
				{mesh: "two.txt", send: "two.txt"},
			},
		},
		{
			name:     "literal names do not search the mesh",
			subpath:  "blue",
			pattern:  "one.txt",
			noSearch: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := func(glob string) ([]string, error) {
				if tc.noSearch {
					t.Fatalf("unexpected mesh search for %q", glob)
				}

				return tc.info[glob], tc.errs[glob]
			}

			got, err := ccSendExpand(tc.subpath, tc.pattern, info)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("files = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// useFileMesh sets the files directory and iom to a new directory on a mesh
// with one node, and restores them when the test ends. It returns the
// directory.
func useFileMesh(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	mesh, err := iomeshage.New(base, &meshage.Node{}, "", false)
	if err != nil {
		t.Fatal(err)
	}

	oldIOM, oldBase := iom, *f_iomBase
	iom, *f_iomBase = mesh, base
	t.Cleanup(func() { iom, *f_iomBase = oldIOM, oldBase })

	return base
}

// writeTestFiles creates each named file below base with the name as its
// content.
func writeTestFiles(t *testing.T, base string, names ...string) {
	t.Helper()

	for _, name := range names {
		path := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// ccFileSend runs cc send in namespace name with the files directory base and
// returns the names that the new command sends.
func ccFileSend(t *testing.T, base, name string, files ...string) ([]string, error) {
	t.Helper()

	subpath := name
	if name == DefaultNamespace {
		subpath = ""
	}

	ccServer, err := ron.NewServer(base, subpath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ccServer.Destroy)

	ns := &Namespace{Name: name, ccServer: ccServer}
	c := &minicli.Command{
		BoolArgs: map[string]bool{"send": true},
		ListArgs: map[string][]string{"file": files},
	}

	resp := &minicli.Response{}
	if err := cliCCFileSend(ns, c, resp); err != nil {
		return nil, err
	}

	return ccServer.GetCommand(resp.Data.(int)).FilesSend, nil
}

func TestCCFileSendAbsoluteGlobUsesFilesDirectory(t *testing.T) {
	base := useFileMesh(t)
	writeTestFiles(t, base, "a.txt", "blue/b.txt")

	got, err := ccFileSend(t, base, "blue", filepath.Join(base, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"a.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files sent = %v, want %v", got, want)
	}
}

func TestCCFileSendGlobMatchesAreLiteral(t *testing.T) {
	base := useFileMesh(t)
	writeTestFiles(t, base, "report1.txt", "report[1].txt", "only/x[1].txt", "only/x1.log")

	got, err := ccFileSend(t, base, DefaultNamespace, "*.txt", "only/*.txt")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"report1.txt", "report[1].txt", "only/x[1].txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files sent = %v, want %v", got, want)
	}
}

func TestCCSendGlob(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "*.txt", want: "*.txt", ok: true},
		{in: "file:*.txt", want: "*.txt", ok: true},
		{in: "file:report?.txt", want: "report?.txt", ok: true},
		{in: "file:dir/[ab].txt", want: "dir/[ab].txt", ok: true},
		{in: "/files/*.txt", want: "/files/*.txt", ok: true},
		{in: "file:one.txt", want: "one.txt"},
		{in: "one.txt", want: "one.txt"},
		{in: "file:///files/*.txt"},
		{in: "http://example.com/a?b=c"},
	}

	for _, tc := range cases {
		got, ok := ccSendGlob(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ccSendGlob(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
