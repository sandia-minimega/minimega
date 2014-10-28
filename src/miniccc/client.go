package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	log "minilog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"ron"
	"strings"
)

func client() {
	log.Debugln("client")
	for {
		c := r.GetNewCommands()
		for _, v := range c {
			log.Debug("processing command %v", v.ID)

			clientCommandExec(v)
		}
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

func clientCommandExec(c *ron.Command) {
	log.Debug("clientCommandExec %v", c.ID)
	resp := &ron.Response{
		ID:   c.ID,
		UUID: r.UUID,
	}

	// get any files needed for the command
	if len(c.FilesSend) != 0 {
		commandGetFiles(c.FilesSend)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if len(c.Command) != 0 {
		path, err := exec.LookPath(c.Command[0])
		if err != nil {
			log.Errorln(err)
			resp.Stderr = err.Error()
		} else {
			cmd := &exec.Cmd{
				Path:   path,
				Args:   c.Command,
				Env:    nil,
				Dir:    "",
				Stdout: &stdout,
				Stderr: &stderr,
			}
			log.Debug("executing %v", strings.Join(c.Command, " "))

			if c.Background {
				log.Debug("starting command %v in background", c.Command)
				err = cmd.Start()
				if err != nil {
					log.Errorln(err)
					resp.Stderr = stderr.String()
				} else {
					go func() {
						cmd.Wait()
						log.Info("command %v exited", strings.Join(c.Command, " "))
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

	if len(c.FilesRecv) != 0 {
		resp.Files = prepareRecvFiles(c.FilesRecv)
	}

	r.PostResponse(resp)
}

func commandGetFiles(files []string) {
	for _, v := range files {
		log.Debug("get file %v", v)
		path := filepath.Join(*f_path, "files", v)

		if _, err := os.Stat(path); err == nil {
			// file exists
			continue
		}

		url := fmt.Sprintf("http://%v:%v/files/%v", *f_parent, *f_port, v)
		log.Debug("file get url %v", url)
		resp, err := http.Get(url)
		if err != nil {
			log.Errorln(err)
			continue
		}

		dir := filepath.Dir(path)
		err = os.MkdirAll(dir, os.FileMode(0770))
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}
		f, err := os.Create(path)
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}
		io.Copy(f, resp.Body)
		f.Close()
		resp.Body.Close()
	}
}
