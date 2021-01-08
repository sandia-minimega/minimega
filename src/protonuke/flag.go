// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type FileSize uint

// DefaultFileSize is 3MB
const DefaultFileSize = FileSize(3 * 1 << 20)

// DefaultFTPFileSize is 500KB
const DefaultFTPFileSize = FileSize(500 * 1 << 10)

func ParseFileSize(s string) (FileSize, error) {
	for i, suffix := range []string{"B", "KB", "MB"} {
		if strings.HasSuffix(s, suffix) {
			v, err := strconv.Atoi(strings.TrimSuffix(s, suffix))
			if err != nil {
				continue
			}

			return FileSize(v * (1 << uint(10*i))), nil
		}
	}

	// Assume MB
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("invalid file size, expected value[B,KB,MB]")
	}

	return FileSize(v * (1 << 20)), nil
}

func (f *FileSize) Set(s string) (err error) {
	*f, err = ParseFileSize(s)
	return
}

func (f *FileSize) String() string {
	for i, suffix := range []string{"B", "KB", "MB"} {
		if *f < 2<<uint(10*(i+1)) {
			return fmt.Sprintf("%v%v", *f/(1<<uint(10*i)), suffix)
		}
	}

	// Default is MB
	return fmt.Sprintf("%vMB", *f/(1<<20))
}
