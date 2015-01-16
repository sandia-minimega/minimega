// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var nukeCLIHandlers = []minicli.Handler{
	{ // nuke
		HelpShort: "attempt to clean up after a crash",
		HelpLong: `
After a crash, the VM state on the machine can be difficult to recover from.
Nuke attempts to kill all instances of QEMU, remove all taps and bridges, and
removes the temporary minimega state on the harddisk.`,
		Patterns: []string{
			"nuke",
		},
		Call: wrapSimpleCLI(cliNuke),
	},
}

func init() {
	registerHandlers("nuke", nukeCLIHandlers)
}

// clean up after an especially bad crash, hopefully we don't have to call
// this one much :)
// currently this will:
// 	kill all qemu instances
//	kill all taps
//	remove everything inside of info.BasePath (careful, that's dangerous)
func cliNuke(c *minicli.Command) *minicli.Response {
	// walk the minimega root tree and do certain actions such as
	// kill qemu pids, remove taps, and remove the bridge
	err := filepath.Walk(*f_base, nukeWalker)
	if err != nil {
		log.Errorln(err)
	}

	// remove all mega_taps
	bNames := nukeBridgeNames(true)
	dirs, err := ioutil.ReadDir("/sys/class/net")
	if err != nil {
		log.Errorln(err)
	} else {
		for _, n := range dirs {
			if strings.Contains(n.Name(), "mega_tap") {
				for _, b := range bNames {
					nukeTap(b, n.Name())
				}
			}
		}
	}

	// remove bridges that have preExist == false
	nukeBridges()

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		log.Errorln(err)
	}

	teardown()
	return nil
}

// return names of bridges as shown in f_base/bridges. Optionally include
// bridges that existed before minimega was launched
func nukeBridgeNames(preExist bool) []string {
	var ret []string
	b, err := os.Open(*f_base + "bridges")
	if err != nil {
		log.Errorln(err)
		return nil
	}
	scanner := bufio.NewScanner(b)
	// skip the first line
	scanner.Scan()
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		log.Debugln(f)
		if len(f) <= 3 {
			continue
		}
		if (f[2] == "true" && preExist) || f[2] == "false" {
			ret = append(ret, f[0])
		}
	}
	log.Debug("nukeBridgeNames got: %v", ret)
	return ret
}

func nukeBridges() {
	bNames := nukeBridgeNames(false)
	for _, b := range bNames {
		var sOut bytes.Buffer
		var sErr bytes.Buffer

		p := process("ovs")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				"del-br",
				b,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Infoln("removing bridge:", b)
		//err := cmd.Run()
		err := cmdTimeout(cmd, OVS_TIMEOUT)
		if err != nil {
			log.Error("%v: %v", err, sErr.String())
		}
	}
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
	}
	return nil
}

func nukeTap(b, tap string) {
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
			b,
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
