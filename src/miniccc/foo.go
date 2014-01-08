package main

import (
	"ron"
	log "minilog"
	"encoding/base64"
	"encoding/gob"
	"math/rand"
	"time"
)

var CID string

type hb struct {
	CID string
//	S Stats
//	R []Responses
}

func init() {
	gob.Register(hb{})
}

func clientSetup() {
	log.Debugln("clientSetup")

	// generate a random byte slice
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	b := make([]byte, 128)
	for i, _ := range b {
		b[i] = byte(r.Int())
	}

	CID := base64.StdEncoding.EncodeToString(b)
	log.Debug("CID: %v", CID)
}

func clientHeartbeat(r *ron) {
	log.Debugln("clientHeartbeat")

	h := &hb{
		CID: CID,
	}

	r.Enc.Encode(h)
}
