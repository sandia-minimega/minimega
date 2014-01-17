package main

import (
	"bytes"
	"encoding/gob"
	log "minilog"
	"net/http"
	"time"
)

type hb struct {
	ID           int64
	Clients      map[int64]*Client
	MaxCommandID int // the highest command ID this node has seen
	Responses    []*Response
}

func init() {
	gob.Register(hb{})
}

func heartbeat() {
	for {
		time.Sleep(time.Duration(ronRate) * time.Second)

		var h *hb
		switch ronMode {
		case MODE_MASTER:
			// do nothing
			return
		case MODE_RELAY:
			h = relayHeartbeat()
		case MODE_CLIENT:
			h = clientHeartbeat()
		default:
			log.Fatal("invalid heartbeat mode %v", ronMode)
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)

		err := enc.Encode(h)
		if err != nil {
			log.Errorln(err)
			continue
		}

		resp, err := http.Post(ronHost, "ron/miniccc", &buf)
		if err != nil {
			log.Errorln(err)
			continue
		}

		newCommands := make(map[int]*Command)
		dec := gob.NewDecoder(resp.Body)

		err = dec.Decode(&newCommands)
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}

		switch ronMode {
		case MODE_RELAY:
			// replace the command list with this one, keeping the list of respondents
			updateCommands(newCommands)
		case MODE_CLIENT:
			clientCommands(newCommands)
		}

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
	w.Write(encodeCommands())
}
