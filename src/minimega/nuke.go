// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// routine to clean up the minimega state after a bad crash
package main

import (
	"errors"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"io/ioutil"
	"bytes"
	"strings"
)

// clean up after an especially bad crash, hopefully we don't have to call
// this one much :)
// currently this will:
// 	kill all qemu instances
//	kill all taps
//	remove everything inside of info.Base_path (careful, that's dangerous)
// TODO: clean up from pid and tap files
func nuke(c cli_command) cli_response { // the cli_response return is just so we can fit in the cli model
	if len(c.Args) != 0 {
		return cli_response{
			Error: errors.New("nuke does not take any arguments"),
		}
	}

	// walk the minimega root tree and do certain actions such as
	// kill qemu pids, remove taps, and remove the bridge
	err := filepath.Walk(*f_base, nuke_walker)
	if err != nil {
		return cli_response{
			Error: err,
		}
	}

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		return cli_response{
			Error: err,
		}
	}
	teardown()
	return cli_response{}
}

func nuke_walker(path string, info os.FileInfo, err error) error {
	if err != nil {
		return nil
	}

	log.Debug("walking file: %v", path)

	switch info.Name() {
	case "qemu.pid":
		d, err := ioutil.ReadFile(path)
		t := strings.TrimSpace(string(d))
		log.Debug("found qemu pid: %v", t)
		if err != nil {
			return err
		}
		var s_out bytes.Buffer
		var s_err bytes.Buffer

		p := process("kill")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				t,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &s_out,
			Stderr: &s_err,
		}
		log.Infoln("killing qemu process:", t)
		err = cmd.Run()
		if err != nil {
			log.Error("%v: %v", err, s_err.String())
		}
	case "taps":
		d, err := ioutil.ReadFile(path)
		t := strings.TrimSpace(string(d))
		if err != nil {
			return err
		}
		f := strings.Fields(t)
		log.Debugln("got taps:", f)
		for _, v := range f {
			var s_out bytes.Buffer
			var s_err bytes.Buffer

			p := process("ip")
			cmd := &exec.Cmd{
				Path: p,
				Args: []string{
					p,
					"link",
					"set",
					v,
					"down",
				},
				Env:    nil,
				Dir:    "",
				Stdout: &s_out,
				Stderr: &s_err,
			}
			log.Info("bringing tap down with cmd: %v", cmd)
			err := cmd.Run()
			if err != nil {
				log.Error("%v: %v", err, s_err.String())
			}

			p = process("tunctl")
			cmd = &exec.Cmd{
				Path: p,
				Args: []string{
					p,
					"-d",
					v,
				},
				Env:    nil,
				Dir:    "",
				Stdout: &s_out,
				Stderr: &s_err,
			}
			log.Info("destroying tap with cmd: %v", cmd)
			err = cmd.Run()
			if err != nil {
				log.Error("%v: %v", err, s_err.String())
			}
		}
	}
	return nil
}
