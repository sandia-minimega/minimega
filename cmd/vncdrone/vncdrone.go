// vncdrone replays VNC keyboard+mouse recordings (as captured by minimega)
// Pass it a directory containing recordings; these recroding filenames must
// follow a strict naming scheme:
//
//	<vm prefix>_<pre,run,post>_<unique name>.kb
//
// The fields are as follows:
//
//	<vm prefix>: A string that must match the prefix of a VM at runtime.  Only
//	files that match to named VMs with the same prefix will be played on that VM.
//	For example, "foo_pre_1.kb" will play on VMs named "foobar" and "footothemoo",
//	but not "nofoo".
//
//	<pre,run,post>: There are 3 groups of files, all of which are randomized within
//	each group. This is to facilitate actions like "login, do work, logout". The
//	typical usage for this would be to have a single "pre" file to login to the VM,
//	one or more "run" files that would be randomly played (potentially for a long
//	time), and then a single "post" file to logout. vncdrone maintains state for
//	each VM and only transitions from pre->run->post->pre... which state
//	transitions happen. State transitions are randomized, so having multiple files
//	in each state group must support being played at random within the group (ie
//	return the desktop to a known "steady state").
//	Special case: If only one "pre" or "post" file exists in their
//	respective groups, then a state change happens after playing that file
//	no matter what.
//
//	<unique name>: The rest of the recording's filename.
//
// Example usage:
//
//	vncdrone -recordings ~/recordings/ -namespace foo
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	f_recordings = flag.String("recordings", "", "directory containing recordings - absolute path required")
	f_base       = flag.String("base", "/tmp/minimega", "minimega base directory")
)

const (
	STATE_PROBABILITY = 10
)

const (
	PRE = iota
	RUN
	POST
)

type Recordings struct {
	Pre  []string
	Run  []string
	Post []string
}

type VM struct {
	State      int
	Runnable   bool
	NumInState int
}

func main() {
	flag.Parse()
	log.Init()

	log.Debugln("vncdrone start")

	c, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatal(err.Error())
	}
	// don't set a pager for the client
	c.Pager = nil

	recordings := make(map[string]*Recordings)
	vms := make(map[string]*VM)

	// get all the recordings
	r, err := ioutil.ReadDir(*f_recordings)
	if err != nil {
		log.Fatal(err.Error())
	}
	for _, v := range r {
		f := strings.Split(v.Name(), "_")
		if len(f) != 3 {
			log.Error("%v does not follow naming convention, skipping", v)
			continue
		}

		log.Debug("reading file: %v", v)

		if _, ok := recordings[f[0]]; !ok {
			recordings[f[0]] = &Recordings{}
		}

		record := recordings[f[0]]
		switch f[1] {
		case "pre":
			record.Pre = append(record.Pre, v.Name())
		case "run":
			record.Run = append(record.Run, v.Name())
		case "post":
			record.Post = append(record.Post, v.Name())
		default:
			log.Error("%v does not follow naming convention, skipping", v)
			continue
		}
	}

	for {
		time.Sleep(10 * time.Second)

		// get all the VMs by name
		cmd := ".annotate false .headers false .columns name vm info"
		log.Debug("issuing command: %v", cmd)

		namesChan := c.Run(cmd)

		bachelorVMs := []string{}

		for v := range namesChan {
			if v.Rendered == "" {
				continue
			}

			// v.Rendered may be several lines
			lines := strings.Split(v.Rendered, "\n")
			for _, name := range lines {
				// only keep VM names for which we have recordings
				for prefix, _ := range recordings {
					if strings.HasPrefix(name, prefix) {
						bachelorVMs = append(bachelorVMs, name)
					}
				}
			}
		}
		log.Debug("got valid VMs: %v", bachelorVMs)

		// update global VM state for the bachelors if we need and discard and VMs that are gone
		nvms := make(map[string]*VM)
		for _, name := range bachelorVMs {
			if v, ok := vms[name]; !ok {
				nvms[name] = &VM{State: PRE}
			} else {
				nvms[name] = v
			}
			nvms[name].Runnable = true
		}
		vms = nvms

		// figure out which VMs can take a new playback
		cmd = ".annotate false .headers false .columns name vnc"
		log.Debug("issuing command: %v", cmd)

		vncChan := c.Run(cmd)

		for v := range vncChan {
			if v.Rendered == "" {
				continue
			}

			// v.Rendered may be several lines
			lines := strings.Split(v.Rendered, "\n")
			for _, name := range lines {
				if _, ok := vms[name]; ok {
					vms[name].Runnable = false
				}
			}
		}

		for name, vm := range vms {
			if !vm.Runnable {
				continue
			}

			// get the recording for this vm
			var r *Recordings
			for recording, record := range recordings {
				if strings.HasPrefix(name, recording) {
					r = record
					break
				}
			}
			if r == nil {
				log.Fatal("couldn't find recording for %v!", name)
			}

			var curr []string
			switch vm.State {
			case PRE:
				curr = r.Pre
			case RUN:
				curr = r.Run
			case POST:
				curr = r.Post
			}

			rand.Seed(time.Now().UnixNano())

			// switch state?
			if vm.NumInState > 0 || len(curr) == 0 {
				if rand.Intn(STATE_PROBABILITY) == 0 || (vm.State == PRE && len(curr) == 1) || (vm.State == POST && len(curr) == 1) {
					// switch state!
					switch vm.State {
					case PRE:
						vm.State = RUN
						curr = r.Run
					case RUN:
						vm.State = POST
						curr = r.Post
					case POST:
						vm.State = PRE
						curr = r.Pre
					}
					vm.NumInState = 0
				}
			}

			// pick a file from the current state
			if len(curr) > 0 {
				file := curr[rand.Intn(len(curr))]
				vm.NumInState++

				fullpath := filepath.Join(filepath.Dir(*f_recordings), file)
				cmd := fmt.Sprintf("vnc play %v %v", name, fullpath)
				log.Debug("issuing command: %v", cmd)

				c.RunAndPrint(cmd, false)
			}
		}
	}
}
