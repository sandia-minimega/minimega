// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// stubNamespaces makes the namespace lookup return the given names and
// records every command miniweb tries to run.
func stubNamespaces(t *testing.T, names ...string) *[]string {
	t.Helper()

	var ran []string

	oldTabular, oldList := runTabularFunc, listNamespacesFunc
	t.Cleanup(func() { runTabularFunc, listNamespacesFunc = oldTabular, oldList })

	runTabularFunc = func(cmd *Command) []map[string]string {
		ran = append(ran, cmd.String())
		return nil
	}

	listNamespacesFunc = func() ([]string, error) {
		ran = append(ran, (&Command{Command: "namespace"}).String())
		return names, nil
	}

	return &ran
}

// setNamespaceFlag sets -namespace for the duration of the test.
func setNamespaceFlag(t *testing.T, name string) {
	t.Helper()

	old := *f_namespace
	t.Cleanup(func() { *f_namespace = old })
	*f_namespace = name
}

func TestRequireNamespace(t *testing.T) {
	cases := []struct {
		name       string
		flag       string
		url        string
		wantStatus int
		wantCalled bool
		wantLookup bool
	}{
		{"no namespace", "", "/vms/info.json", http.StatusOK, true, false},
		{"existing namespace", "", "/vms/info.json?namespace=foo", http.StatusOK, true, true},
		{"cleared namespace", "", "/vms/info.json?namespace=gone", http.StatusNotFound, false, true},
		{"flag namespace", "pinned", "/vms/info.json?namespace=gone", http.StatusOK, true, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ran := stubNamespaces(t, "minimega", "foo")
			setNamespaceFlag(t, c.flag)

			called := false
			h := requireNamespace(func(w http.ResponseWriter, r *http.Request) {
				called = true
			})

			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodGet, c.url, nil))

			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if called != c.wantCalled {
				t.Errorf("handler called = %v, want %v", called, c.wantCalled)
			}
			if got := len(*ran) > 0; got != c.wantLookup {
				t.Errorf("looked up namespaces = %v, want %v (ran %q)", got, c.wantLookup, *ran)
			}

			header := rec.Header().Get(unknownNamespaceHeader)
			if (c.wantStatus == http.StatusNotFound) != (header != "") {
				t.Errorf("%v header = %q", unknownNamespaceHeader, header)
			}
		})
	}
}

