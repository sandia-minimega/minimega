// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	log "minilog"
	"net/http"
	"os"
	"runtime"
	"time"
)

type hb struct {
	UUID         string
	Client       *Client
	MaxCommandID int    // the highest command ID this node has seen
	File         string // used by serial connections to initiate file get operations instead of a true heartbeat
}

func init() {
	gob.Register(hb{})
}

func (r *Ron) heartbeat() {
	log.Debugln("heartbeat")

	s := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(s)
	for {
		time.Sleep(time.Duration(r.rate) * time.Second)

		h := r.clientHeartbeat()

		first := true
		for {
			if !first {
				wait := rnd.Intn(r.rate)
				log.Debug("ron retry heartbeat after %v seconds", wait)
				time.Sleep(time.Duration(wait) * time.Second)
			} else {
				first = false
			}

			newCommands, err, posted := r.submitHeartbeat(h)
			if err != nil {
				log.Errorln(err)
				if posted {
					break
				} else {
					continue
				}
			}

			r.clientCommands(newCommands)

			break
		}
	}
}

func (r *Ron) submitHeartbeat(h *hb) (map[int]*Command, error, bool) {
	if r.serialPath != "" {
		return r.serialHeartbeat(h)
	} else {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)

		err := enc.Encode(h)
		if err != nil {
			return nil, err, false
		}

		host := fmt.Sprintf("http://%v:%v/heartbeat", r.parent, r.port)
		log.Debug("ron host %v", host)

		resp, err := http.Post(host, "ron/miniccc", &buf)
		if err != nil {
			return nil, err, false
		}

		newCommands := make(map[int]*Command)
		dec := gob.NewDecoder(resp.Body)

		err = dec.Decode(&newCommands)
		if err != nil {
			resp.Body.Close()
			return nil, err, true
		}
		resp.Body.Close()
		return newCommands, nil, true
	}
}

// heartbeat is the means of communication between clients and an upstream
// parent. Clients will send status and any responses from completed commands
// in a POST, while existing commands will be returned as the response.
func (r *Ron) handleHeartbeat(w http.ResponseWriter, req *http.Request) {
	if req.Body == nil {
		log.Error("no data received: %v %v", req.RemoteAddr, req.URL)
		return
	}
	defer req.Body.Close()
	dec := gob.NewDecoder(req.Body)
	var h hb
	err := dec.Decode(&h)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debug("heartbeat from %v", h.UUID)

	// process the heartbeat in a goroutine so we can send the command list back faster
	go r.masterHeartbeat(&h)

	// send the command list back
	buf, err := r.encodeCommands()
	if err != nil {
		log.Errorln(err)
		return
	}
	w.Write(buf)
}

func (r *Ron) masterHeartbeat(h *hb) {
	r.clientLock.Lock()
	t := time.Now()
	r.clients[h.Client.UUID] = h.Client
	r.clients[h.Client.UUID].Checkin = t

	r.checkMaxCommandID(h.MaxCommandID)

	if len(h.Client.Responses) > 0 {
		r.masterResponseQueue <- h.Client.Responses
	}

	// don't track responses in our client list
	r.clients[h.Client.UUID].Responses = []*Response{}

	r.clientLock.Unlock()
}

func (r *Ron) clientHeartbeat() *hb {
	log.Debugln("clientHeartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	c := &Client{
		UUID:     r.UUID,
		Arch:     runtime.GOARCH,
		OS:       runtime.GOOS,
		Hostname: hostname,
	}

	macs, ips := getNetworkInfo()
	c.MAC = macs
	c.IP = ips

	// attach any command responses and clear the response queue
	r.responseQueueLock.Lock()
	c.Responses = r.clientResponseQueue
	r.clientResponseQueue = []*Response{}
	r.responseQueueLock.Unlock()

	h := &hb{
		UUID:         c.UUID,
		Client:       c,
		MaxCommandID: r.getMaxCommandID(),
	}

	log.Debug("client heartbeat %v", h)
	return h
}
