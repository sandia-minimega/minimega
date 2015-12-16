// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package miniclient

import (
	"encoding/json"
	"fmt"
	"goreadline"
	"io"
	"minicli"
	log "minilog"
	"minipager"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
)

type Response struct {
	Resp     minicli.Responses
	Rendered string
	More     bool // whether there are more responses coming
}

type Conn struct {
	url string

	conn net.Conn

	enc *json.Encoder
	dec *json.Decoder

	// Set the Pager to use for long output messages
	Pager minipager.Pager
}

func Dial(base string) (*Conn, error) {
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
func (mm *Conn) Run(cmd *minicli.Command) chan *Response {
	if cmd == nil {
		// Language spec: "Receiving from a nil channel blocks forever."
		// Instead, make and immediately close the channel so that range
		// doesn't block and receives no values.
		out := make(chan *Response)
		close(out)

		return out
	}

	err := mm.enc.Encode(*cmd)
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

// Run a command and print the response.
func (mm *Conn) RunAndPrint(cmd *minicli.Command, page bool) {
	for resp := range mm.Run(cmd) {
		if page && mm.Pager != nil {
			mm.Pager.Page(resp.Rendered)
		} else if resp.Rendered != "" {
			fmt.Println(resp.Rendered)
		}

		errs := resp.Resp.Error()
		if errs != "" {
			fmt.Fprintln(os.Stderr, errs)
		}
	}
}

// Attach creates a CLI interface to the dialed minimega instance
func (mm *Conn) Attach() {
	// set up signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Debug("caught signal, disconnecting")
		goreadline.Rlcleanup()
		os.Exit(0)
	}()

	// start our own rlwrap
	fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
	fmt.Println("use 'disconnect' or ^d to exit just the minimega command line")
	fmt.Println()
	defer goreadline.Rlcleanup()

	var exitNext bool
	for {
		prompt := fmt.Sprintf("minimega:%v$ ", mm.url)
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			return
		}
		command := string(line)
		log.Debug("got from stdin: `%s`", line)

		// HAX: Shortcut some commands without using minicli
		if command == "disconnect" {
			log.Debugln("disconnecting")
			return
		} else if command == "quit" {
			if !exitNext {
				fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
				fmt.Println("If you really want to stop the minimega daemon, enter 'quit' again")
				exitNext = true
				continue
			}
		}

		exitNext = false

		cmd, err := minicli.Compile(command)
		if err != nil {
			log.Error("%v", err)
			//fmt.Println("closest match: TODO")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		mm.RunAndPrint(cmd, true)

		if command == "quit" {
			return
		}
	}
}
