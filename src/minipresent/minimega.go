package main

import (
	"encoding/json"
	"io"
	log "minilog"
	"net"
	"strings"
)

type cliCommand struct {
	Command string
	Args    []string
	ackChan chan cliResponse
	TID     int32
}

type cliResponse struct {
	Response string
	Error    string // because you can't gob/json encode an error type
	More     bool   // more is set if the called command will be sending multiple responses
	TID      int32
}

func makeCommand(s string) cliCommand {
	f := strings.Fields(s)
	var command string
	var args []string
	if len(f) > 0 {
		command = f[0]
	}
	if len(f) > 1 {
		args = f[1:]
	}
	return cliCommand{
		Command: command,
		Args:    args,
	}
}

func sendCommand(c cliCommand) cliResponse {
	// try to connect to the local minimega
	f := *f_minimega + "minimega"
	conn, err := net.Dial("unix", f)
	if err != nil {
		log.Fatalln(err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	err = enc.Encode(&c)
	if err != nil {
		log.Error("local command gob encode: %v", err)
		return cliResponse{
			Error: err.Error(),
		}
	}
	log.Debugln("encoded command:", c)

	var Responses string
	var Errors string
	for {
		var r cliResponse
		err = dec.Decode(&r)
		if err != nil {
			if err == io.EOF {
				log.Infoln("server disconnected")
			} else {
				log.Error("local command gob decode: %v", err)
			}
			return cliResponse{
				Error: err.Error(),
			}
		}
		if r.Error != "" {
			Errors += r.Error
			if !strings.HasSuffix(r.Error, "\n") {
				Errors += "\n"
			}
			log.Errorln(r.Error)
		}
		if r.Response != "" {
			Responses += r.Response
			if !strings.HasSuffix(r.Response, "\n") {
				Responses += "\n"
			}
		}
		if !r.More {
			log.Debugln("got last message")
			break
		} else {
			log.Debugln("expecting more data")
		}
	}
	return cliResponse{
		Response: Responses,
		Error:    Errors,
	}
}
