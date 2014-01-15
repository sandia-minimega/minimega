package main

import (
	"fmt"
	log "minilog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	CID      string
	Hostname string
	Arch     string
	OS       string
	IP       []string
	MAC      []string
	Checkin  time.Time
}

var (
	clients            map[string]*Client
	clientLock         sync.Mutex
	clientExpiredCount int
	upstreamQueue      *hb
)

func init() {
	clients = make(map[string]*Client)
	upstreamQueue = &hb{
		Clients: make(map[string]*Client),
	}
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
	checkMaxCommandID(h.MaxCommandID)
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
	log.Debugln("handleRoot")
	clientLock.Lock()
	numClients := len(clients)
	archMix := make(map[string]int)
	osMix := make(map[string]int)
	for _, v := range clients {
		archMix[v.Arch]++
		osMix[v.OS]++
	}
	clientLock.Unlock()

	// TODO: html template
	resp := fmt.Sprintf("<html>Active clients: %v<br>Expired clients: %v<br>", numClients, clientExpiredCount)
	resp += "Architecture Mix:<br>"
	for k, v := range archMix {
		resp += fmt.Sprintf("%v (%.02f%%)<br>", k, 100*(float64(v)/float64(numClients)))
	}
	resp += "OS Mix:<br>"
	for k, v := range osMix {
		resp += fmt.Sprintf("%v (%.02f%%)<br>", k, 100*(float64(v)/float64(numClients)))
	}
	w.Write([]byte(resp))
}

func handleList(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleList")
	raw := false
	if strings.HasSuffix(r.URL.Path, "raw") {
		raw = true
	}

	var resp string
	if !raw {
		resp += "<html><table border=1><tr><td>CID</td><td>Hostname</td><td>Arch</td><td>OS</td><td>IP</td><td>MAC</td></tr>"
	} else {
		resp += "CID,Hostname,Arch,OS,IP,MAC\n"
	}
	clientLock.Lock()
	for _, v := range clients {
		if !raw {
			resp += fmt.Sprintf("<tr><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td></tr>", v.CID, v.Hostname, v.Arch, v.OS, v.IP, v.MAC)
		} else {
			resp += fmt.Sprintf("%v,%v,%v,%v,%v,%v\n", v.CID, v.Hostname, v.Arch, v.OS, v.IP, v.MAC)
		}
	}
	clientLock.Unlock()
	if !raw {
		resp += "</table></html>"
	}

	w.Write([]byte(resp))
}

func relayHeartbeat() *hb {
	log.Debugln("relayHeartbeat")
	// deep copy of the upstream queue
	clientLock.Lock()
	defer clientLock.Unlock()
	h := &hb{
		Clients:      upstreamQueue.Clients,
		MaxCommandID: getMaxCommandID(),
	}

	// reset the upstream queue
	upstreamQueue.Clients = make(map[string]*Client)

	return h
}
