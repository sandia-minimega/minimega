// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"os"
	"path/filepath"
	"ron"
	"time"
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
func recvFiles(files []string) {
	start := time.Now()
	var size int64

	for _, v := range files {
		log.Info("requesting file %v", v)

		dst := filepath.Join(*f_path, "files", v)

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

		// recv all parts of this file
		for {
			resp := <-client.fileChan
			if resp.File.Name != v {
				log.Error("filename mismatch: %v != %v", resp.File.Name, v)
				break
			}

			// unable to retrieve this file
			if resp.Error != "" {
				log.Error("%v", resp.Error)
				break
			}

			if err := resp.File.Recv(dst); err != nil {
				log.Errorln(err)
				break
			}

			size += int64(len(resp.File.Data))

			if resp.File.EOF {
				break
			}
		}
	}

	d := time.Since(start)
	rate := (float64(size) / 1024 / d.Seconds())

	log.Debug("received %v bytes in %v (%v KBps)", size, d, rate)

	return
}
