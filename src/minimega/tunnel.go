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

func tunnelHandler(ws *websocket.Conn) {
	// URL should be of the form `/tunnel/<name>`
	path := strings.Trim(ws.Config().Location.Path, "/")

	fields := strings.Split(path, "/")
	if len(fields) != 2 {
		return
	}
	name := fields[1]

	vms := GlobalVMs()
	vm := vms.findVM(name, true)
	if vm == nil {
		log.Errorln(vmNotFound(name))
		return
	}

	var port int

	switch vm.GetType() {
	case KVM:
		// Undocumented "feature" of websocket -- need to set to PayloadType in
		// order for a direct io.Copy to work.
		ws.PayloadType = websocket.BinaryFrame

		port = vm.(*KvmVM).VNCPort
	case CONTAINER:
		// See above. The javascript terminal needs it to be a TextFrame.
		ws.PayloadType = websocket.TextFrame

		port = vm.(*ContainerVM).ConsolePort
	default:
		log.Error("unknown VM type: %v", vm.GetType())
		return
	}

	// connect to the remote host
	rhost := fmt.Sprintf("%v:%v", vm.GetHost(), port)
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer remote.Close()

	log.Info("ws client connected to %v", rhost)

	go io.Copy(ws, remote)
	io.Copy(remote, ws)

	log.Info("ws client disconnected from %v", rhost)
}
