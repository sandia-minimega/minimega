// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
)

// InitrdWithFile writes dst as a copy of the initrd src with one more file,
// name, that holds data. The kernel unpacks each archive of an initramfs in
// turn, so the file is an extra gzip cpio archive after the original one and
// no rebuild of the image is necessary. The directory of name must exist in
// src.
func InitrdWithFile(src, dst, name string, data []byte) error {
	base, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Write(base)
	// the kernel skips zero bytes between archives, and wants 4-byte alignment
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}

	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(cpioArchive(name, data)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	return os.WriteFile(dst, buf.Bytes(), 0644)
}

// cpioArchive returns a cpio archive in the "newc" format with one regular
// file.
func cpioArchive(name string, data []byte) []byte {
	var buf bytes.Buffer

	entry := func(name string, mode int, data []byte) {
		// magic, then 13 fields of 8 hex digits: ino, mode, uid, gid, nlink,
		// mtime, filesize, devmajor, devminor, rdevmajor, rdevminor,
		// namesize (with the NUL), check
		fmt.Fprintf(&buf, "070701%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
			0, mode, 0, 0, 1, 0, len(data), 0, 0, 0, 0, len(name)+1, 0)
		buf.WriteString(name)
		buf.WriteByte(0)
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
		buf.Write(data)
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
	}

	entry(name, 0100644, data)
	entry("TRAILER!!!", 0, nil)

	return buf.Bytes()
}
