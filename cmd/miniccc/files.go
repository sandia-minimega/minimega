// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// sendFiles reads the files and sends them in multiple chunks to the server.
func sendFiles(ID int, files []string) {
	// expand and try to read each of the files
	for _, f := range files {
		log.Info("sending file %v", f)

		names, err := filepath.Glob(f)
		if err != nil {
			log.Errorln(err)
			continue
		}

		for _, name := range names {
			if err := sendFile(ID, name); err != nil {
				log.Errorln(err)
				continue
			}
		}
	}
}

func sendFile(ID int, filename string) error {
	log.Debug("sendFile: %v for command %v", filename, ID)

	// TODO: Change PART_SIZE based on memory size?
	return ron.SendFile("/", filename, ID, ron.PART_SIZE, sendMessage)
}

// recvFiles retrieves a list of files from the ron server by requesting each
// one individually.
func recvFiles(files []string, destination string) {
	start := time.Now()
	var size int64
	destDir := recvDir(*f_path, destination)

	for _, v := range files {
		log.Info("requesting file %v", v)

		dst := filepath.Join(destDir, v)

		if _, err := os.Stat(dst); err == nil {
			// file exists (TODO: overwrite?)
			log.Info("skipping %v -- already exists", dst)
			continue
		}

		m := &ron.Message{
			Type: ron.MESSAGE_FILE,
			UUID: client.UUID,
			File: &ron.File{
				Name: v,
			},
		}

		if err := sendMessage(m); err != nil {
			log.Error("send failed: %v", err)
			return
		}

		size += recvParts(client.fileChan, v, dst)
	}

	d := time.Since(start)
	rate := (float64(size) / 1024 / d.Seconds())

	log.Debug("received %v bytes in %v (%v KBps)", size, d, rate)

	return
}

// recvDir returns the client directory that receives sent files. A relative
// destination is below the files directory in base.
func recvDir(base, destination string) string {
	dir := filepath.Join(base, "files")

	if destination == "" {
		return dir
	}

	// On Windows, IsAbs is false for a path such as \opt that has no drive.
	if filepath.IsAbs(destination) || os.IsPathSeparator(destination[0]) {
		return filepath.Clean(destination)
	}

	return filepath.Join(dir, destination)
}

// recvParts writes the parts of file name from parts to dst and returns the
// number of bytes written. It reads parts until the final part or an error
// from the server, also after a write fails, so that the parts of this file
// do not reach the next transfer.
func recvParts(parts <-chan *ron.Message, name, dst string) int64 {
	var size int64
	var failed bool

	for {
		resp := <-parts
		if resp.File.Name != name {
			log.Error("filename mismatch: %v != %v", resp.File.Name, name)
			return size
		}

		// unable to retrieve this file
		if resp.Error != "" {
			log.Error("%v", resp.Error)
			return size
		}

		if !failed {
			if err := resp.File.Recv(dst); err != nil {
				log.Errorln(err)
				failed = true
			} else {
				size += int64(len(resp.File.Data))
			}
		}

		if resp.File.EOF {
			return size
		}
	}
}
