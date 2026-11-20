// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/tests/live/harness"

	"golang.org/x/net/websocket"
)

// web is the miniweb client: it does not follow redirects, so they can be checked.
var web = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// webDo sends a request to miniweb and returns the status code and body.
func webDo(t *testing.T, method, path string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, harness.MiniwebURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := web.Do(req)
	if err != nil {
		t.Fatalf("%v %v: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%v %v: reading body: %v", method, path, err)
	}
	return resp.StatusCode, resp.Header, body
}

// webGet fetches a miniweb path and fails unless it returns 200.
func webGet(t *testing.T, path string) []byte {
	t.Helper()
	code, _, body := webDo(t, http.MethodGet, path)
	if code != http.StatusOK {
		t.Fatalf("GET %v: %v %q", path, code, truncate(body))
	}
	return body
}

// webJSON fetches a miniweb JSON table.
func webJSON(t *testing.T, path string) []map[string]string {
	t.Helper()
	var rows []map[string]string
	if err := json.Unmarshal(webGet(t, path), &rows); err != nil {
		t.Fatalf("GET %v: %v", path, err)
	}
	return rows
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}

// nsPath prefixes a miniweb path with the test namespace, as the UI does.
func nsPath(path string) string { return "/" + harness.Namespace + path }

// testMiniweb drives the miniweb container service against the running
// experiment, through the same routes its UI and phenix-style clients use.
func testMiniweb(t *testing.T) {
	t.Run("Redirects", testMiniwebRedirects)
	t.Run("Pages", testMiniwebPages)
	t.Run("VMs", testMiniwebVMs)
	t.Run("Tables", testMiniwebTables)
	t.Run("Files", testMiniwebFiles)
	t.Run("Screenshot", testMiniwebScreenshot)
	t.Run("VNC", testMiniwebVNC)
	t.Run("VMControl", testMiniwebVMControl)
	t.Run("ConsoleDisabled", testMiniwebConsoleDisabled)
}

func testMiniwebRedirects(t *testing.T) {
	for path, want := range map[string]string{
		"/":                           "/vms",
		"/" + harness.Namespace:       "/" + harness.Namespace + "/",
		"/" + harness.Namespace + "/": "/" + harness.Namespace + "/vms",
	} {
		code, h, _ := webDo(t, http.MethodGet, path)
		if loc := h.Get("Location"); code != http.StatusFound || !strings.HasSuffix(loc, want) {
			t.Errorf("GET %v = %v %q, want 302 to %v", path, code, loc, want)
		}
	}
}

// localRef matches root-relative src/href attributes ("/js/x.js", not "//cdn" or "vms").
var localRef = regexp.MustCompile(`(?:src|href)="(/[^/"][^"]*)"`)

func testMiniwebPages(t *testing.T) {
	refs := map[string]bool{}
	pages := 0
	for _, page := range []string{"/vms", "/hosts", "/vlans", "/graph", "/tilevnc", "/files/", "/minibuilder", "/namespaces"} {
		for _, path := range []string{page, nsPath(page)} {
			if page == "/namespaces" && path != page {
				continue
			}
			body := webGet(t, path)
			pages++
			if !bytes.Contains(body, []byte("<title>minimega</title>")) {
				t.Errorf("GET %v: no minimega layout: %q", path, truncate(body))
			}
			for _, m := range localRef.FindAllSubmatch(body, -1) {
				refs[string(m[1])] = true
			}
		}
	}

	// the new VM form lists the saved configuration, and no blank entry
	body := webGet(t, nsPath("/vms/new"))
	if !bytes.Contains(body, []byte(`value="live-base"`)) {
		t.Errorf("/vms/new does not offer the live-base config")
	}
	if bytes.Contains(body, []byte(`<option value=""></option>`)) {
		t.Errorf("/vms/new offers a blank config")
	}

	// every script, stylesheet and image the pages load ships in the image;
	// /console is a nav link to a feature the image leaves disabled
	delete(refs, "/console")
	refs["/docs/"] = true
	for _, ref := range harness.Keys(refs) {
		if code, _, _ := webDo(t, http.MethodGet, ref); code != http.StatusOK {
			t.Errorf("GET %v (linked from the UI) = %v", ref, code)
		}
	}
	t.Logf("%d pages and %d assets", pages, len(refs))
}

