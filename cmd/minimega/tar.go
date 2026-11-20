// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// tarSources maps directories extracted by the tar: prefix to their tarballs.
var tarSources = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

var errTarTopLevel = errors.New("unable to handle tar without a single top-level directory")

// tarLock serializes extractions so launches never see a partial filesystem.
var tarLock sync.Mutex

// tarPreprocess implements the tar: prefix. It fetches the tarball via
// iomeshage if needed, extracts it, and returns the extracted directory.
func tarPreprocess(v string) (string, error) {
	// strip the scheme by hand: url.Parse treats "tar:/abs/path" as a path
	// rather than an opaque string and would decode escapes such as %20
	path := v[len("tar:"):]

	tarball, err := fetchTarball(path, "")
	if err != nil {
		return "", err
	}

	dst, extracted, err := untar(tarball)
	if err != nil {
		return "", err
	}

	// hosts fetch tarballs in the iomeshage directory by their relative name
	src := tarball
	if rel, err := filepath.Rel(*f_iomBase, tarball); err == nil && !strings.HasPrefix(rel, "..") {
		src = rel
	}

	tarSources.Lock()
	defer tarSources.Unlock()

	if prev, ok := tarSources.m[dst]; ok && prev != src && !extracted {
		// dst still holds prev's contents, so keep sending prev to other hosts
		log.Warn("tar: %v already exists from %v, not extracting %v", dst, prev, src)
		return dst, nil
	}

	tarSources.m[dst] = src

	return dst, nil
}

// tarSource returns the tarball that the tar: prefix extracted to dir, if any.
func tarSource(dir string) string {
	tarSources.Lock()
	defer tarSources.Unlock()

	return tarSources.m[dir]
}

// fetchTarball returns a tarball's local path, fetching it via iomeshage if it
// is relative or in the iomeshage directory.
func fetchTarball(path, updatee string) (string, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, *f_iomBase) {
		return iomHelper(path, updatee)
	}

	return path, nil
}

// isDir reports whether path is a directory, without following symlinks.
func isDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir()
}

// untar extracts a tarball with a single top-level directory next to itself,
// unless already extracted, and returns the path to that directory and
// whether this call extracted it.
func untar(tarball string) (string, bool, error) {
	cmd := exec.Command("tar", "-tf", tarball)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// list stdout only: tar warns on stderr, e.g. about absolute member names
	out, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("unable to list tar %v: %v: %v", tarball, err, strings.TrimSpace(stderr.String()))
	}

	top, err := tarTopLevel(string(out))
	if err != nil {
		return "", false, fmt.Errorf("%v: %v", tarball, err)
	}

	dir := filepath.Dir(tarball)
	dst := filepath.Join(dir, top)

	tarLock.Lock()
	defer tarLock.Unlock()

	// check to see if we already extracted this tar
	if _, err := os.Lstat(dst); err == nil {
		if !isDir(dst) {
			return "", false, fmt.Errorf("%v: %v is not a directory", tarball, dst)
		}

		return dst, false, nil
	}

	log.Debug("untar %v to %v", tarball, dst)

	// extract to a temporary directory so a failure never leaves a partial dst
	tmp, err := os.MkdirTemp(dir, ".untar_")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmp)

	if out, err := processWrapper("tar", "-C", tmp, "-xf", tarball); err != nil {
		return "", false, fmt.Errorf("unable to untar %v: %v: %v", tarball, err, strings.TrimSpace(out))
	}

	// a symlink here could point the container at a host directory
	if !isDir(filepath.Join(tmp, top)) {
		return "", false, fmt.Errorf("%v: top-level entry %v is not a directory", tarball, top)
	}

	if err := os.Rename(filepath.Join(tmp, top), dst); err != nil {
		// another host sharing this directory may have extracted it first
		if isDir(dst) {
			return dst, false, nil
		}
		return "", false, err
	}

	return dst, true, nil
}

// tarTopLevel returns the single top-level entry in the output of `tar -t`.
func tarTopLevel(listing string) (string, error) {
	var top string

	for _, name := range strings.Split(listing, "\n") {
		name = strings.TrimLeft(name, "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}

		t, _, _ := strings.Cut(name, "/")
		if t == "" || t == "." {
			continue
		}

		if top != "" && t != top {
			return "", errTarTopLevel
		}

		top = t
	}

	if top == "" || top == ".." {
		return "", errTarTopLevel
	}

	return top, nil
}
