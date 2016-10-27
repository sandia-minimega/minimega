// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"io/ioutil"
	log "minilog"
	"os"
	"path/filepath"
	"ron"
	"time"
)

// readFiles reads the contents of the specified files so that they can be sent
// to the ron server.
func readFiles(files []*ron.File) []*ron.File {
	var res []*ron.File

	// expand and try to read each of the files
	for _, f := range files {
		log.Info("sending file %v", f.Name)

		names, err := filepath.Glob(f.Name)
		if err != nil {
			log.Errorln(err)
			continue
		}

		for _, name := range names {
			log.Debug("reading file %v", name)
			d, err := ioutil.ReadFile(name)
			if err != nil {
				log.Errorln(err)
				continue
			}

			fi, err := os.Stat(name)
			if err != nil {
				log.Errorln(err)
				continue
			}
			perm := fi.Mode() & os.ModePerm

			res = append(res, &ron.File{
				Name: name,
				Perm: perm,
				Data: d,
			})
		}
	}

	return res
}

// recvFiles retrieves a list of files from the ron server by requesting each
// one individually.
func recvFiles(files []*ron.File) {
	start := time.Now()
	var size int64

	for _, v := range files {
		log.Info("requesting file %v", v)

		dst := filepath.Join(*f_path, "files", v.Name)

		if _, err := os.Stat(dst); err == nil {
			// file exists (TODO: overwrite?)
			log.Info("skipping %v -- already exists")
			continue
		}

		m := &ron.Message{
			Type:     ron.MESSAGE_FILE,
			UUID:     client.UUID,
			Filename: v.Name,
		}

		if err := sendMessage(m); err != nil {
			log.Error("send failed: %v", err)
			return
		}

		resp := <-client.fileChan
		if resp.Filename != v.Name {
			log.Error("filename mismatch: %v != %v", resp.Filename, v.Name)
			continue
		}

		if resp.Error != "" {
			log.Error("%v", resp.Error)
			continue
		}

		dir := filepath.Dir(dst)

		if err := os.MkdirAll(dir, os.FileMode(0770)); err != nil {
			log.Errorln(err)
			continue
		}

		if err := ioutil.WriteFile(dst, resp.File, v.Perm); err != nil {
			log.Errorln(err)
			continue
		}

		size += int64(len(resp.File))
	}

	d := time.Since(start)
	rate := (float64(size) / 1024 / d.Seconds())

	log.Debug("received %v bytes in %v (%v KBps)", size, d, rate)

	return
}
