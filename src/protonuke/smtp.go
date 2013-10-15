package main

import (
	log "minilog"
)

func smtpClient() {
	log.Debugln("smtpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
	}
}
