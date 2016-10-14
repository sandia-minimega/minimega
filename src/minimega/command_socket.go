// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"io"
	"minicli"
	"miniclient"
	log "minilog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func commandSocketStart() {
	l, err := net.Listen("unix", filepath.Join(*f_base, "minimega"))
	if err != nil {
		log.Error("commandSocketStart: %v", err)
		teardown()
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("commandSocketStart: accept: %v", err)
		}
		log.Infoln("client connected")

		go commandSocketHandle(conn)
	}
}

func commandSocketRemove() {
	f := filepath.Join(*f_base, "minimega")
	err := os.Remove(f)
	if err != nil {
		log.Errorln(err)
	}
}

func commandSocketHandle(c net.Conn) {
	defer c.Close()

	enc := json.NewEncoder(c)
	dec := json.NewDecoder(c)

	var err error

	for err == nil {
		var cmd *minicli.Command

		cmd, err = readLocalCommand(dec)
		if err != nil {
			break
		}

		if cmd == nil {
			err = sendLocalResp(enc, nil, false)
			continue
		}

		// HAX: Don't record the read command
		if hasCommand(cmd, "read") {
			cmd.SetRecord(false)
		}

		// HAX: Work around so that we can add the more boolean.
		var prev minicli.Responses

		// Keep sending until we hit the first error, then just consume the
		// channel to ensure that we release any locks acquired by cmd.
		for resp := range RunCommands(cmd) {
			if prev != nil && err == nil {
				err = sendLocalResp(enc, prev, true)
			} else if err != nil && len(resp) > 0 {
				log.Info("dropping resp from %v", resp[0].Host)
			}

			prev = resp
		}

		if err == nil {
			err = sendLocalResp(enc, prev, false)
		}
	}

	// finally, log the error, if there was one
	if err == nil || err == io.EOF {
		log.Infoln("command client disconnected")
	} else if err != nil && strings.Contains(err.Error(), "write: broken pipe") {
		log.Infoln("command client disconnected without waiting for responses")
	} else if err != nil {
		log.Errorln(err)
	}
}

func readLocalCommand(dec *json.Decoder) (*minicli.Command, error) {
	var cmd minicli.Command

	if err := dec.Decode(&cmd); err != nil {
		return nil, err
	}

	log.Debug("got command over socket: %v", cmd)

	// HAX: Reprocess the original command since the Call target cannot be
	// serialized... is there a cleaner way to do this?
	return minicli.Compile(cmd.Original)
}

func sendLocalResp(enc *json.Encoder, resp minicli.Responses, more bool) error {
	r := miniclient.Response{
		More: more,
	}
	if resp != nil {
		r.Resp = resp
		r.Rendered = resp.String()
	}

	return enc.Encode(&r)
}
