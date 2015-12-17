package main

import (
	"minicli"
	"miniclient"
	log "minilog"
	"strings"
)

func sendCommand(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	log.Debug("sendCommand: %v", s)

	mm, err := miniclient.Dial(*f_minimega)
	if err != nil {
		log.Errorln(err)
		return err.Error()
	}
	defer mm.Close()

	cmd := &minicli.Command{Original: s}

	var responses string
	for resp := range mm.Run(cmd) {
		if r := resp.Rendered; r != "" {
			responses += r + "\n"
		}
		if e := resp.Resp.Error(); e != "" {
			responses += e + "\n"
		}
	}
	return responses
}
