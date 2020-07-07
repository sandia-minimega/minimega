package main

// everything after sendCommand is forked from miniclient and trimmed to build
// in appengine

import (
	"encoding/json"
	"io"
	"net"
	"path"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func sendCommand(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	log.Debug("sendCommand: %v", s)

	mm, err := dial(*f_minimega)
	if err != nil {
		log.Errorln(err)
		return err.Error()
	}
	defer mm.Close()

	var responses string
	for resp := range mm.Run(s) {
		if r := resp.Rendered; r != "" {
			responses += r + "\n"
		}
		if e := resp.Resp.Error(); e != "" {
			responses += e + "\n"
		}
	}
	return responses
}

type Request struct {
	Command   string
	Suggest   string
	PlumbPipe string
}

type Response struct {
	// Resp, Rendered, More are returned in response to a command
	Resp     minicli.Responses
	Rendered string
	More     bool // whether there are more responses coming

	// Suggest is returned in response to Suggest request
	Suggest []string `json:"omitempty"`
}

type Conn struct {
	url string

	conn net.Conn

	enc *json.Encoder
	dec *json.Decoder
}

func dial(base string) (*Conn, error) {
	var mm = &Conn{
		url: path.Join(base, "minimega"),
	}
	var err error

	// try to connect to the local minimega
	mm.conn, err = net.Dial("unix", mm.url)
	if err != nil {
		return nil, err
	}

	mm.enc = json.NewEncoder(mm.conn)
	mm.dec = json.NewDecoder(mm.conn)

	return mm, nil
}

func (mm *Conn) Close() error {
	return mm.conn.Close()
}

// Run a command through a JSON pipe, hand back channel for responses.
func (mm *Conn) Run(cmd string) chan *Response {
	if cmd == "" {
		// Language spec: "Receiving from a nil channel blocks forever."
		// Instead, make and immediately close the channel so that range
		// doesn't block and receives no values.
		out := make(chan *Response)
		close(out)

		return out
	}

	err := mm.enc.Encode(Request{Command: cmd})
	if err != nil {
		log.Fatal("local command gob encode: %v", err)
	}
	log.Debugln("encoded command:", cmd)

	respChan := make(chan *Response)

	go func() {
		defer close(respChan)

		for {
			var r Response
			err = mm.dec.Decode(&r)
			if err != nil {
				if err == io.EOF {
					log.Fatal("server disconnected")
				}

				log.Fatal("local command gob decode: %v", err)
			}

			respChan <- &r
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}()

	return respChan
}