// TestClearedNamespaceNotRecreated reproduces #1255: a page left open on a
// cleared namespace keeps polling /<namespace>/... and must not run any
// "namespace <name> ..." command, which would recreate the namespace.
func TestClearedNamespaceNotRecreated(t *testing.T) {
	ran := stubNamespaces(t, "minimega")
	setNamespaceFlag(t, "")

	// a mux of its own, so that the test can run again (-count) with the route
	oldMux := mux
	mux = http.NewServeMux()
	t.Cleanup(func() { mux = oldMux })

	const path = "/test-1255.json"
	mux.HandleFunc(path, mustAuth(func(w http.ResponseWriter, r *http.Request) {
		runTabularFunc(NewCommand(r))
	}))

	rec := httptest.NewRecorder()
	mustAuth(indexHandler)(rec, httptest.NewRequest(http.MethodGet, "/gone"+path, nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	for _, cmd := range *ran {
		if strings.Contains(cmd, `namespace "gone"`) {
			t.Errorf("ran %q, which recreates the namespace", cmd)
		}
	}

	// the same request for a namespace that exists still runs in it
	ran = stubNamespaces(t, "minimega", "foo")

	rec = httptest.NewRecorder()
	mustAuth(indexHandler)(rec, httptest.NewRequest(http.MethodGet, "/foo"+path, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(*ran) == 0 || !strings.Contains((*ran)[len(*ran)-1], `namespace "foo"`) {
		t.Errorf("command not run in namespace foo: %q", *ran)
	}
}

func TestIndexRedirectKeepsNamespace(t *testing.T) {
	setNamespaceFlag(t, "")

	// a mux of its own that records the namespace that /vms receives
	oldMux := mux
	mux = http.NewServeMux()
	t.Cleanup(func() { mux = oldMux })

	var got string
	mux.HandleFunc("/vms", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("namespace")
	})

	cases := []struct {
		url, location, namespace string
	}{
		{"/", "/vms", ""},
		{"/?namespace=foo", "/foo/vms", "foo"},
		{"/?namespace=a%2Fb", "/a%2Fb/vms", "a/b"},
		{"/?namespace=foo%20bar", "/foo%20bar/vms", "foo bar"},
	}

	for _, c := range cases {
		rec := httptest.NewRecorder()
		indexHandler(rec, httptest.NewRequest(http.MethodGet, c.url, nil))

		if rec.Code != http.StatusFound {
			t.Errorf("%v: status = %d, want %d", c.url, rec.Code, http.StatusFound)
		}
		loc := rec.Header().Get("Location")
		if loc != c.location {
			t.Errorf("%v: Location = %q, want %q", c.url, loc, c.location)
		}
		if c.namespace == "" {
			continue
		}

		// follow the redirect
		got = ""
		indexHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, loc, nil))
		if got != c.namespace {
			t.Errorf("%v: namespace = %q, want %q", loc, got, c.namespace)
		}
	}

	// a namespace with no trailing slash
	rec := httptest.NewRecorder()
	indexHandler(rec, httptest.NewRequest(http.MethodGet, "/a%2Fb", nil))
	if loc := rec.Header().Get("Location"); loc != "/a%2Fb/" {
		t.Errorf("/a%%2Fb: Location = %q, want %q", loc, "/a%2Fb/")
	}
}

// TestRequireNamespaceLookupFails checks that a failed lookup, for example
// when minimega is down, is not reported as an unknown namespace.
func TestRequireNamespaceLookupFails(t *testing.T) {
	setNamespaceFlag(t, "")
	useMinimega(t, nil)

	called := false
	h := requireNamespace(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/vms/info.json?namespace=foo", nil))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if called {
		t.Error("handler called after a failed lookup")
	}
	if header := rec.Header().Get(unknownNamespaceHeader); header != "" {
		t.Errorf("%v header = %q after a failed lookup", unknownNamespaceHeader, header)
	}
}

// useMinimega points miniweb at a fake minimega socket in a new directory.
// If resp is not nil, the fake answers each command with it.
func useMinimega(t *testing.T, resp *miniclient.Response) {
	t.Helper()

	base := t.TempDir()

	oldBase, oldMm := *f_base, mm
	t.Cleanup(func() {
		if mm != nil {
			mm.Close()
		}
		*f_base, mm = oldBase, oldMm
	})
	*f_base, mm = base, nil

	if resp == nil {
		return
	}

	l, err := net.Listen("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		dec, enc := json.NewDecoder(conn), json.NewEncoder(conn)
		for {
			var req miniclient.Request
			if err := dec.Decode(&req); err != nil {
				return
			}
			if err := enc.Encode(resp); err != nil {
				return
			}
		}
	}()
}

func TestListNamespaces(t *testing.T) {
	t.Run("minimega down", func(t *testing.T) {
		useMinimega(t, nil)

		if names, err := listNamespaces(); err == nil {
			t.Errorf("got %v and no error", names)
		}
	})

	t.Run("listing", func(t *testing.T) {
		useMinimega(t, &miniclient.Response{Resp: minicli.Responses{{
			Host:    "host",
			Header:  []string{"namespace", "vlans", "active"},
			Tabular: [][]string{{"minimega", "", "true"}, {"foo", "", "false"}},
		}}})

		names, err := listNamespaces()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(names, []string{"minimega", "foo"}) {
			t.Errorf("got %v", names)
		}
	})

	t.Run("error response", func(t *testing.T) {
		useMinimega(t, &miniclient.Response{Resp: minicli.Responses{{Host: "host", Error: "boom"}}})

		if names, err := listNamespaces(); err == nil || err.Error() != "boom" {
			t.Errorf("got %v, %v", names, err)
		}
	})
}
