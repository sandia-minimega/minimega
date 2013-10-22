package main

import (
	log "minilog"
)

func httpClient() {
	log.Debugln("httpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("http host %v from %v", h, o)
	}
}

func httpServer() {
	http.HandleFunc("/", httpHandler)
	log.Fatalln(http.ListenAndServe(":80", nil))
}

func httpHandler(w http.ResponseWriter, r *http.Request) {

}
