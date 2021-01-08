// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	"io"
	log "minilog"
	"net"
	"os"

	"golang.org/x/net/websocket"
)

// connectWsHandler returns a function to service a websocket for the given VM
func connectWsHandler(vmType, host string, port int) func(*websocket.Conn) {
	return func(ws *websocket.Conn) {
		switch vmType {
		case "kvm":
			// Undocumented "feature" of websocket -- need to set to
			// PayloadType in order for a direct io.Copy to work.
			ws.PayloadType = websocket.BinaryFrame
		case "container":
			// See above. The javascript terminal needs it to be a TextFrame.
			ws.PayloadType = websocket.TextFrame
		}

		// connect to the remote host
		rhost := fmt.Sprintf("%v:%v", host, port)
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
}

// consoleWsHandler returns a function to service a websocket for the given pty
func consoleWsHandler(tty *os.File, pid int) func(*websocket.Conn) {
	return func(ws *websocket.Conn) {
		defer func() {
			tty.Close()
		}()

		proc, err := os.FindProcess(pid)
		if err != nil {
			log.Error("unable to find process: %v", pid)
			return
		}

		go io.Copy(ws, tty)
		io.Copy(tty, ws)

		proc.Kill()
		proc.Wait()

		ptyMu.Lock()
		defer ptyMu.Unlock()

		delete(ptys, pid)
	}
}
