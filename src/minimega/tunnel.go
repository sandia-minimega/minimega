// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// novnctun supports creating a websocket based tunnel to vnc ports on other
// hosts and serving a novnc client to the machine requesting the tunnel. This
// is used to automate connecting to virtual machines on a cluster when the
// user does not have direct access to cluster nodes. novnctun runs on the
// routable head node of the cluster, the user connects to it, and tunnels are
// created to connect to virtual machines.

package main

import (
	"fmt"
	"io"
	log "minilog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"websocket"
)

const VNC_WS_BUF = 32768

func vncWsHandler(w http.ResponseWriter, r *http.Request) {
	// we assume that if we got here, then the url must be sane and of
	// the format /ws/<vm_name>
	path := r.URL.Path
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	fields := strings.Split(path, "/")
	if len(fields) != 4 {
		http.NotFound(w, r)
		return
	}
	vmName := fields[2]

	vms := GlobalVMs()
	vm, err := vms.FindKvmVM(vmName)
	if err != nil {
		log.Errorln(err)
		http.StatusText(404)
		return
	}

	// connect to the remote host
	rhost := fmt.Sprintf("%s:%s", vm.Host, strconv.Itoa(vm.VNCPort))
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		http.StatusText(500)
		return
	}
	defer remote.Close()

	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		go func() {
			// Copy from remote -> ws
			go io.Copy(ws, remote)
			io.Copy(remote, ws)
		}()

		rbuf := make([]byte, VNC_WS_BUF)
		for {
			n, err := remote.Read(rbuf)
			if err != nil {
				if !strings.Contains(err.Error(), "closed network connection") && err != io.EOF {
					log.Errorln(err)
				}
				break
			}

			err = websocket.Message.Send(ws, rbuf[:n])
			if err != nil {
				log.Errorln(err)
				break
			}
		}
	}).ServeHTTP(w, r)
}
