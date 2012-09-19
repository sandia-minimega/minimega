package main

import (
	"errors"
	"fmt"
	log "minilog"
	"novnctun"
	"os"
	"os/exec"
	"strconv"
)

var (
	vnc_server *novnctun.Tun
	vnc_novnc  string = "misc/novnc"
)

const vnc_port = ":8080"

// register a Hosts() function on type vm_list, allowing us to point novnctun at it
func (vms *vm_list) Hosts() map[string][]string {
	ret := make(map[string][]string)

	//TODO: once we have this up for multiple hosts, generalize this for many hosts
	// the vnc port is just 5900 + the vm id
	host, err := os.Hostname()
	if err != nil {
		log.Errorln(err)
		return nil
	}
	for _, vm := range vms.vms {
		port := fmt.Sprintf("%v", 5900+vm.Id)
		ret[host] = append(ret[host], port)
	}
	return ret
}

func cli_vnc(c cli_command) cli_response {
	// we have 4 possible cases:
	// vnc - just launch with everything default and fire up the browser
	// vnc ID - launch with default and fire up the browser to that specific id
	// vnc novnc - set the vnc path
	// vnc serve :8080 serve on a specific port and don't launch anything
	if len(c.Args) == 0 {
		if vnc_server == nil {
			vnc_serve(vnc_port)
		}
		err := vnc_launch("")
		if err != nil {
			return cli_response{
				Error: err,
			}
		}
		return cli_response{}
	}
	switch c.Args[0] {
	case "novnc":
		if len(c.Args) == 1 {
			return cli_response{
				Response: vnc_novnc,
			}
		} else if len(c.Args) > 2 {
			return cli_response{
				Error: errors.New("vnc novnc takes 2 arguments"),
			}
		}
		vnc_novnc = c.Args[1]
	case "serve":
		if len(c.Args) == 1 { // just start the server
			if vnc_server == nil {
				vnc_serve(vnc_port)
			} else {
				e := fmt.Sprintf("vnc already running on: %v", vnc_server.Addr)
				return cli_response{
					Error: errors.New(e),
				}
			}
		} else if len(c.Args) == 2 {
			if vnc_server == nil {
				vnc_serve(c.Args[1])
			} else {
				e := fmt.Sprintf("vnc already running on: %v", vnc_server.Addr)
				return cli_response{
					Error: errors.New(e),
				}
			}
		} else {
			return cli_response{
				Error: errors.New("invalid command"),
			}
		}
	default: // must be an id right?
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err,
			}
		}
		if id < len(vms.vms) {
			if vnc_server == nil {
				vnc_serve(vnc_port)
			}
			host, err := os.Hostname()
			if err != nil {
				return cli_response{
					Error: err,
				}
			}
			s := fmt.Sprintf("/%v/%v", host, 5900+id)
			err = vnc_launch(s)
			if err != nil {
				return cli_response{
					Error: err,
				}
			}
		} else {
			return cli_response{
				Error: errors.New("invalid VM id"),
			}
		}
	}
	return cli_response{}
}

func vnc_serve(addr string) {
	vnc_server = &novnctun.Tun{
		Addr:   addr,
		Hosts:  &vms,
		Files:  vnc_novnc,
		Unsafe: false,
	}
	go func() {
		log.Errorln(vnc_server.Start())
	}()
}

func vnc_launch(url string) error {
	path := process("browser")
	cmd := &exec.Cmd{
		Path:   path,
		Args:   []string{path, url},
		Env:    nil,
		Dir:    "",
		Stdout: nil,
		Stderr: nil,
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	return nil
}
