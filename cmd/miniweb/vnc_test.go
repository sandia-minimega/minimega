// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/websocket"
)

const rfbVersion = "RFB 003.008\n"

// fakeVNCServer accepts TCP connections and sends an RFB version message on
// each one. It returns the port.
func fakeVNCServer(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.Write([]byte(rfbVersion))
				io.Copy(io.Discard, c)
			}()
		}
	}()

	return l.Addr().(*net.TCPAddr).Port
}

// newVNCTestServer starts a server with the miniweb routes for the VNC page.
// Each VM in the returned map has type kvm and the fake VNC server port.
func newVNCTestServer(t *testing.T) (*httptest.Server, *sync.Map) {
	t.Helper()

	port := fakeVNCServer(t)

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "vnc.html"), []byte("vnc page"), 0644); err != nil {
		t.Fatal(err)
	}

	oldRoot, oldNamespace := *f_root, *f_namespace
	*f_root, *f_namespace = tmp, ""
	oldRunTabular := runTabularFunc
	oldListNamespaces := listNamespacesFunc
	oldMux := mux

	listNamespacesFunc = func() ([]string, error) {
		return []string{"minimega", "ns1"}, nil
	}

	var queries sync.Map
	runTabularFunc = func(cmd *Command) []map[string]string {
		queries.Store(cmd.Namespace+" "+strings.Join(cmd.Filters, " "), true)
		return []map[string]string{{
			"host":     "127.0.0.1",
			"type":     "kvm",
			"vnc_port": strconv.Itoa(port),
		}}
	}

	mux = http.NewServeMux()
	mux.HandleFunc("/", mustAuth(indexHandler))
	mux.HandleFunc("/vm/", mustAuth(vmHandler))

	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		srv.Close()
		*f_root, *f_namespace = oldRoot, oldNamespace
		runTabularFunc = oldRunTabular
		listNamespacesFunc = oldListNamespaces
		mux = oldMux
	})

	return srv, &queries
}

// TestVNCConnectRoutes tests the VNC page and websocket routes for VM names
// with and without dots, with and without a namespace prefix.
func TestVNCConnectRoutes(t *testing.T) {
	srv, queries := newVNCTestServer(t)
	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	cases := []struct {
		path      string
		name      string
		namespace string
	}{
		{"/vm/host.leaf-1.net/connect", "host.leaf-1.net", ""},
		{"/vm/plain/connect", "plain", ""},
		{"/ns1/vm/host.leaf-1.net/connect", "host.leaf-1.net", "ns1"},
		{"/ns1/vm/plain/connect", "plain", "ns1"},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			for _, page := range []string{c.path, c.path + "/"} {
				resp, err := http.Get(srv.URL + page)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK || string(body) != "vnc page" {
					t.Errorf("GET %v: got %v %q, want 200 \"vnc page\"", page, resp.StatusCode, body)
				}
			}

			ws, err := websocket.Dial(wsBase+c.path+"/ws", "", srv.URL)
			if err != nil {
				t.Fatalf("websocket %v: %v", c.path+"/ws", err)
			}
			defer ws.Close()

			buf := make([]byte, len(rfbVersion))
			if _, err := io.ReadFull(ws, buf); err != nil {
				t.Fatalf("websocket %v: read: %v", c.path+"/ws", err)
			}
			if string(buf) != rfbVersion {
				t.Errorf("websocket %v: got %q, want %q", c.path+"/ws", buf, rfbVersion)
			}

			key := c.namespace + " " + "name=" + strconv.Quote(c.name)
			if _, ok := queries.Load(key); !ok {
				t.Errorf("no vm info query for %q", key)
			}
		})
	}

	// A path with a double slash is not a websocket route: the mux redirects it.
	if _, err := websocket.Dial(wsBase+"//vm/host.leaf-1.net/connect/ws", "", srv.URL); err == nil {
		t.Error("websocket with a double slash: got a connection, want an error")
	}
}

// TestVNCHTMLContract tests that vnc.html gives noVNC an empty host and an
// absolute websocket path. With a host, noVNC 1.7.0 puts "./" before the path,
// so an absolute path becomes "//vm/<name>/connect/ws", which miniweb rejects.
func TestVNCHTMLContract(t *testing.T) {
	content, err := os.ReadFile("../../web/vnc.html")
	if err != nil {
		t.Fatalf("read vnc.html: %v", err)
	}

	html := string(content)

	required := []string{
		`<base href="/novnc/">`,
		`window.location.hash = "autoconnect=true&host=&path=" +`,
		`encodeURIComponent(window.location.pathname.replace(/\/+$/, "") + "/ws")`,
	}
	for _, s := range required {
		if !strings.Contains(html, s) {
			t.Errorf("vnc.html does not contain %q", s)
		}
	}
}
