// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func writeResponse(t *testing.T, s *Server, id int, uuid string, files map[string]string) {
	t.Helper()

	dir := filepath.Join(s.responsePath(&id), uuid)
	if err := os.MkdirAll(dir, 0775); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0775); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0660); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetClientResponses(t *testing.T) {
	s := &Server{path: t.TempDir()}

	// 10 sorts after 2 numerically but before it lexically
	writeResponse(t, s, 10, "aaa", map[string]string{"stdout": "ten"})
	writeResponse(t, s, 2, "bbb", map[string]string{"stdout": "b-out", "stderr": "b-err", "exitcode": "7"})
	writeResponse(t, s, 2, "aaa", map[string]string{
		"stdout":   "1/aaa/stdout:\nfoo/bar\n",
		"stderr":   "2/bbb/stderr:\n",
		"exitcode": "0",
		"recv.txt": "received file",
	})

	want := []ClientResponse{
		{ID: 2, UUID: "aaa", Stdout: "1/aaa/stdout:\nfoo/bar\n", Stderr: "2/bbb/stderr:\n"},
		{ID: 2, UUID: "bbb", Stdout: "b-out", Stderr: "b-err"},
		{ID: 10, UUID: "aaa", Stdout: "ten"},
	}

	got, err := s.GetClientResponses(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("all: got %#v, want %#v", got, want)
	}

	id := 2
	got, err = s.GetClientResponses(&id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want[:2]) {
		t.Fatalf("id 2: got %#v, want %#v", got, want[:2])
	}
}

func TestGetClientResponsesReceivedDirs(t *testing.T) {
	s := &Server{path: t.TempDir()}

	// cc recv of /stdout/report.txt and /stderr/log.txt creates directories
	writeResponse(t, s, 1, "aaa", map[string]string{
		"stdout/report.txt": "report",
		"stderr/log.txt":    "log",
	})
	writeResponse(t, s, 2, "aaa", map[string]string{"stdout": "out"})

	want := []ClientResponse{
		{ID: 1, UUID: "aaa"},
		{ID: 2, UUID: "aaa", Stdout: "out"},
	}

	got, err := s.GetClientResponses(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestGetClientResponsesMissing(t *testing.T) {
	s := &Server{path: t.TempDir()}

	got, err := s.GetClientResponses(nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v; want empty non-nil slice", got, err)
	}

	id := 5
	if _, err := s.GetClientResponses(&id); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestGetClientInfoSorted(t *testing.T) {
	s := &Server{clients: map[string]*client{}}
	for _, u := range []string{"c", "a", "b"} {
		s.clients[u] = &client{Client: &Client{UUID: u, IPs: []string{"10.0.0." + strconv.Itoa(len(u))}}}
	}

	got := s.GetClientInfo()
	if len(got) != 3 || got[0].UUID != "a" || got[1].UUID != "b" || got[2].UUID != "c" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestDataTypesGobRoundTrip(t *testing.T) {
	gob.Register([]Command{})
	gob.Register([]ClientInfo{})
	gob.Register([]ClientResponse{})

	for _, v := range []interface{}{
		[]Command{{ID: 1, Command: []string{"echo", "hi"}, Filter: &Filter{UUID: "a"}}},
		[]ClientInfo{{UUID: "a", IPs: []string{"10.0.0.1"}}},
		[]ClientResponse{{ID: 1, UUID: "a", Stdout: "out", Stderr: "err"}},
	} {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(&struct{ Data interface{} }{v}); err != nil {
			t.Fatal(err)
		}

		var out struct{ Data interface{} }
		if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out.Data, v) {
			t.Fatalf("got %#v, want %#v", out.Data, v)
		}
	}
}
