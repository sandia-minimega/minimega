// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package miniclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"minipager"
	"net"
	"os"
	"path"
	"sync"

	"github.com/chzyer/readline"
)

// Request sent to minimega -- ethier a command to run or a string to return
// suggestions for
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

	// lock so we don't try to use enc/dec for concurrent Runs
	lock sync.Mutex

	enc *json.Encoder
	dec *json.Decoder

	// Set the Pager to use for long output messages
	Pager minipager.Pager
}

func Dial(base string) (*Conn, error) {
	var mm = &Conn{
		url: path.Join(base, "minimega"),
	}

	// try to connect to the local minimega
	conn, err := net.Dial("unix", mm.url)
	if err != nil {
		return nil, err
	}

	mm.conn = conn
	mm.enc = json.NewEncoder(mm.conn)
	mm.dec = json.NewDecoder(mm.conn)

	return mm, nil
}

func (mm *Conn) Close() error {
	return mm.conn.Close()
}

// Read or write to a named pipe.
func (mm *Conn) Pipe(pipe string) (io.Reader, io.WriteCloser) {
	err := mm.enc.Encode(Request{
		PlumbPipe: pipe,
	})
	if err != nil {
		log.Fatal("local pipe gob encode: %v", err)
	}

	rr, rw, err := os.Pipe()
	if err != nil {
		log.Fatalln(err)
	}
	wr, ww, err := os.Pipe()
	if err != nil {
		log.Fatalln(err)
	}

	var done bool

	go func() {
		var buf string
		defer rr.Close()
		for {
			err := mm.dec.Decode(&buf)
			if done {
				return
			}
			if err == io.EOF {
				log.Fatalln("server disconnected")
			} else if err != nil {
				log.Fatal("local command gob decode: %v", err)
			}

			_, err = rw.WriteString(buf)
			if done {
				return
			}
			if err != nil {
				log.Fatal("write: %v", err)
			}
		}
	}()

	go func() {
		defer rw.Close()
		defer mm.Close()
		for {
			scanner := bufio.NewScanner(wr)
			for scanner.Scan() {
				err = mm.enc.Encode(scanner.Text() + "\n")
				if err != nil {
					log.Fatal("local command gob encode: %v", err)
				}
			}

			// scanners don't return EOF errors
			if err := scanner.Err(); err != nil {
				log.Fatal("read: %v", err)
			}

			log.Debugln("client closed stdin")
			done = true
			return
		}
	}()

	return rr, ww
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

	mm.lock.Lock()

	err := mm.enc.Encode(Request{Command: cmd})
	if err != nil {
		log.Fatal("local command gob encode: %v", err)
	}
	log.Debugln("encoded command:", cmd)

	respChan := make(chan *Response)

	go func() {
		defer mm.lock.Unlock()
		defer close(respChan)

		for {
			var r Response
			if err := mm.dec.Decode(&r); err != nil {
				if err == io.EOF {
					log.Fatalln("server disconnected")
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
func (mm *Conn) RunAndPrint(cmd string, page bool) {
	for resp := range mm.Run(cmd) {
		if page && mm.Pager != nil {
			mm.Pager.Page(resp.Rendered)
		} else if resp.Rendered != "" {
			fmt.Println(resp.Rendered)
		}

		if err := resp.Resp.Error(); err != "" {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

func (mm *Conn) Suggest(input string) []string {
	err := mm.enc.Encode(Request{Suggest: input})
	if err != nil {
		log.Fatal("local command gob encode: %v", err)
	}
	log.Debugln("encoded suggest:", input)

	var r Response
	if err := mm.dec.Decode(&r); err != nil {
		if err == io.EOF {
			log.Fatalln("server disconnected")
		}

		log.Fatal("local command gob decode: %v", err)
	}

	return r.Suggest
}

// Attach creates a CLI interface to the dialed minimega instance
func (mm *Conn) Attach() {
	fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
	fmt.Println("use 'disconnect' or ^d to exit just the minimega command line")
	fmt.Println()

	rl, err := readline.New("")
	if err != nil {
		log.Fatalln(err)
	}
	defer rl.Close()

	var quit bool
	for {
		rl.SetPrompt(fmt.Sprintf("minimega:%v$ ", mm.url))

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		} else if err == io.EOF {
			break
		}

		log.Debug("got from stdin: `%s`", line)

		// don't bother sending blank lines to minimega
		if line == "" {
			continue
		}

		// HAX: Shortcut some commands without sending them to minimega
		if line == "disconnect" {
			log.Debugln("disconnecting")
			return
		} else if line == "quit" && !quit {
			fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
			fmt.Println("If you really want to stop the minimega daemon, enter 'quit' again")
			quit = true
			continue
		}

		quit = false

		mm.RunAndPrint(line, true)
	}
}
