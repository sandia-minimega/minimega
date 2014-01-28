package main

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	clients             map[int64]*Client
	clientLock          sync.Mutex
	clientExpiredCount  int
	upstreamQueue       *hb
	upstreamQueueLock   sync.Mutex
	masterResponseQueue chan map[int64][]*Response
)

func init() {
	clients = make(map[int64]*Client)
	upstreamQueue = &hb{
		Clients: make(map[int64]*Client),
	}
	masterResponseQueue = make(chan map[int64][]*Response, 1024)
	go clientReaper()
}

func processHeartbeat(h *hb) {
	clientLock.Lock()
	t := time.Now()
	mrq := make(map[int64][]*Response)
	for k, v := range h.Clients {
		clients[k] = v
		clients[k].Checkin = t
		if ronMode == MODE_MASTER && len(v.Responses) > 0 {
			mrq[k] = v.Responses
		} else {
			upstreamQueueLock.Lock()
			upstreamQueue.Clients[k] = clients[k]
			upstreamQueueLock.Unlock()
		}
		log.Debug("added/updated client: %v", k)
	}
	checkMaxCommandID(h.MaxCommandID)
	if ronMode == MODE_MASTER && len(mrq) > 0 {
		masterResponseQueue <- mrq
	}
	clientLock.Unlock()
}

func masterResponseProcessor() {
	for {
		r := <-masterResponseQueue
		for k, v := range r {
			for _, c := range v {
				log.Debug("got response %v : %v", k, c.ID)
				commandCheckIn(c.ID, k)
				if !shouldRecord(c.ID) {
					log.Debug("ignoring non recording response")
					continue
				}

				path := fmt.Sprintf("%v/responses/%v/%v/", *f_base, c.ID, k)
				err := os.MkdirAll(path, os.FileMode(0770))
				if err != nil {
					log.Errorln(err)
					log.Error("could not record response %v : %v", k, c.ID)
					continue
				}
				// generate stdout and stderr if they exist
				if c.Stdout != "" {
					err := ioutil.WriteFile(path+"stdout", []byte(c.Stdout), os.FileMode(0660))
					if err != nil {
						log.Errorln(err)
						log.Error("could not record stdout %v : %v", k, c.ID)
					}
				}
				if c.Stderr != "" {
					err := ioutil.WriteFile(path+"stderr", []byte(c.Stderr), os.FileMode(0660))
					if err != nil {
						log.Errorln(err)
						log.Error("could not record stderr %v : %v", k, c.ID)
					}
				}

				// write out files if they exist
				for f, d := range c.Files {
					fpath := fmt.Sprintf("%v/%v", path, f)
					log.Debug("writing file %v", fpath)
					dir := filepath.Dir(fpath)
					err := os.MkdirAll(dir, os.FileMode(0770))
					if err != nil {
						log.Errorln(err)
						continue
					}
					err = ioutil.WriteFile(fpath, d, os.FileMode(0660))
					if err != nil {
						log.Errorln(err)
						continue
					}
				}
			}
		}
	}
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
	upstreamQueueLock.Lock()
	defer upstreamQueueLock.Unlock()
	h := &hb{
		Clients:      upstreamQueue.Clients,
		MaxCommandID: getMaxCommandID(),
	}

	// reset the upstream queue
	upstreamQueue.Clients = make(map[int64]*Client)

	return h
}
