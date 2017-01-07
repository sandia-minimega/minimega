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
		log.Fatal("commandSocketStart: %v", err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Error("commandSocketStart: accept: %v", err)
			}
			log.Infoln("client connected")

			go commandSocketHandle(conn)
		}
	}()
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
		var r *miniclient.Request

		r, err = readLocalRequest(dec)
		if err != nil {
			break
		}

		if r.Suggest != "" {
			err = sendLocalSuggest(enc, minicli.Suggest(r.Suggest))
			continue
		}

		if r.Command == nil {
			err = sendLocalResp(enc, nil, false)
			continue
		}

		// HAX: Don't record the read command
		if hasCommand(r.Command, "read") {
			r.Command.SetRecord(false)
		}

		// HAX: Work around so that we can add the more boolean.
		var prev minicli.Responses

		// Keep sending until we hit the first error, then just consume the
		// channel to ensure that we release any locks acquired by cmd.
		for resp := range RunCommands(r.Command) {
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

func readLocalRequest(dec *json.Decoder) (*miniclient.Request, error) {
	var r miniclient.Request

	if err := dec.Decode(&r); err != nil {
		return nil, err
	}

	log.Debug("got request over socket: %v", r)

	if r.Command != nil {
		// HAX: Reprocess the original command since the Call target cannot be
		// serialized... is there a cleaner way to do this?
		cmd, err := minicli.Compile(r.Command.Original)
		if err != nil {
			return nil, err
		}

		r.Command = cmd
	}

	return &r, nil
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

func sendLocalSuggest(enc *json.Encoder, suggest []string) error {
	r := miniclient.Response{
		Suggest: suggest,
	}

	return enc.Encode(&r)
}
