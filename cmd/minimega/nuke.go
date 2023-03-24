// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var nukeCLIHandlers = []minicli.Handler{
	{ // nuke
		HelpShort: "attempt to clean up after a crash",
		HelpLong: `
After a crash, the VM state on the machine can be difficult to recover from.
nuke attempts to kill all instances of QEMU, remove all taps and bridges, and
removes the temporary minimega state on the harddisk.

Should be run with caution.`,
		Patterns: []string{
			"nuke",
		},
		Call: wrapSimpleCLI(cliNuke),
	},
}

// clean up after an especially bad crash
// currently this will:
//
//		kill all qemu instances
//		kill all taps
//	 	kill all containers
//		remove everything inside of info.BasePath (careful, that's dangerous)
//	  exit
func cliNuke(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// nuke any state we have
	DestroyNamespace(Wildcard)

	// nuke any container related items
	containerNuke()

	// walk the minimega root tree and do certain actions such as
	// kill qemu pids, remove taps, and remove the bridge
	err := filepath.Walk(*f_base, nukeWalker)
	if err != nil {
		log.Errorln(err)
	}

	// allow udev to sync
	time.Sleep(time.Second * 1)

	// remove bridges that have preExist == false
	nukeBridges()

	// remove all live mega_tap names
	var tapNames []string
	dirs, err := ioutil.ReadDir("/sys/class/net")
	if err != nil {
		log.Errorln(err)
	} else {
		for _, n := range dirs {
			if strings.Contains(n.Name(), "mega_tap") {
				tapNames = append(tapNames, n.Name())
			}
		}
	}
	nukeTaps(tapNames)

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		log.Errorln(err)
	}

	Shutdown("nuked")
	return unreachable()
}

// nukeTaps removes a list of tap devices
func nukeTaps(taps []string) {
	for _, t := range taps {
		if err := bridge.DestroyTap(t); err != nil {
			log.Error("%v -- %v", t, err)
		}
	}
}

// return names of bridges as shown in f_base/bridges. Optionally include
// bridges that existed before minimega was launched
func nukeBridgeNames(preExist bool) []string {
	var ret []string

	b, err := os.Open(filepath.Join(*f_base, "bridges"))
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		log.Errorln(err)
		return nil
	}

	scanner := bufio.NewScanner(b)
	// skip the first line
	scanner.Scan()
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		log.Debugln(f)
		if len(f) <= 2 {
			continue
		}
		if (f[1] == "true" && preExist) || f[1] == "false" {
			ret = append(ret, f[0])
		}
	}
	log.Debug("nukeBridgeNames got: %v", ret)
	return ret
}

func nukeBridges() {
	for _, b := range nukeBridgeNames(false) {
		if err := bridge.DestroyBridge(b); err != nil {
			log.Error("%v -- %v", b, err)
		}
	}
}

// Walks the f_base directory and kills procs read from any qemu or
// dnsmasq pid files
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

		args := []string{
			"kill",
			t,
		}
		log.Infoln("killing process:", t)

		out, err := processWrapper(args...)
		if err != nil && !strings.Contains(err.Error(), "No such process") {
			log.Error("%v: %v", err, out)
		}
	}
	return nil
}