func testMiniwebVMs(t *testing.T) {
	want := harness.VMInfo(t)

	// miniweb talks to minimega over the same socket and lists our VMs, by
	// query string and by path prefix
	for _, path := range []string{"/vms/info.json?namespace=" + harness.Namespace, nsPath("/vms/info.json")} {
		rows := webJSON(t, path)
		got := map[string]map[string]string{}
		for _, r := range rows {
			got[r["name"]] = r
		}
		for _, s := range topology {
			r, ok := got[s.Name]
			if !ok {
				t.Errorf("%v missing %v", path, s.Name)
				continue
			}
			for _, col := range []string{"id", "uuid", "type", "memory", "vcpus", "host"} {
				if r[col] != want[s.Name][col] {
					t.Errorf("%v: %v %v = %q, vm info says %q", path, s.Name, col, r[col], want[s.Name][col])
				}
			}
			if r["type"] != "kvm" {
				t.Errorf("%v: %v type = %q", path, s.Name, r["type"])
			}
		}
		if len(rows) != len(topology) {
			t.Errorf("%v: %d VMs, want %d", path, len(rows), len(topology))
		}
	}

	// vm top samples CPU over time, so give it a moment to report every VM
	harness.Eventually(t, time.Minute, 5*time.Second, func() error {
		code, _, body := webDo(t, http.MethodGet, nsPath("/vms/top.json"))
		var rows []map[string]string
		if err := json.Unmarshal(body, &rows); code != http.StatusOK || err != nil {
			return fmt.Errorf("top.json: %v %v", code, err)
		}
		names := map[string]bool{}
		for _, r := range rows {
			names[r["name"]] = true
		}
		for _, s := range topology {
			if !names[s.Name] {
				return fmt.Errorf("top.json missing %v (%d rows)", s.Name, len(rows))
			}
		}
		return nil
	})
}

func testMiniwebTables(t *testing.T) {
	hosts := webJSON(t, "/hosts.json")
	if len(hosts) != 1 || hosts[0]["cpus"] == "" {
		t.Errorf("hosts.json = %v, want one host", hosts)
	}

	found := false
	for _, r := range webJSON(t, "/namespaces.json") {
		found = found || r["namespace"] == harness.Namespace
	}
	if !found {
		t.Errorf("namespaces.json does not list %v", harness.Namespace)
	}

	vlans := map[string]string{}
	for _, r := range webJSON(t, nsPath("/vlans.json")) {
		vlans[r["alias"]] = r["vlan"]
	}
	for _, r := range harness.MustTable(t, "vlans") {
		if vlans[r["alias"]] != r["vlan"] {
			t.Errorf("vlans.json %v = %q, vlans says %q", r["alias"], vlans[r["alias"]], r["vlan"])
		}
	}
	if vlans["lan-a"] != "1001" {
		t.Errorf("vlans.json lan-a = %q, want 1001", vlans["lan-a"])
	}
}

func testMiniwebFiles(t *testing.T) {
	// run.sh stages the guest image and topology under files/live
	names := map[string]bool{}
	for _, r := range webJSON(t, nsPath("/files.json?path=live")) {
		names[r["name"]] = true
	}
	for _, f := range []string{"live/topology.mm", "live/live.kernel", "live/live.initrd"} {
		if !names[f] {
			t.Errorf("files.json?path=live missing %v: %v", f, harness.Keys(names))
		}
	}

	// a single file is streamed back through minimega's "file stream"
	want, err := os.ReadFile(filepath.Join(harness.MMFiles, "live", "topology.mm"))
	if err != nil {
		t.Fatal(err)
	}
	code, h, body := webDo(t, http.MethodGet, nsPath("/files/live/topology.mm"))
	if code != http.StatusOK || !bytes.Equal(body, want) {
		t.Errorf("download topology.mm: %v, %d bytes, want %d", code, len(body), len(want))
	}
	if d := h.Get("Content-Disposition"); !strings.Contains(d, "topology.mm") {
		t.Errorf("download Content-Disposition = %q", d)
	}
}

