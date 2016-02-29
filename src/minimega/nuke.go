// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"goreadline"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strings"
	"time"
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
// 	kill all qemu instances
//	kill all taps
//  kill all containers
//	remove everything inside of info.BasePath (careful, that's dangerous)
//  exit()
func cliNuke(c *minicli.Command) *minicli.Response {
	// nuke any container related items
	containerNuke()

	// hold the reaper lock so nothing is deleted from under us
	// this is never released as we Exit() at the end of this function
	reapTapsLock.Lock()

	// walk the minimega root tree and do certain actions such as
	// kill qemu pids, remove taps, and remove the bridge
	err := filepath.Walk(*f_base, nukeWalker)
	if err != nil {
		log.Errorln(err)
	}

	// allow udev to sync
	time.Sleep(time.Second * 1)

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

	// remove any stale mega_taps from open vswitch
	tapNames = ovsGetTaps()
	nukeTaps(tapNames)

	// remove bridges that have preExist == false
	nukeBridges()

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		log.Errorln(err)
	}

	// clean up possibly leftover state
	nukeState()

	os.Exit(0)
	return nil
}

// Nuke a list of tap names
func nukeTaps(taps []string) {
	// Stack ovs commands for |\/|aximum power
	var args []string

	for _, t := range taps {
		// Delete the tap device
		nukeTap(t)

		// Add to the ovs cmd
		args = append(args, "del-port", t, "--")
	}

	if len(args) > 0 {
		ovsCmdWrapper(args)
	}
}

// Nuke all possible leftover state
// Similar to teardown(), but designed to be called from nuke
func nukeState() {
	goreadline.Rlcleanup()
	vncClear()
	clearAllCaptures()
	ksmDisable()
	vms.cleanDirs()
}

// return names of bridges as shown in f_base/bridges. Optionally include
// bridges that existed before minimega was launched
func nukeBridgeNames(preExist bool) []string {
	var ret []string
	b, err := os.Open(filepath.Join(*f_base, "bridges"))
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
	bNames := nukeBridgeNames(false)
	for _, b := range bNames {
		if err := ovsDelBridge(b); err != nil {
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
		if err != nil {
			log.Error("%v: %v", err, out)
		}
	}
	return nil
}

func nukeTap(tap string) {

	if err := delTap(tap); err != nil {
		log.Error("%v -- %v", tap, err)
	}
}
