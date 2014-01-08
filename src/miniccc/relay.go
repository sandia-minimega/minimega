// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"net/http"
)

func (r *ron) newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron", easter)
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/", http.NotFound)

	host := fmt.Sprintf(":%v", r.port)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	return nil
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
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)

}