func testMiniwebScreenshot(t *testing.T) {
	path := nsPath("/vm/host-a1/screenshot.png")
	img, err := png.Decode(bytes.NewReader(webGet(t, path)))
	if err != nil {
		t.Fatalf("%v is not a PNG: %v", path, err)
	}
	t.Logf("screenshot %v", img.Bounds().Size())

	// size bounds the longer side, as the tile view requests it
	cfg, err := png.DecodeConfig(bytes.NewReader(webGet(t, path+"?size=200")))
	if err != nil || cfg.Width > 200 || cfg.Height > 200 {
		t.Errorf("size=200 screenshot: %vx%v %v", cfg.Width, cfg.Height, err)
	}

	if b := webGet(t, path+"?base64=1"); !bytes.HasPrefix(b, []byte("data:image/png;base64,")) {
		t.Errorf("base64 screenshot: %q", truncate(b))
	}

	if code, _, _ := webDo(t, http.MethodGet, nsPath("/vm/no-such-vm/screenshot.png")); code != http.StatusNotFound {
		t.Errorf("screenshot of unknown VM = %v, want 404", code)
	}
}

func testMiniwebVNC(t *testing.T) {
	if b := webGet(t, nsPath("/vm/host-a1/connect/")); !bytes.Contains(b, []byte("noVNC")) {
		t.Errorf("connect page is not noVNC: %q", truncate(b))
	}
	if code, _, _ := webDo(t, http.MethodGet, nsPath("/vm/no-such-vm/connect/")); code != http.StatusNotFound {
		t.Errorf("connect to unknown VM = %v, want 404", code)
	}

	// the websocket tunnels to the VM's VNC server, which greets with its RFB version
	ws := dialWS(t, nsPath("/vm/host-a1/connect/ws"))
	defer ws.Close()
	greeting := make([]byte, 12)
	if _, err := io.ReadFull(ws, greeting); err != nil {
		t.Fatalf("reading RFB greeting: %v", err)
	}
	if !regexp.MustCompile(`^RFB \d{3}\.\d{3}\n$`).Match(greeting) {
		t.Errorf("VNC greeting = %q", greeting)
	}
}

// dialWS opens a miniweb websocket with the connect, handshake and reads bounded.
func dialWS(t *testing.T, path string) *websocket.Conn {
	t.Helper()
	u, err := url.Parse(harness.MiniwebURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := websocket.NewConfig(u.String(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Location.Path = path
	cfg.Location.Scheme = "ws"

	d := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if u.Scheme == "https" {
		cfg.Location.Scheme = "wss"
		conn, err = tls.DialWithDialer(d, "tcp", u.Host, &tls.Config{ServerName: u.Hostname()})
	} else {
		conn, err = d.Dial("tcp", u.Host)
	}
	if err != nil {
		t.Fatalf("websocket %v: %v", cfg.Location, err)
	}
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	ws, err := websocket.NewClient(cfg, conn)
	if err != nil {
		conn.Close()
		t.Fatalf("websocket %v: %v", cfg.Location, err)
	}
	return ws
}

func testMiniwebVMControl(t *testing.T) {
	// host-d4 runs no cc agent, so pausing it does not disturb cc phases
	const vm = "host-d4"
	t.Cleanup(func() { harness.Run("vm start " + vm) })

	state := func() string { return harness.VMInfo(t)[vm]["state"] }
	post := func(action string) {
		t.Helper()
		if code, _, body := webDo(t, http.MethodPost, nsPath("/vm/"+vm+"/"+action)); code != http.StatusOK {
			t.Fatalf("POST %v %v: %v %q", action, vm, code, truncate(body))
		}
	}

	post("stop")
	if s := state(); s != "PAUSED" {
		t.Errorf("state after POST stop = %v", s)
	}
	post("start")
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		if s := state(); s != "RUNNING" {
			return fmt.Errorf("state after POST start = %v", s)
		}
		return nil
	})

	// control actions need POST and report minimega's errors
	if code, _, _ := webDo(t, http.MethodGet, nsPath("/vm/"+vm+"/stop")); code != http.StatusNotFound {
		t.Errorf("GET stop = %v, want 404", code)
	}
	if s := state(); s != "RUNNING" {
		t.Errorf("GET stop changed the state to %v", s)
	}
	code, _, body := webDo(t, http.MethodPost, nsPath("/vm/no-such-vm/stop"))
	if code != http.StatusBadRequest || !bytes.Contains(body, []byte("not found")) {
		t.Errorf("POST stop on unknown VM = %v %q, want 400 vm not found", code, truncate(body))
	}
}

func testMiniwebConsoleDisabled(t *testing.T) {
	// the image runs miniweb without -console, so nothing executes commands
	for _, path := range []string{"/console", "/command", "/commands"} {
		if code, _, _ := webDo(t, http.MethodPost, path); code != http.StatusNotImplemented {
			t.Errorf("POST %v = %v, want 501", path, code)
		}
	}
}
