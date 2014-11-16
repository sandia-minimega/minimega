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
	"encoding/base64"
	"fmt"
	"io"
	log "minilog"
	"net"
	"net/http"
	"strings"
	"websocket"
)

const VNC_WS_BUF = 32768

func vncWsHandler(w http.ResponseWriter, r *http.Request) {
	// we assume that if we got here, then the url must be sane and of
	// the format /<host>/<port>
	url := r.URL.String()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	if len(fields) != 6 {
		http.NotFound(w, r)
		return
	}

	rhost := fmt.Sprintf("%v:%v", fields[3], fields[4])

	// connect to the remote host
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		http.StatusText(500)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		go func() {
			for {
				sbuf := make([]byte, VNC_WS_BUF)
				dbuf := make([]byte, VNC_WS_BUF)

				n, err := ws.Read(sbuf)
				if err != nil {
					if !strings.Contains(err.Error(), "closed network connection") && err != io.EOF {
						log.Errorln(err)
					}
					break
				}
				if r, ok := vncRecording[rhost]; ok {
					r.AddAction(string(sbuf[:n]))
				}
				n, err = base64.StdEncoding.Decode(dbuf, sbuf[:n])
				if err != nil {
					log.Errorln(err, string(sbuf[:n]))
					break
				}
				_, err = remote.Write(dbuf[0:n])
				if err != nil {
					log.Errorln(err)
					break
				}
			}
			remote.Close()
		}()
		func() {
			sbuf := make([]byte, VNC_WS_BUF)
			dbuf := make([]byte, 2*VNC_WS_BUF)
			for {
				n, err := remote.Read(sbuf)
				if err != nil {
					if !strings.Contains(err.Error(), "closed network connection") && err != io.EOF {
						log.Errorln(err)
					}
					break
				}
				base64.StdEncoding.Encode(dbuf, sbuf[0:n])
				n = base64.StdEncoding.EncodedLen(n)

				_, err = ws.Write(dbuf[0:n])
				if err != nil {
					log.Errorln(err)
					break
				}
			}
			ws.Close()
		}()
	}).ServeHTTP(w, r)
}
