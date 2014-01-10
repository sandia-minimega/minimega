package main

import (
	"encoding/base64"
	"math/rand"
	log "minilog"
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
	c := &Client{
		CID: CID,
	}
	me := make(map[string]*Client)
	me[CID] = c
	h := &hb{
		ID:      CID,
		Clients: me,
	}
	log.Debug("client heartbeat %v", h)
	return h
}
