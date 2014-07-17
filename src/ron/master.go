package ron

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"path/filepath"
	"time"
)

func (r *Ron) masterResponseProcessor() {
	for {
		responses := <-r.masterResponseQueue
		for _, v := range responses {
			uuid := v.UUID
			log.Debug("got response %v : %v", uuid, v.ID)
			r.commandCheckIn(v.ID, uuid)
			if !r.shouldRecord(v.ID) {
				log.Debug("ignoring non recording response")
				continue
			}

			path := fmt.Sprintf("%v/%v/%v/", r.path+RESPONSE_PATH, v.ID, uuid)
			err := os.MkdirAll(path, os.FileMode(0770))
			if err != nil {
				log.Errorln(err)
				log.Error("could not record response %v : %v", uuid, v.ID)
				continue
			}
			// generate stdout and stderr if they exist
			if v.Stdout != "" {
				err := ioutil.WriteFile(path+"stdout", []byte(v.Stdout), os.FileMode(0660))
				if err != nil {
					log.Errorln(err)
					log.Error("could not record stdout %v : %v", uuid, v.ID)
				}
			}
			if v.Stderr != "" {
				err := ioutil.WriteFile(path+"stderr", []byte(v.Stderr), os.FileMode(0660))
				if err != nil {
					log.Errorln(err)
					log.Error("could not record stderr %v : %v", uuid, v.ID)
				}
			}

			// write out files if they exist
			for f, d := range v.Files {
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

// clientReaper periodically flushes old entries from the client list
func (r *Ron) clientReaper() {
	for {
		time.Sleep(time.Duration(REAPER_RATE) * time.Second)
		log.Debugln("clientReaper")
		t := time.Now()
		r.clientLock.Lock()
		for k, v := range r.clients {
			if v.reap(t) {
				log.Debug("client %v expired", k)
				r.clientExpiredCount++
				delete(r.clients, k)
			}
		}
		r.clientLock.Unlock()
	}
}

func (c *Client) reap(t time.Time) bool {
	if t.Sub(c.Checkin) > (time.Duration(CLIENT_EXPIRED) * time.Second) {
		return true
	}
	return false
}
