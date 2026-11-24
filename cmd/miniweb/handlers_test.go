// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

var registerIndex sync.Once

// serve runs a request through the global mux with the index and a stub VMs page.
func serve(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	registerIndex.Do(func() {
		mux.HandleFunc("/", indexHandler)
		mux.HandleFunc("/vms", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("vms namespace=" + r.URL.Query().Get("namespace")))
		})
		mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("files"))
		})
	})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestIndexRedirects(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/", "/vms"},
		{"/live", "/live/"},
		{"/live/", "/live/vms"},
		{"/?namespace=a%20b", "/a%20b/vms"},
		{"/?namespace=vms", "/vms/vms"},
		// a namespace named like a route keeps the query form
		{"/?namespace=files", "/vms?namespace=files"},
	} {
		w := serve(t, http.MethodGet, tc.path)
		if w.Code != http.StatusFound || w.Header().Get("Location") != tc.want {
			t.Errorf("GET %v = %v %q, want 302 %q", tc.path, w.Code, w.Header().Get("Location"), tc.want)
		}
	}
}

func TestIndexNamespacePrefix(t *testing.T) {
	w := serve(t, http.MethodGet, "/live/vms")
	if got := w.Body.String(); w.Code != http.StatusOK || got != "vms namespace=live" {
		t.Errorf("GET /live/vms = %v %q", w.Code, got)
	}

	// a namespace in both the path and the query is rejected
	if w := serve(t, http.MethodGet, "/live/vms?namespace=other"); w.Code != http.StatusNotFound {
		t.Errorf("GET /live/vms?namespace=other = %v, want 404", w.Code)
	}
}

func TestVmsPathWithFixedNamespace(t *testing.T) {
	old := *f_namespace
	*f_namespace = "fixed"
	defer func() { *f_namespace = old }()

	r := httptest.NewRequest(http.MethodGet, "/?namespace=live", nil)
	if got := vmsPath(r); got != "/vms" {
		t.Errorf("vmsPath with -namespace = %q, want /vms", got)
	}
}

func TestConsoleUnknownPtyDoesNotDeadlock(t *testing.T) {
	// a missing pty used to return with ptyMu held, hanging every later request
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			consoleHandler(w, httptest.NewRequest(http.MethodGet, "/console/999999/size?rows=24&cols=80", nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("request %v: code = %v, want 404", i, w.Code)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("consoleHandler deadlocked on an unknown pty")
	}
}

func TestVmHandlerControlNeedsPost(t *testing.T) {
	for _, path := range []string{"/vm/foo/start", "/vm/foo/stop", "/vm/foo/kill", "/vm/foo"} {
		w := httptest.NewRecorder()
		vmHandler(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Errorf("GET %v = %v, want 404 or 400", path, w.Code)
		}
	}
}

func TestVmHandlerControlUnreachable(t *testing.T) {
	// without a minimega to run the command, the action is not reported as done
	old := *f_base
	*f_base = t.TempDir()
	defer func() { *f_base = old }()

	w := httptest.NewRecorder()
	vmHandler(w, httptest.NewRequest(http.MethodPost, "/vm/foo/stop", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("POST stop without minimega = %v %q, want 502", w.Code, w.Body.String())
	}
}
