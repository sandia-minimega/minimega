// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"ron"
	"strings"
	"time"
)

func client() {
	log.Debugln("client")
	for {
		command := <-c.Commands
		log.Debug("processing command %v", command.ID)
		clientCommandExec(command)
	}
}

func prepareRecvFiles(files []string) map[string][]byte {
	log.Debug("prepareRecvFiles %v", files)
	r := make(map[string][]byte)
	// expand everything
	var nfiles []string
	for _, f := range files {
		tmp, err := filepath.Glob(f)
		if err != nil {
			log.Errorln(err)
			continue
		}
		nfiles = append(nfiles, tmp...)
	}
	for _, f := range nfiles {
		log.Debug("reading file %v", f)
		d, err := ioutil.ReadFile(f)
		if err != nil {
			log.Errorln(err)
			continue
		}
		r[f] = d
	}
	return r
}

func clientCommandExec(command *ron.Command) {
	log.Debug("clientCommandExec %v", command.ID)
	resp := &ron.Response{
		ID: command.ID,
	}

	// get any files needed for the command
	if len(command.FilesSend) != 0 {
		commandGetFiles(command.FilesSend)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if len(command.Command) != 0 {
		path, err := exec.LookPath(command.Command[0])
		if err != nil {
			log.Errorln(err)
			resp.Stderr = err.Error()
		} else {
			cmd := &exec.Cmd{
				Path:   path,
				Args:   command.Command,
				Env:    nil,
				Dir:    "",
				Stdout: &stdout,
				Stderr: &stderr,
			}
			log.Debug("executing %v", strings.Join(command.Command, " "))

			if command.Background {
				log.Debug("starting command %v in background", command.Command)
				err = cmd.Start()
				if err != nil {
					log.Errorln(err)
					resp.Stderr = stderr.String()
				} else {
					go func() {
						cmd.Wait()
						log.Info("command %v exited", strings.Join(command.Command, " "))
						log.Info(stdout.String())
						log.Info(stderr.String())
					}()
				}
			} else {
				err := cmd.Run()
				if err != nil {
					log.Errorln(err)
				}
				resp.Stdout = stdout.String()
				resp.Stderr = stderr.String()
			}
		}
	}

	if len(command.FilesRecv) != 0 {
		resp.Files = prepareRecvFiles(command.FilesRecv)
	}

	c.Respond(resp)
}

func commandGetFiles(files []string) {
	start := time.Now()
	var byteCount int64
	for _, v := range files {
		log.Debug("get file %v", v)
		path := filepath.Join(*f_path, "files", v)

		if _, err := os.Stat(path); err == nil {
			// file exists
			continue
		}

		file, err := c.GetFile(v)
		if err != nil {
			log.Errorln(err)
			continue
		}

		dir := filepath.Dir(path)
		err = os.MkdirAll(dir, os.FileMode(0770))
		if err != nil {
			log.Errorln(err)
			continue
		}
		f, err := os.Create(path)
		if err != nil {
			log.Errorln(err)
			continue
		}
		f.Write(file)
		f.Close()
		byteCount += int64(len(file))
	}
	end := time.Now()
	elapsed := end.Sub(start)
	kbytesPerSecond := (float64(byteCount) / 1024.0) / elapsed.Seconds()
	log.Debug("received %v bytes in %v (%v kbytes/second)", byteCount, elapsed, kbytesPerSecond)
}
