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

func terminalWsHandler(ws *websocket.Conn) {
	// URL should be of the form `/termws/<vm_name>`
	path := strings.Trim(ws.Config().Location.Path, "/")

	fields := strings.Split(path, "/")
	if len(fields) != 2 {
		return
	}
	vmName := fields[1]

	vms := GlobalVMs()
	vm, err := vms.findContainerVM(vmName)
	if err != nil {
		log.Errorln(err)
		return
	}

	// Undocumented "feature" of websocket -- need to set to PayloadType in
	// order for a direct io.Copy to work. The javascript terminal needs it
	// to be a TextFrame.
	ws.PayloadType = websocket.TextFrame

	// connect to the remote host
	rhost := fmt.Sprintf("%v:%v", vm.Host, vm.ConsolePort)
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