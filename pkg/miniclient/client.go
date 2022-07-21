// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/minipager"

	"github.com/peterh/liner"
)

const (
	TOKEN_MAX = 1024 * 1024
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
	Suggest []string `json:",omitempty"`
}

type Conn struct {
	url string

	// first error encountered
	err error

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

	var conn net.Conn

	var backoff = 10 * time.Millisecond

	// try to connect to the local minimega
	for {
		var err error

		conn, err = net.Dial("unix", mm.url)
		if err == nil {
			break
		} else if err, ok := err.(*net.OpError); ok && err.Temporary() {
			time.Sleep(backoff)
			backoff *= 2
		} else {
			return nil, err
		}
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
		mm.err = fmt.Errorf("local pipe gob encode: %v", err)
		return nil, nil
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
				mm.err = errors.New("server disconnected")
				return
			} else if err != nil {
				mm.err = fmt.Errorf("local command gob decode: %v", err)
				return
			}

			_, err = rw.WriteString(buf)
			if done {
				return
			}
			if err != nil {
				mm.err = fmt.Errorf("write: %v", err)
				return
			}
		}
	}()

	go func() {
		defer rw.Close()
		defer mm.Close()
		for {
			scanner := bufio.NewScanner(wr)
			buf := make([]byte, 0, TOKEN_MAX)
			scanner.Buffer(buf, TOKEN_MAX)
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
	out := make(chan *Response)

	if cmd == "" {
		// Language spec: "Receiving from a nil channel blocks forever."
		// Instead, make and immediately close the channel so that range
		// doesn't block and receives no values.
		close(out)
		return out
	}

	mm.lock.Lock()

	err := mm.enc.Encode(Request{Command: cmd})
	if err != nil {
		mm.err = fmt.Errorf("local command gob encode: %v", err)

		mm.lock.Unlock()

		// see above
		close(out)
		return out
	}
	log.Debugln("encoded command:", cmd)

	go func() {
		defer mm.lock.Unlock()
		defer close(out)

		for {
			var r Response
			if err := mm.dec.Decode(&r); err != nil {
				if err == io.EOF {
					mm.err = errors.New("server disconnected")
					return
				}

				mm.err = fmt.Errorf("local command gob decode: %v", err)
				return
			}

			out <- &r
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}()

	return out
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
	mm.lock.Lock()
	defer mm.lock.Unlock()

	err := mm.enc.Encode(Request{Suggest: input})
	if err != nil {
		mm.err = fmt.Errorf("local command gob encode: %v", err)
		return nil
	}
	log.Debugln("encoded suggest:", input)

	var r Response
	if err := mm.dec.Decode(&r); err != nil {
		if err == io.EOF {
			mm.err = errors.New("server disconnected")
			return nil
		}

		mm.err = fmt.Errorf("local command gob decode: %v", err)
		return nil
	}

	return r.Suggest
}

func (mm *Conn) Error() error {
	mm.lock.Lock()
	defer mm.lock.Unlock()

	return mm.err
}

// Attach creates a CLI interface to the dialed minimega instance
func (mm *Conn) Attach(namespace string) {
	fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
	fmt.Println("use 'disconnect' or ^d to exit just the minimega command line")
	fmt.Println()

	input := liner.NewLiner()
	defer input.Close()

	input.SetCtrlCAborts(true)
	input.SetTabCompletionStyle(liner.TabPrints)
	input.SetCompleter(mm.Suggest)

	prompt := fmt.Sprintf("minimega:%v$ ", mm.url)

	if namespace != "" {
		prompt = fmt.Sprintf("minimega:%v[%v]$ ", mm.url, namespace)
	}

	var quit bool
	for {
		line, err := input.Prompt(prompt)
		if err == liner.ErrPromptAborted {
			continue
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)

		log.Debug("got line from stdin: `%s`", line)

		// skip blank lines
		if line == "" {
			continue
		}

		input.AppendHistory(line)

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

		if namespace != "" {
			line = fmt.Sprintf("namespace %q %v", namespace, line)
		}

		mm.RunAndPrint(line, true)

		if err := mm.Error(); err != nil {
			log.Errorln(err)
			break
		}
	}
}
