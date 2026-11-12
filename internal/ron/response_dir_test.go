// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, base, subpath string) *Server {
	t.Helper()

	s, err := NewServer(base, subpath, nil)
	if err != nil {
		t.Fatalf("NewServer(%q, %q): %v", base, subpath, err)
	}

	return s
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%v: want not exist, got err = %v", path, err)
	}
}

func mustExist(t *testing.T, path string) os.FileInfo {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%v: want exist, got err = %v", path, err)
	}

	return info
}

// writeResponse sends a response through the response handler and waits until
// the handler records it on disk.
func writeStdoutResponse(t *testing.T, s *Server, id int, uuid, stdout string) {
	t.Helper()

	s.responses <- &Client{
		UUID: uuid,
		Responses: []*Response{
			{ID: id, Stdout: stdout, ExitCode: 0, RecordExitCode: true},
		},
	}

	path := filepath.Join(s.responsePath(&id), uuid, "stdout")
	deadline := time.Now().Add(5 * time.Second)

	// the file exists before its content is written, so wait for the content
	for {
		if data, err := os.ReadFile(path); err == nil && string(data) == stdout {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("response handler did not write %v", path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestNewServerDoesNotCreateResponseDir(t *testing.T) {
	base := t.TempDir()

	s := newTestServer(t, base, "ns")
	defer s.Destroy()

	mustNotExist(t, filepath.Join(base, "ns"))
}

func TestNewServerDefaultNamespaceDoesNotCreateResponseDir(t *testing.T) {
	base := t.TempDir()

	s := newTestServer(t, base, "")
	defer s.Destroy()

	mustExist(t, base)
	mustNotExist(t, filepath.Join(base, RESPONSE_PATH))
}

func TestReadersWithoutResponseDir(t *testing.T) {
	base := t.TempDir()

	s := newTestServer(t, base, "ns")
	defer s.Destroy()

	for _, raw := range []bool{false, true} {
		res, err := s.GetResponses(raw)
		if err != nil || res != "" {
			t.Errorf("GetResponses(%v) = %q, %v; want \"\", nil", raw, res, err)
		}
	}

	if _, err := s.GetResponse(1, false); err == nil || !strings.Contains(err.Error(), "no responses for 1") {
		t.Errorf("GetResponse(1) error = %v; want no responses for 1", err)
	}

	if err := s.DeleteResponse(1); err != nil {
		t.Errorf("DeleteResponse(1) = %v; want nil", err)
	}

	if err := s.ClearResponses(); err != nil {
		t.Errorf("ClearResponses() = %v; want nil", err)
	}

	mustNotExist(t, filepath.Join(base, "ns"))
}

func TestFilesSendFallsBackWithoutSubpathDir(t *testing.T) {
	base := t.TempDir()

	if err := os.WriteFile(filepath.Join(base, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newTestServer(t, base, "ns")
	defer s.Destroy()

	cmd, err := s.NewFilesSendCommand([]string{"file.txt"})
	if err != nil {
		t.Fatalf("NewFilesSendCommand: %v", err)
	}

	if len(cmd.FilesSend) != 1 || cmd.FilesSend[0] != "file.txt" {
		t.Fatalf("FilesSend = %v; want [file.txt]", cmd.FilesSend)
	}
}

func TestResponseWriteCreatesResponseDir(t *testing.T) {
	base := t.TempDir()

	// a folder made with the old NewServer mode, to compare after the umask
	ref := filepath.Join(base, "ref")
	if err := os.Mkdir(ref, 0o775); err != nil {
		t.Fatal(err)
	}
	want := mustExist(t, ref).Mode().Perm()

	s := newTestServer(t, base, "ns")

	writeStdoutResponse(t, s, 1, "client-1", "hello")

	for _, dir := range []string{filepath.Join(base, "ns"), s.responsePath(nil)} {
		if got := mustExist(t, dir).Mode().Perm(); got != want {
			t.Errorf("%v: mode = %v; want %v", dir, got, want)
		}
	}

	res, err := s.GetResponses(false)
	if err != nil {
		t.Fatalf("GetResponses: %v", err)
	}

	if !strings.Contains(res, "hello") {
		t.Errorf("GetResponses = %q; want it to contain hello", res)
	}

	if code, err := s.GetExitCode(1, "client-1"); err == nil {
		t.Errorf("GetExitCode for unknown client = %v, nil; want error", code)
	}

	s.Destroy()

	// responses remain after destroy, so the subpath folder stays
	mustExist(t, filepath.Join(s.responsePath(nil), "1", "client-1", "stdout"))
}

func TestFileRecvCreatesResponseDir(t *testing.T) {
	base := t.TempDir()

	s := newTestServer(t, base, "ns")
	defer s.Destroy()

	f := &File{ID: 2, Name: "out/file.txt", Perm: 0o660, Data: []byte("data"), EOF: true}
	if err := s.recvFile("client-1", f); err != nil {
		t.Fatalf("recvFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(base, "ns", RESPONSE_PATH, "2", "client-1", "out", "file.txt"))
	if err != nil || string(got) != "data" {
		t.Fatalf("received file = %q, %v; want data", got, err)
	}
}

func TestDestroyRemovesEmptySubpathDir(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, s *Server) error
	}{
		{"no responses", func(t *testing.T, s *Server) error { return nil }},
		{"cleared responses", func(t *testing.T, s *Server) error {
			writeStdoutResponse(t, s, 1, "client-1", "hello")
			return s.ClearResponses()
		}},
		{"deleted response", func(t *testing.T, s *Server) error {
			writeStdoutResponse(t, s, 1, "client-1", "hello")
			return s.DeleteResponse(1)
		}},
		{"empty response dir", func(t *testing.T, s *Server) error {
			return os.MkdirAll(s.responsePath(nil), 0o775)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()

			s := newTestServer(t, base, "ns")

			if err := tt.setup(t, s); err != nil {
				t.Fatal(err)
			}

			s.Destroy()

			mustNotExist(t, filepath.Join(base, "ns"))
			mustExist(t, base)
		})
	}
}

func TestDestroyKeepsNonEmptySubpathDir(t *testing.T) {
	base := t.TempDir()
	keep := filepath.Join(base, "ns", "minirouter-r1")

	s := newTestServer(t, base, "ns")

	if err := os.MkdirAll(filepath.Dir(keep), 0o775); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(keep, []byte("conf"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.Destroy()

	mustExist(t, keep)
}

func TestDestroyNeverRemovesBaseDir(t *testing.T) {
	for _, subpath := range []string{"", ".", "ns/.."} {
		t.Run(subpath, func(t *testing.T) {
			base := t.TempDir()

			s := newTestServer(t, base, subpath)
			s.Destroy()

			mustExist(t, base)
		})
	}
}

// A subpath that climbs out of the base must not let Destroy remove the base
// itself or an empty folder next to it.
func TestDestroyStaysInsideBaseDir(t *testing.T) {
	t.Run("response folder is the base", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), RESPONSE_PATH)
		if err := os.Mkdir(base, 0775); err != nil {
			t.Fatal(err)
		}

		s := newTestServer(t, base, "..")
		s.Destroy()

		mustExist(t, base)
	})

	t.Run("response folder is next to the base", func(t *testing.T) {
		root := t.TempDir()
		base := filepath.Join(root, "files")
		sibling := filepath.Join(root, "other", RESPONSE_PATH)

		for _, dir := range []string{base, sibling} {
			if err := os.MkdirAll(dir, 0775); err != nil {
				t.Fatal(err)
			}
		}

		s := newTestServer(t, base, "../other")
		s.Destroy()

		mustExist(t, sibling)
	})
}

func TestDestroyIsIdempotent(t *testing.T) {
	base := t.TempDir()

	s := newTestServer(t, base, "ns")
	s.Destroy()
	s.Destroy()

	mustNotExist(t, filepath.Join(base, "ns"))
	mustExist(t, base)
}
