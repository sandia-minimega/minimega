// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	ufs "github.com/Harvey-OS/ninep/filesystem"
	"github.com/Harvey-OS/ninep/protocol"
)

var rootFS struct {
	// embed
	*protocol.Server

	running bool

	// active connection, set when running
	remote, local net.Conn
}

// ufsMessage handles a message from the server and relays it to UFS
func ufsMessage(m *ron.Message) {
	switch m.UfsMode {
	case ron.UFS_OPEN:
		if rootFS.running {
			log.Error("ufs is already running")
			return
		}

		if rootFS.Server == nil {
			log.Info("init rootFS")
			root := "/"
			if runtime.GOOS == "windows" {
				// TODO: what if there is more that one volume?
				root = filepath.VolumeName(os.Getenv("SYSTEMROOT")) + "\\"
			}

			fs, err := ufs.NewServer(ufs.Root(root), ufs.Trace(log.Debug))
			if err != nil {
				log.Error("unable to create file server: %v", err)
				return
			}

			ps, err := protocol.NewServer(fs, protocol.Trace(log.Debug))
			if err != nil {
				log.Error("unable to create ninep server: %v", err)
				return
			}
			rootFS.Server = ps

			log.Info("init'd rootFS")
		}

		rootFS.running = true

		rootFS.remote, rootFS.local = net.Pipe()

		go ron.Trunk(rootFS.remote, client.UUID, ufsSendMessage)

		log.Info("accepting tunneled connection")

		if err := rootFS.Accept(rootFS.local); err != nil {
			log.Error("ufs error: %v", err)
			rootFS.running = false
		}
	case ron.UFS_CLOSE:
		if !rootFS.running {
			log.Error("ufs not running")
			return
		}

		rootFS.running = false
		rootFS.remote.Close()
	case ron.UFS_DATA:
		if !rootFS.running {
			log.Error("ufs not running")
			return
		}

		// relay the Tunnel data from ron
		rootFS.remote.Write(m.Tunnel)
	}
}

// ufsSendMessage tweaks the message generated by ron.Trunk before calling
// sendMessage.
func ufsSendMessage(m *ron.Message) error {
	m.Type = ron.MESSAGE_UFS
	m.UfsMode = ron.UFS_DATA

	return sendMessage(m)
}
