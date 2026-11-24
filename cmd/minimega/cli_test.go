// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/iomeshage"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
)

// useFilesDir points the files directory and iom at an empty directory on a
// mesh of one node, and restores them when the test ends.
func useFilesDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	mesh, err := iomeshage.New(dir, &meshage.Node{}, "", false)
	if err != nil {
		t.Fatal(err)
	}

	oldBase, oldIom := *f_iomBase, iom
	*f_iomBase, iom = dir, mesh
	t.Cleanup(func() { *f_iomBase, iom = oldBase, oldIom })

	return dir
}

// urlServer serves body at each path that ends with /images/disk.qc2 and
// counts the requests.
func urlServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		if !strings.HasSuffix(r.URL.Path, "/images/disk.qc2") {
			http.NotFound(w, r)
			return
		}

		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv, &requests
}

// TestPreprocessDownloadsURL checks that an http argument becomes the path
// of the downloaded file, and that a second use does not download again.
func TestPreprocessDownloadsURL(t *testing.T) {
	dir := useFilesDir(t)
	srv, requests := urlServer(t, "disk image")

	want := filepath.Join(dir, "images", "disk.qc2")

	for i := 0; i < 2; i++ {
		got, err := cliPreprocess(srv.URL + "/images/disk.qc2")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("cliPreprocess = %q, want %q", got, want)
		}

		data, err := os.ReadFile(want)
		if err != nil || string(data) != "disk image" {
			t.Fatalf("downloaded file = %q, %v", data, err)
		}
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

// TestPreprocessURLNotFound checks that a failed download returns an error
// and leaves no file, so that a later use does not find an empty file.
func TestPreprocessURLNotFound(t *testing.T) {
	dir := useFilesDir(t)
	srv, _ := urlServer(t, "disk image")

	if got, err := cliPreprocess(srv.URL + "/images/missing.qc2"); err == nil {
		t.Errorf("cliPreprocess = %q, want an error", got)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "images"))
	if err == nil && len(entries) != 0 {
		t.Errorf("the failed download left %v", entries[0].Name())
	}
}

// TestFetchURLLookup checks what fetchURL does with each result of the mesh
// lookup.
func TestFetchURLLookup(t *testing.T) {
	notFound := fmt.Errorf("get /images/disk.qc2: %w", iomeshage.ErrNotFound)
	timeout := errors.New("timeout")

	for _, tt := range []struct {
		name     string
		lookup   error
		download bool
	}{
		{"no node has the file", notFound, true},
		{"the lookup fails", timeout, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := useFilesDir(t)
			srv, requests := urlServer(t, "disk image")

			got, err := fetchURL(srv.URL+"/images/disk.qc2", "/images/disk.qc2", func(file, updatee string) (string, error) {
				return "", tt.lookup
			})

			if tt.download {
				if want := filepath.Join(dir, "images", "disk.qc2"); err != nil || got != want {
					t.Errorf("fetchURL = %q, %v, want %q", got, err, want)
				}
			} else if !errors.Is(err, tt.lookup) {
				t.Errorf("fetchURL = %q, %v, want %v", got, err, tt.lookup)
			}

			if n := requests.Load(); (n == 1) != tt.download {
				t.Errorf("requests = %d, want a download: %v", n, tt.download)
			}
		})
	}
}

// TestFetchURLStaysInFilesDir checks that ".." in the path of a URL does not
// put the download outside the files directory.
func TestFetchURLStaysInFilesDir(t *testing.T) {
	dir := useFilesDir(t)
	srv, _ := urlServer(t, "disk image")

	got, err := fetchURL(srv.URL+"/images/disk.qc2", "/../../outside.qc2", func(file, updatee string) (string, error) {
		return "", iomeshage.ErrNotFound
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "outside.qc2"); got != want {
		t.Errorf("fetchURL = %q, want %q", got, want)
		os.Remove(got)
	}
}

// TestPreprocessURLPathInFilesDir checks that a URL whose path starts with
// the path of the files directory is downloaded one time. The mesh lookup
// removes that prefix, so the lookup and the download must use one name.
func TestPreprocessURLPathInFilesDir(t *testing.T) {
	dir := useFilesDir(t)
	srv, requests := urlServer(t, "disk image")

	var first string

	for i := 0; i < 2; i++ {
		got, err := cliPreprocess(srv.URL + dir + "/images/disk.qc2")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(got); err != nil {
			t.Fatal(err)
		}

		if i == 0 {
			first = got
		} else if got != first {
			t.Errorf("second use = %q, want %q", got, first)
		}
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

// TestPreprocessURLRelativeFilesDir checks that a URL whose path starts with
// a relative files directory is downloaded one time, to the file that the
// mesh lookup finds.
func TestPreprocessURLRelativeFilesDir(t *testing.T) {
	dir := useFilesDir(t)
	t.Chdir(filepath.Dir(dir))
	*f_iomBase = filepath.Base(dir)

	srv, requests := urlServer(t, "disk image")
	want := filepath.Join(*f_iomBase, "images", "disk.qc2")

	for i := 0; i < 2; i++ {
		got, err := cliPreprocess(srv.URL + "/" + *f_iomBase + "/images/disk.qc2")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("cliPreprocess = %q, want %q", got, want)
		}
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

// TestWgetKeepsUmask checks that the umask limits the mode of a downloaded
// file.
func TestWgetKeepsUmask(t *testing.T) {
	dir := t.TempDir()
	srv, _ := urlServer(t, "disk image")

	for _, tt := range []struct {
		umask int
		want  os.FileMode
	}{
		{0022, 0644},
		{0077, 0600},
	} {
		dst := filepath.Join(dir, fmt.Sprintf("%04o.qc2", tt.umask))

		old := syscall.Umask(tt.umask)
		err := wget(srv.URL+"/images/disk.qc2", dst)
		syscall.Umask(old)

		if err != nil {
			t.Fatal(err)
		}

		fi, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != tt.want {
			t.Errorf("umask %04o: mode = %04o, want %04o", tt.umask, got, tt.want)
		}
	}
}
