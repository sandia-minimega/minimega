// Copyright 2012 The Ninep Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This code is imported from the old ninep repo,
// with some changes.

// +build !windows

package ufs

// resetDir seeks to the beginning of the file so that the file list can be
// read again.
func resetDir(f *File) error {
	_, err := f.File.Seek(0, SeekStart)
	return err
}
