package main

import (
	log "minilog"
	"net"
	"path/filepath"
)

func commandSocketStart() {
	l, err := net.Listen("unix", filepath.Join(*f_path, "minirouter"))
	if err != nil {
		log.Fatalln("commandSocketStart: %v", err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("commandSocketStart: accept: %v", err)
		}
		log.Infoln("client connected")

		go commandSocketHandle(conn)
	}
}

func commandSocketHandle(conn net.Conn) {

}
