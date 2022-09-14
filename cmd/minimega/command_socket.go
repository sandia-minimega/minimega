// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
				continue
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
	done := make(chan struct{})

	var err error

	for err == nil {
		var r *miniclient.Request
		var cmd *minicli.Command

		r, err = readLocalRequest(dec)
		if err != nil {
			break
		}

		// check the plumbing
		if r.PlumbPipe != "" {
			reader := plumber.NewReader(r.PlumbPipe)
			writer := plumber.NewWriter(r.PlumbPipe)

			go func() {
				for {
					select {
					case <-reader.Done:
						return
					case line := <-reader.C:
						if err := enc.Encode(line); err != nil {
							if !strings.Contains(err.Error(), "write: broken pipe") {
								log.Errorln(err)
							}
							break
						}
					}
				}

			}()

			for err == nil {
				var line string
				err = dec.Decode(&line)
				if err != nil {
					break
				}

				if line != "" {
					writer <- line
				}
			}

			// stop the reader
			reader.Close()
			close(writer)

			break
		}

		// should have a command or suggestion but not both
		if (r.Command == "") == (r.Suggest == "") {
			resp := &minicli.Response{
				Host:  hostname,
				Error: "must specify either command or suggest",
			}
			err = sendLocalResp(enc, minicli.Responses{resp}, false)
			continue
		}

		// client wants a suggestion
		if r.Suggest != "" {
			err = sendLocalSuggest(enc, cliCompleter(r.Suggest))
			continue
		}

		r.Command = minicli.ExpandAliases(r.Command)

		// client specified a command
		cmd, err = minicli.Compile(r.Command)
		if err != nil {
			resp := &minicli.Response{
				Host:  hostname,
				Error: err.Error(),
			}
			err = sendLocalResp(enc, minicli.Responses{resp}, false)
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			err = sendLocalResp(enc, nil, false)
			continue
		}

		// HAX: Don't record the read command
		if hasCommand(cmd, "read") {
			cmd.SetRecord(false)
		}

		go func() {
			for {
				status := make(chan string)

				addStatusMessageChannel("cmdsocket", status)

				select {
				case <-done:
					delStatusMessageChannel("cmdsocket")
					return
				case s := <-status:
					sendLocalStatus(enc, s)
				}
			}
		}()

		// HAX: Work around so that we can add the more boolean.
		var prev minicli.Responses

		// Keep sending until we hit the first error, then just consume the
		// channel to ensure that we release any locks acquired by cmd.
		for resp := range runCommands(cmd) {
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

	close(done)

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

func sendLocalStatus(enc *json.Encoder, status string) error {
	r := miniclient.Response{
		Rendered: status,
		Status:   true,
	}

	return enc.Encode(&r)
}
