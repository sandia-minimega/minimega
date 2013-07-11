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
	"bytes"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// clean up after an especially bad crash, hopefully we don't have to call
// this one much :)
// currently this will:
// 	kill all qemu instances
//	kill all taps
//	remove everything inside of info.BasePath (careful, that's dangerous)
func nuke(c cliCommand) cliResponse { // the cliResponse return is just so we can fit in the cli model
	if len(c.Args) != 0 {
		return cliResponse{
			Error: "nuke does not take any arguments",
		}
	}

	// walk the minimega root tree and do certain actions such as
	// kill qemu pids, remove taps, and remove the bridge
	err := filepath.Walk(*f_base, nukeWalker)
	if err != nil {
		log.Errorln(err)
	}

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		log.Errorln(err)
	}

	// remove all mega_taps, but leave the mega_bridge
	dirs, err := ioutil.ReadDir("/sys/class/net")
	if err != nil {
		log.Errorln(err)
	} else {
		for _, n := range dirs {
			if strings.Contains(n.Name(), "mega_tap") {
				nukeTap(n.Name())
			}
		}
	}

	teardown()
	return cliResponse{}
}

func nukeWalker(path string, info os.FileInfo, err error) error {
	if err != nil {
		return nil
	}

	log.Debug("walking file: %v", path)

	switch info.Name() {
	case "qemu.pid", "dnsmasq.pid":
		d, err := ioutil.ReadFile(path)
		t := strings.TrimSpace(string(d))
		log.Debug("found pid: %v", t)
		if err != nil {
			return err
		}
		var sOut bytes.Buffer
		var sErr bytes.Buffer

		p := process("kill")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				t,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Infoln("killing process:", t)
		err = cmd.Run()
		if err != nil {
			log.Error("%v: %v", err, sErr.String())
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
			nukeTap(v)
		}
	}
	return nil
}

func nukeTap(tap string) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer

	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tap,
			"down",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("bringing tap down with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		log.Error("%v: %v", err, sErr.String())
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"tuntap",
			"del",
			"mode",
			"tap",
			tap,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("destroying tap with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		log.Error("%v: %v", err, sErr.String())
	}

	p = process("ovs")
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"del-port",
			"mega_bridge",
			tap,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("removing tap from mega_bridge with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		log.Error("%v: %v", err, sErr.String())
	}
}
