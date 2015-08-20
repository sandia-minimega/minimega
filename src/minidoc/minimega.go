package main

import (
	"minicli"
	log "minilog"
	"strings"
)

func sendCommand(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	log.Debug("sendCommand: %v", s)

	mm, err := Dial(*f_minimega)
	if err != nil {
		log.Errorln(err)
		return err.Error()
	}
	defer mm.Close()

	cmd := &minicli.Command{Original: s}

	var responses string
	for resp := range mm.Run(cmd) {
		r := resp.Resp.String()
		e := resp.Resp.Error()
		if r != "" {
			responses += r + "\n"
		}
		if e != "" {
			responses += e + "\n"
		}
	}
	return responses
}
