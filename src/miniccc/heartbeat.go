package main

import (
	"bytes"
	"encoding/gob"
	"io"
	log "minilog"
	"net/http"
	"time"
)

type hb struct {
	ID      string
	Clients map[string]*Client
	//	S Stats
	//	R []Responses
}

func init() {
	gob.Register(hb{})
}

func (r *ron) heartbeat() {
	for {
		time.Sleep(time.Duration(r.rate) * time.Second)

		var h *hb
		switch r.mode {
		case MODE_MASTER:
			// do nothing
			return
		case MODE_RELAY:
			h = relayHeartbeat()
		case MODE_CLIENT:
			h = clientHeartbeat()
		default:
			log.Fatal("invalid heartbeat mode %v", r.mode)
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)

		enc.Encode(h)

		resp, err := http.Post(r.host, "ron/miniccc", &buf)
		if err != nil {
			log.Errorln(err)
			continue
		}

		// debug
		var buf2 bytes.Buffer
		io.Copy(&buf2, resp.Body)

		log.Debugln(buf2.String())
		resp.Body.Close()
	}
}

// heartbeat is the means of communication between clients and an upstream
// parent. Clients will send status and any responses from completed commands
// in a POST, while existing commands will be returned as the response.
func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		log.Error("no data received: %v %v", r.RemoteAddr, r.URL)
		return
	}
	defer r.Body.Close()
	dec := gob.NewDecoder(r.Body)
	var h hb
	err := dec.Decode(&h)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debug("heartbeat from %v", h.ID)

	// process the heartbeat in a goroutine so we can send the command list back faster
	go processHeartbeat(&h)

	// send the command list back
}
