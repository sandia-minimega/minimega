// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
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
	list := func(pattern string) []string { return mesh[pattern] }

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

	none := func(string) []string { return nil }
	if err := fetchCCUpdateFiles("ns", none, get); err == nil {
		t.Fatal("fetchCCUpdateFiles did not fail when the mesh has no binaries")
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
