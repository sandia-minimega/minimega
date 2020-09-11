// vncdrone replays VNC keyboard+mouse recordings (as captured by minimega)
// Pass it a directory containing recordings; these recording filenames must
// match the filename of the disk image used to boot a VM; e.g. if you have
// a VM booted from "ubuntu_linux.qcow2" and you want to run a replay on it,
// you should name the replay file "ubuntu_linux.kb".
//
// vncdrone automatically runs all your recording files across as many compatible
// VMs as it can find.
//
// Example usage:
//         vncdrone -recordings ~/recordings/ -nodes cluster_node1,cluster_node[5-11]
package main

import (
	"container/ring"
	"flag"
	"fmt"
	"github.com/sandia-minimega/minimega/internal/miniclient"
	"github.com/sandia-minimega/minimega/internal/minipager"
	log "github.com/sandia-minimega/minimega/pkg/minilog"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"
)

var (
	f_recordings = flag.String("recordings", "", "directory containing recordings")
	f_nodes      = flag.String("nodes", "", "node(s) running VMs")
	f_base       = flag.String("base", "/tmp/minimega", "minimega base directory")
)

func main() {
	flag.Parse()
	log.Init()

	c, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatal(err.Error())
	}
	c.Pager = minipager.DefaultPager

	// Get the list of files containing VNC recordings
	recordings, err := ioutil.ReadDir(*f_recordings)
	if err != nil {
		log.Fatal(err.Error())
	}

	r := ring.New(len(recordings))
	for _, rec := range recordings {
		r.Value = rec.Name()
		r = r.Next()
	}

	// This is a little complex.
	for {
		// Get one of the recordings
		filename := r.Value.(string)
		r = r.Next()
		// Strip off the .kb extension
		name := strings.TrimSuffix(filename, filepath.Ext(filename))
		// Now we need to find VMs whose disk image is name.qcow2
		diskname := name + ".qcow2"
		log.Debug(fmt.Sprintf("Attempting to play %s... Searching for a VM using the %s disk image", filename, diskname))

		// Get a list of all current VNC playbacks
		// this will come back as host,id
		cmd := ".csv true .annotate false .headers false .columns host,id vnc"
		vncresponsechan := c.Run(cmd)

		// Now make a map of all the VMs that are busy
		// "busy" maps hostnames to a slice of strings representing VMs currently playing something
		busy := make(map[string][]string)
		for v := range vncresponsechan {
			if v.Rendered == "" {
				continue
			}
			// v.Rendered may be several lines
			lines := strings.Split(v.Rendered, "\n")
			for _, line := range lines {
				split := strings.Split(line, ",")
				// Grab the list of busy VMs, add the new one, save it back
				b := busy[split[0]]
				vmid := split[1]
				b = append(b, vmid)
				busy[split[0]] = b
			}
		}

		// Get a list of all VMs
		// this will come back as host,id
		cmd = fmt.Sprintf("mesh send %s .header false .csv true .columns id .filter disk=%s vm info kvm", *f_nodes, diskname)
		vmresponsechan := c.Run(cmd)

	outside:
		for resp := range vmresponsechan {
			if resp.Rendered == "" {
				continue
			}

			// resp.Rendered may contain many lines
			lines := strings.Split(resp.Rendered, "\n")
		checkvm:
			for _, line := range lines {
				split := strings.Split(line, ",")
				if len(split) != 2 {
					continue
				}
				// This VM could run our recording, as long as it's not busy
				host := split[0]
				id := split[1]

				//log.Debug(fmt.Sprintf("checking %s:%s", host, id))

				// check if this VM is busy
				b := busy[host]
				for _, busyid := range b {
					if busyid == id {
						// this VM is already playing a recording
						continue checkvm
					}
				}

				// if we got here, the VM is not busy, so start playing the recording!
				recordingpath := filepath.Join(*f_recordings, filename)
				log.Debug("Playing %v on %v %v", recordingpath, host, id)
				cmd := fmt.Sprintf("vnc playback %s %s %s", host, id, recordingpath)
				c.RunAndPrint(cmd, false)
				break outside
			}
		}
		time.Sleep(1 * time.Second)
	}
}
