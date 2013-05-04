package main

import (
	"fmt"
	log "minilog"
	"novnctun"
	"os"
	"strconv"
	"strings"
)

var (
	vncServer *novnctun.Tun
	vncNovnc  string = "misc/novnc"
)

const vncPort = ":8080"

// register a Hosts() function on type vmList, allowing us to point novnctun at it
func (vms *vmList) Hosts() map[string][]string {
	ret := make(map[string][]string)

	// the vnc port is just 5900 + the vm id

	// first grab our own list of hosts
	host, err := os.Hostname()
	if err != nil {
		log.Errorln(err)
		return nil
	}
	for _, vm := range vms.vms {
		if vm.State != VM_QUIT && vm.State != VM_ERROR {
			port := fmt.Sprintf("%v", 5900+vm.Id)
			ret[host] = append(ret[host], port)
		}
	}

	// get a list of the other hosts on the network
	cmd := cliCommand{
		Args: []string{"hostname"},
	}
	resp := meshageBroadcast(cmd)
	if resp.Error != "" {
		log.Errorln(resp.Error)
		return nil
	}

	hosts := strings.Fields(resp.Response)

	for _, h := range hosts {
		// get a list of vms from that host
		cmd := cliCommand{
			Args: []string{h, "vm_status"},
		}
		resp := meshageSet(cmd)
		if resp.Error != "" {
			log.Errorln(resp.Error)
			continue // don't error out if just one host fails us
		}

		lines := strings.Split(resp.Response, "\n")
		for _, l := range lines {
			// the vm id is the second field
			// TODO: TEST filter out any quit or error state vms from remote vnc lsit
			f := strings.Fields(l)
			if len(f) == 7 {
				if !strings.Contains(f[3], "QUIT") && !strings.Contains(f[3], "ERROR") {
					val, err := strconv.Atoi(f[1])
					if err != nil {
						log.Errorln(err)
						continue
					}
					port := fmt.Sprintf("%v", 5900+val)
					ret[h] = append(ret[h], port)
				}
			}
		}
	}
	return ret
}

func cliVnc(c cliCommand) cliResponse {
	// we have 2 possible cases:
	// vnc novnc - set the vnc path
	// vnc serve :8080 serve on a specific port and don't launch anything
	if len(c.Args) == 0 {
		return cliResponse{
			Error: "vnc takes at least one argument",
		}
	}
	switch c.Args[0] {
	case "novnc":
		if len(c.Args) == 1 {
			return cliResponse{
				Response: vncNovnc,
			}
		} else if len(c.Args) > 2 {
			return cliResponse{
				Error: "vnc novnc takes 2 arguments",
			}
		}
		vncNovnc = c.Args[1]
	case "serve":
		if len(c.Args) == 1 { // just start the server
			if vncServer == nil {
				vncServe(vncPort)
			} else {
				e := fmt.Sprintf("vnc already running on: %v", vncServer.Addr)
				return cliResponse{
					Error: e,
				}
			}
		} else if len(c.Args) == 2 {
			if vncServer == nil {
				vncServe(c.Args[1])
			} else {
				e := fmt.Sprintf("vnc already running on: %v", vncServer.Addr)
				return cliResponse{
					Error: e,
				}
			}
		} else {
			return cliResponse{
				Error: "invalid command",
			}
		}
	default: // must be an id right?
		return cliResponse{
			Error: "invalid command",
		}
	}
	return cliResponse{}
}

func vncServe(addr string) {
	vncServer = &novnctun.Tun{
		Addr:   addr,
		Hosts:  &vms,
		Files:  vncNovnc,
		Unsafe: false,
	}
	go func() {
		log.Errorln(vncServer.Start())
	}()
}
