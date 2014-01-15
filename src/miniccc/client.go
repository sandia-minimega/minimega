package main

import (
	"encoding/base64"
	"math/rand"
	log "minilog"
	"net"
	"os"
	"runtime"
	"time"
)

var CID string

func clientSetup() {
	log.Debugln("clientSetup")

	// generate a random byte slice
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	b := make([]byte, 16)
	for i, _ := range b {
		b[i] = byte(r.Int())
	}

	CID = base64.StdEncoding.EncodeToString(b)
	log.Debug("CID: %v", CID)
}

func clientHeartbeat() *hb {
	log.Debugln("clientHeartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	c := &Client{
		CID:      CID,
		Arch:     runtime.GOARCH,
		OS:       runtime.GOOS,
		Hostname: hostname,
	}

	// process network info
	ints, err := net.Interfaces()
	if err != nil {
		log.Fatalln(err)
	}
	for _, v := range ints {
		if v.HardwareAddr.String() == "" {
			// skip localhost and other weird interfaces
			continue
		}
		log.Debug("found mac: %v", v.HardwareAddr)
		c.MAC = append(c.MAC, v.HardwareAddr.String())
		addrs, err := v.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, w := range addrs {
			log.Debug("found ip: %v", w)
			c.IP = append(c.IP, w.String())
		}
	}

	me := make(map[string]*Client)
	me[CID] = c
	h := &hb{
		ID:           CID,
		Clients:      me,
		MaxCommandID: getMaxCommandID(),
	}
	log.Debug("client heartbeat %v", h)
	return h
}

func clientCommands(newCommands map[int]*Command) {
	// nothing for now

}
