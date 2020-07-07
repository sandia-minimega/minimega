package main

import (
	"bufio"
	"net"
	"path/filepath"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func commandSocketStart() {
	l, err := net.Listen("unix", filepath.Join(*f_path, "minirouter"))
	if err != nil {
		log.Fatal("commandSocketStart: %v", err)
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
	// just read comments off the wire
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug("got command: %v", line)
		r, err := minicli.ProcessString(line, false)
		if err != nil {
			log.Errorln(err)
		}
		<-r
	}
	if err := scanner.Err(); err != nil {
		log.Errorln(err)
	}
	conn.Close()
}
