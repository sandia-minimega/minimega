package main

import (
	"encoding/json"
	"io"
	log "minilog"
	"net"
	"path"
	"strings"
)

// from minicli/command.go
// we only need to populate the Original string
type Command struct {
	Pattern  string // the specific pattern that was matched
	Original string // original raw input

	StringArgs map[string]string
	BoolArgs   map[string]bool
	ListArgs   map[string][]string

	Subcommand *Command // parsed command

	Call CLIFunc `json:"-"`
}

type CLIFunc func(*Command, chan Responses)
type Responses []*Response

// A response as populated by handler functions.
type Response struct {
	Host     string      // Host this response was created on
	Response string      // Simple response
	Header   []string    // Optional header. If set, will be used for both Response and Tabular data.
	Tabular  [][]string  // Optional tabular data. If set, Response will be ignored
	Error    string      // Because you can't gob/json encode an error type
	Data     interface{} // Optional user data
}

type localResponse struct {
	Resp     Responses
	Rendered string
	More     bool // whether there are more responses coming
}

func sendCommand(s string) (string, string) {
	if strings.TrimSpace(s) == "" {
		return "", ""
	}

	log.Debug("sendCommand: %v", s)

	c := Command{
		Original: s,
	}

	var responses string
	var errors string
	for resp := range runCommand(c) {
		if resp.Rendered != "" {
			// strip out any errors
			d := strings.SplitN(resp.Rendered, "Error", 1)
			if d[0] != "" {
				responses += d[0] + "\n"
			}
			if len(d) == 2 && d[1] != "" {
				errors += "Error" + d[1] + "\n"
			}
		}
	}
	return responses, errors
}

// runCommand runs a command through a JSON pipe.
func runCommand(cmd Command) chan *localResponse {
	conn, err := net.Dial("unix", path.Join(*f_minimega, "minimega"))
	if err != nil {
		log.Errorln(err)
		return nil
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	log.Debug("encoding command: %v", cmd)

	err = enc.Encode(cmd)
	if err != nil {
		log.Errorln("local command json encode: %v", err)
		return nil
	}

	log.Debugln("encoded command:", cmd)

	respChan := make(chan *localResponse)

	go func() {
		defer close(respChan)

		for {
			var r localResponse
			err = dec.Decode(&r)
			if err != nil {
				if err == io.EOF {
					log.Infoln("server disconnected")
					return
				}

				log.Errorln("local command json decode: %v", err)
				return
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
