// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// FileInfo is used by the calling API to describe existing files.
type FileInfo struct {
	// Path is the absolute path to the file
	Path string

	// Size of the file
	Size int64

	// Modification time of the file
	ModTime time.Time

	// Murmur3 hash of the file
	Hash string

	// embed
	os.FileMode
}

func newFileInfo(path, hash string, fi os.FileInfo) FileInfo {
	return FileInfo{
		Path:     path,
		Size:     fi.Size(),
		ModTime:  fi.ModTime(),
		Hash:     hash,
		FileMode: fi.Mode(),
	}
}

func (f FileInfo) numParts() int64 {
	if f.IsDir() {
		return 0
	}

	return (f.Size + PART_SIZE - 1) / PART_SIZE
}

func (iom *IOMeshage) Rel(info FileInfo) string {
	rel, err := filepath.Rel(iom.base, info.Path)
	if err != nil {
		log.Error("file info from outside iomBase: %v", info.Path)
		return ""
	}

	return rel

}

// List files and directories on the local node. List on a file returns the
// info for that file only. List on a directory returns the contents of that
// directory. Supports expanding globs. When recursive is true, reports all
// files and directories below the specified path(s).
func (iom *IOMeshage) List(path string, recurse bool) ([]FileInfo, error) {
	glob, err := filepath.Glob(iom.cleanPath(path))
	if err != nil {
		return nil, err
	}

	var res []FileInfo

	for _, f := range glob {
		info, err := os.Stat(f)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			res = append(res, newFileInfo(f, iom.getHash(f), info))
			continue
		}

		if !recurse {
			files, err := ioutil.ReadDir(f)
			if err != nil {
				return nil, err
			}

			for _, info := range files {
				path := filepath.Join(f, info.Name())
				res = append(res, newFileInfo(path, iom.getHash(path), info))
			}

			continue
		}

		// recurse on a directory
		err = filepath.Walk(f, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				res = append(res, newFileInfo(path, iom.getHash(path), info))
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

// Delete a file or directory on the local node. Supports Globs.
func (iom *IOMeshage) Delete(path string) error {
	glob, err := filepath.Glob(iom.cleanPath(path))
	if err != nil {
		return err
	}

	for _, f := range glob {
		if f == iom.base {
			// the user *probably* doesn't want to actually remove the iom.base
			// directory since them they wouldn't be able to transfer any more
			// files. Instead, remove all its contents.
			log.Info("deleting iomeshage directory contents")
			files, err := ioutil.ReadDir(f)
			if err != nil {
				return err
			}

			for _, file := range files {
				if err := os.RemoveAll(filepath.Join(f, file.Name())); err != nil {
					return err
				}
			}
		}

		if err := os.RemoveAll(f); err != nil {
			return err
		}
	}

	return nil
}

// cleanPath returns the a full path rooted in the iom base directory.
func (iom *IOMeshage) cleanPath(path string) string {
	// prepend a "/" to the directory so that commands can't affect files above
	// the iom.base directory. For example, filepath.Clean will replace "/../"
	// with "/".
	path = filepath.Join(iom.base, filepath.Clean("/"+path))
	log.Debug("cleaned path is %v", path)

	return path
}

// Read a filepart and return a byteslice.
func (iom *IOMeshage) readPart(filename string, part int64) []byte {
	if !strings.HasPrefix(filename, iom.base) {
		filename = filepath.Join(iom.base, filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		log.Errorln(err)
		return nil
	}
	defer f.Close()

	// we do have the file, calculate the number of parts
	fi, err := f.Stat()
	if err != nil {
		log.Errorln(err)
		return nil
	}

	parts := (fi.Size() + PART_SIZE - 1) / PART_SIZE // integer divide with ceiling instead of floor
	if part > parts {
		log.Errorln("attempt to read beyond file")
		return nil
	}

	// read up to PART_SIZE
	data := make([]byte, PART_SIZE)
	n, err := f.ReadAt(data, part*PART_SIZE)

	if err != nil {
		if err != io.EOF {
			log.Errorln(err)
			return nil
		}
	}

	return data[:n]
}

func (iom *IOMeshage) getHash(path string) string {
	iom.hashLock.RLock()
	defer iom.hashLock.RUnlock()

	return iom.hashes[path]
}

func (iom *IOMeshage) updateHash(path, hash string) {
	iom.hashLock.Lock()
	defer iom.hashLock.Unlock()

	iom.hashes[path] = hash
}

// stream reads a file from the local node's filesystem and returns the parts
// via a channel.
func stream(fname string) (chan []byte, error) {
	out := make(chan []byte)

	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}

	go func() {
		defer f.Close()
		defer close(out)

		for {
			buf := make([]byte, PART_SIZE)

			n, err := f.Read(buf)
			if err == io.EOF {
				log.Info("finished streaming: %v", fname)
				return
			} else if err != nil {
				log.Error("streaming %v failed: %v", fname, err)
				return
			}

			out <- buf[:n]
		}
	}()

	return out, nil
}

// touch creates an empty file and all its parent directories.
func touch(fname string, perm os.FileMode) error {
	// create parent directories
	if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
		return err
	}

	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	f.Close()

	log.Debug("changing permissions: %v %v", fname, perm)
	return os.Chmod(fname, perm)
}
