// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// transferDirPrefix starts the name of every temporary transfer directory.
const transferDirPrefix = "transfer_"

// preallocate reserves size bytes for f; a variable so tests can simulate a full disk.
var preallocate = preallocateFile

// newTransfer creates the temporary directory and file that the parts of msg
// are written into. When the size of the file is known, the space for the
// whole file is reserved up front so that a full disk is reported before the
// transfer starts instead of truncating the file at the end.
func (iom *IOMeshage) newTransfer(msg *Message) (*Transfer, error) {
	dir, err := os.MkdirTemp(iom.base, iom.transferPrefix())
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, filepath.Base(msg.Filename))

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	if msg.Size > 0 {
		if err := preallocate(f, msg.Size); err != nil {
			f.Close()
			os.RemoveAll(dir)
			return nil, fmt.Errorf("reserving %v bytes in %v: %w", msg.Size, iom.base, err)
		}
	}

	// queued transfers keep no open file, so a large directory cannot use up
	// the file descriptors of the process
	if err := f.Close(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	return &Transfer{
		Dir:      dir,
		Filename: msg.Filename,
		Parts:    make(map[int64]bool),
		NumParts: int(msg.Part),
		Size:     msg.Size,
		Inflight: -1,
		Queued:   true,
		path:     path,
	}, nil
}

// transferPrefix names this node's transfer directories, so that nodes
// sharing a files directory never remove each other's.
func (iom *IOMeshage) transferPrefix() string {
	return transferDirPrefix + iom.name + "_"
}

// transferDirOwner returns the node that created the transfer directory with
// the given name. Names have the form transfer_<node>_<digits>, and the
// random digits from os.MkdirTemp never contain an underscore.
func transferDirOwner(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, transferDirPrefix)
	if !ok {
		return "", false
	}

	i := strings.LastIndexByte(rest, '_')
	if i <= 0 || i == len(rest)-1 {
		return "", false
	}

	for _, c := range rest[i+1:] {
		if c < '0' || c > '9' {
			return "", false
		}
	}

	return rest[:i], true
}

// transferDir returns the node that owns name, a top-level entry of iom.base,
// if the entry is a transfer directory.
func (iom *IOMeshage) transferDir(name string) (string, bool) {
	owner, ok := transferDirOwner(name)
	if !ok {
		return "", false
	}

	fi, err := os.Lstat(filepath.Join(iom.base, name))
	if err != nil || !fi.IsDir() {
		return "", false
	}

	return owner, true
}

// removeStaleTransfers removes transfer directories that this node left
// behind, for example when it was killed during a transfer.
func (iom *IOMeshage) removeStaleTransfers() {
	entries, err := os.ReadDir(iom.base)
	if err != nil {
		log.Error("finding stale transfers: %v", err)
		return
	}

	for _, e := range entries {
		if owner, ok := iom.transferDir(e.Name()); !ok || owner != iom.name {
			continue
		}

		dir := filepath.Join(iom.base, e.Name())
		log.Info("removing stale transfer directory %v", dir)
		if err := os.RemoveAll(dir); err != nil {
			log.Error("removing stale transfer directory: %v", err)
		}
	}
}

// isTransferPath reports whether path is inside a transfer directory.
func (iom *IOMeshage) isTransferPath(path string) bool {
	rel, err := filepath.Rel(iom.base, path)
	if err != nil {
		return false
	}

	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	_, ok := iom.transferDir(first)
	return ok
}

// checkPart returns an error if data cannot be part p of the transfer.
func (t *Transfer) checkPart(p int64, data []byte) error {
	return checkPart(p, data, int64(t.NumParts), t.Size)
}

// checkPart returns an error if data cannot be part p of a file with numParts
// parts and size bytes (0 if unknown). Every part but the last is exactly
// PART_SIZE bytes long.
func checkPart(p int64, data []byte, numParts, size int64) error {
	n := int64(len(data))

	switch {
	case p < 0 || p >= numParts:
		return fmt.Errorf("part %v out of range [0, %v)", p, numParts)
	case n == 0 || n > PART_SIZE:
		return fmt.Errorf("part %v has invalid length %v", p, n)
	case p < numParts-1 && n != PART_SIZE:
		return fmt.Errorf("part %v is %v bytes, expected %v", p, n, PART_SIZE)
	case p == numParts-1 && size > 0 && n != size-p*PART_SIZE:
		return fmt.Errorf("last part %v is %v bytes, expected %v", p, n, size-p*PART_SIZE)
	}

	return nil
}

// writePart writes part p into the transfer file. transferLock must be held.
func (t *Transfer) writePart(p int64, data []byte) error {
	if err := t.checkPart(p, data); err != nil {
		return err
	}

	f, err := os.OpenFile(t.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	if _, err := f.WriteAt(data, p*PART_SIZE); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	t.Parts[p] = true

	return nil
}

// readPart reads a completed part back from the transfer file.
func (t *Transfer) readPart(p int64) ([]byte, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := readPartAt(f, p)
	if err != nil {
		return nil, err
	}

	if err := t.checkPart(p, data); err != nil {
		return nil, err
	}

	return data, nil
}

// finishTransfer moves a completed transfer file into place.
func (iom *IOMeshage) finishTransfer(t *Transfer, perm os.FileMode) error {
	// flush to disk so that write errors are reported before the file is visible
	if err := syncFile(t.path); err != nil {
		return err
	}

	// set the final mode while the file is still private to the transfer
	if err := os.Chmod(t.path, perm); err != nil {
		return err
	}

	fullPath := filepath.Join(iom.base, t.Filename)

	// create subdirectories
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	// Give the file system watcher time to start watching the directory (if it's
	// not already) before moving the file into it.
	time.Sleep(500 * time.Millisecond)

	return os.Rename(t.path, fullPath)
}

// syncFile flushes the contents of the named file to disk.
func syncFile(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}
