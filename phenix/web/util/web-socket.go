package util

import (
	"io"
	"net"

	log "github.com/activeshadow/libminimega/minilog"
	"golang.org/x/net/websocket"
)

// Taken (almost) as-is from minimega/miniweb.

func ConnectWSHandler(endpoint string) func(*websocket.Conn) {
	return func(ws *websocket.Conn) {
		// Undocumented "feature" of websocket -- need to set to
		// PayloadType in order for a direct io.Copy to work.
		ws.PayloadType = websocket.BinaryFrame

		// connect to the remote host
		remote, err := net.Dial("tcp", endpoint)
		if err != nil {
			log.Errorln(err)
			return
		}

		defer remote.Close()

		log.Info("ws client connected to %v", endpoint)

		go io.Copy(ws, remote)
		io.Copy(remote, ws)

		log.Info("ws client disconnected from %v", endpoint)
	}
}
