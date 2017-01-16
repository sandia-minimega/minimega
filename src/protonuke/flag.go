// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

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

func (f *FileSize) Set(s string) error {
	for i, suffix := range []string{"B", "KB", "MB"} {
		if strings.HasSuffix(s, suffix) {
			v, err := strconv.Atoi(strings.TrimSuffix(s, suffix))
			if err != nil {
				continue
			}

			*f = FileSize(v * (1 << uint(10*i)))
			return nil
		}
	}

	// Assume MB
	v, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("invalid file size, expected value[B,KB,MB]")
	}

	*f = FileSize(v * (1 << 20))
	return nil
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
