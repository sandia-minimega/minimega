// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"

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
			log.Debugln("client connected")

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
			err = commandSocketPipe(enc, dec, r.PlumbPipe)
			break
		}

		// should have a command or suggestion but not both
		if (r.Command == "") == (r.Suggest == "") {
			resp := &minicli.Response{
				Host:  hostname,
				Error: "must specify either command or suggest",
			}
			err = sendLocalResp(enc, minicli.Responses{resp}, false, r.TTY)
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
			err = sendLocalResp(enc, minicli.Responses{resp}, false, r.TTY)
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			err = sendLocalResp(enc, nil, false, r.TTY)
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
				err = sendLocalResp(enc, prev, true, r.TTY)
			} else if err != nil && len(resp) > 0 {
				log.Info("dropping resp from %v", resp[0].Host)
			}

			prev = resp
		}

		if err == nil {
			err = sendLocalResp(enc, prev, false, r.TTY)
		}
	}

	close(done)

	// finally, log the error, if there was one
	if err == nil || err == io.EOF {
		log.Debugln("command client disconnected")
	} else if isClientGone(err) {
		log.Infoln("command client disconnected without waiting for responses")
	} else {
		log.Errorln(err)
	}
}

// isClientGone reports whether err is how a write to, or a read from, the
// command socket fails when the client disconnected early: EPIPE on a write
// and, if the client left data it was sent unread, ECONNRESET on a read.
func isClientGone(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

// commandSocketPipe attaches a client to a named pipe: lines from the client
// are written to the pipe and lines written to the pipe are sent to the
// client, until the client closes its end of the connection or disconnects.
//
// It returns the error that ended the client's input (io.EOF if the client
// closed its end), once every line from the client has been written to the
// pipe and nothing more will be sent to the client, so the caller can close
// the connection.
func commandSocketPipe(enc *json.Encoder, dec *json.Decoder, pipe string) error {
	reader := plumber.NewReader(pipe)
	writer, written := plumber.NewWriterDone(pipe)

	// Send lines written to the pipe to the client. Nothing else uses enc
	// while the client is attached to the pipe.
	sent := make(chan struct{})
	go func() {
		defer close(sent)

		var gone bool

		// C is closed once the reader is closed and off the pipe
		for line := range reader.C {
			if gone {
				continue
			}

			if err := enc.Encode(line); err != nil {
				if !isClientGone(err) {
					log.Error("pipe %v: %v", pipe, err)
				}

				// stop the plumber from delivering to a client that is gone
				gone = true
				reader.Close()
			}
		}
	}()

	var err error
	for {
		var line string
		if err = dec.Decode(&line); err != nil {
			break
		}

		if line != "" {
			writer <- line
		}
	}

	// The client has nothing more to write, or is gone. Finish writing its
	// lines to the pipe while the reader still drains the pipe, then stop the
	// reader and wait for it, so that nothing is sent on a closed connection
	// and the client is off the pipe when its connection is closed.
	close(writer)
	<-written

	reader.Close()
	<-sent

	return err
}

func readLocalRequest(dec *json.Decoder) (*miniclient.Request, error) {
	var r miniclient.Request

	if err := dec.Decode(&r); err != nil {
		return nil, err
	}

	log.Debug("got request over socket: %v", r)

	return &r, nil
}

// sendLocalResp sends resp to the client. Column width limits only apply when
// the client prints to a terminal.
func sendLocalResp(enc *json.Encoder, resp minicli.Responses, more, tty bool) error {
	r := miniclient.Response{
		More: more,
	}
	if resp != nil {
		r.Resp = resp
		if tty {
			r.Rendered = resp.String()
		} else {
			r.Rendered = resp.FullString()
		}
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
