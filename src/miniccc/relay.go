package main

import (
	"fmt"
	log "minilog"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	CID     string
	Checkin time.Time
}

var (
	clients            map[string]*Client
	clientLock         sync.Mutex
	clientExpiredCount int
	upstreamQueue      *hb
)

func init() {
	clients = make(map[string]*Client)
	go clientReaper()
}

func processHeartbeat(h *hb) {
	clientLock.Lock()
	t := time.Now()
	for k, v := range h.Clients {
		clients[k] = v
		clients[k].Checkin = t
		upstreamQueue.Clients[k] = clients[k]
		log.Debug("added/updated client: %v", k)
	}
	clientLock.Unlock()
}

// clientReaper periodically flushes old entries from the client list
func clientReaper() {
	for {
		time.Sleep(time.Duration(REAPER_RATE) * time.Second)
		log.Debugln("clientReaper")
		t := time.Now()
		clientLock.Lock()
		for k, v := range clients {
			if v.Reap(t) {
				log.Debug("client %v expired", k)
				clientExpiredCount++
				delete(clients, k)
			}
		}
		clientLock.Unlock()
	}
}

func (c *Client) Reap(t time.Time) bool {
	if t.Sub(c.Checkin) > (time.Duration(CLIENT_EXPIRED) * time.Second) {
		return true
	}
	return false
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	numClients := len(clients)

	// TODO: html template
	resp := fmt.Sprintf("<html>Active clients: %v<br>Expired clients: %v</html>", numClients, clientExpiredCount)
	w.Write([]byte(resp))
}

func relayHeartbeat() *hb {
	log.Debugln("relayHeartbeat")
	// deep copy of the upstream queue
	clientLock.Lock()
	defer clientLock.Unlock()
	h := &hb{
		Clients: upstreamQueue.Clients,
	}

	// reset the upstream queue
	upstreamQueue.Clients = make(map[string]*Client)

	return h
}
