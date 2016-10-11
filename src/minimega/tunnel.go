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
	"strings"

	"golang.org/x/net/websocket"
)

func vncWsHandler(ws *websocket.Conn) {
	// URL should be of the form `/ws/<vm_name>`
	path := strings.Trim(ws.Config().Location.Path, "/")

	fields := strings.Split(path, "/")
	if len(fields) != 2 {
		return
	}
	vmName := fields[1]

	vms := GlobalVMs()
	vm, err := vms.findKvmVM(vmName)
	if err != nil {
		log.Errorln(err)
		return
	}

	// Undocumented "feature" of websocket -- need to set to PayloadType in
	// order for a direct io.Copy to work.
	ws.PayloadType = websocket.BinaryFrame

	// connect to the remote host
	rhost := fmt.Sprintf("%v:%v", vm.Host, vm.VNCPort)
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer remote.Close()

	go io.Copy(remote, ws)
	io.Copy(ws, remote)

	log.Info("ws client disconnected from %v", rhost)
}

func terminalWsHandler(w http.ResponseWriter, r *http.Request) {
	// we assume that if we got here, then the url must be sane and of
	// the format /ws/<host>/<port>
	path := r.URL.Path
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	fields := strings.Split(path, "/")
	if len(fields) != 5 {
		http.NotFound(w, r)
		return
	}
	fields = fields[2:]

	rhost := fmt.Sprintf("%v:%v", fields[0], fields[1])

	// connect to the remote host
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		http.StatusText(500)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		go func() {
			var data []byte
			for {
				websocket.Message.Receive(ws, &data)
				remote.Write(data)
			}
			remote.Close()
		}()
		rbuf := make([]byte, 1)
		for {
			n, err := remote.Read(rbuf)
			if err != nil {
				if !strings.Contains(err.Error(), "closed network connection") && err != io.EOF {
					log.Errorln(err)
				}
				break
			}
			websocket.Message.Send(ws, string(rbuf[:n]))
		}
		ws.Close()
	}).ServeHTTP(w, r)
}