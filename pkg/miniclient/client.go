// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
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
	"sync/atomic"
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

	// TTY is set if the client prints to a terminal, enabling column width
	// limits on rendered output.
	TTY bool `json:",omitempty"`
}

type Response struct {
	// Resp, Rendered, More are returned in response to a command
	Resp     minicli.Responses
	Rendered string
	More     bool // whether there are more responses coming

	// Status is set if responding with status update for command (Rendered will
	// contain the status message).
	Status bool

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

	// TTY is sent with commands, see Request.TTY
	TTY bool

	// finish ends the session that Pipe started, if any
	finish func(error)
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

// Close closes the connection. It also ends a session that Pipe started: its
// reader and writer then return net.ErrClosed.
func (mm *Conn) Close() error {
	mm.lock.Lock()
	finish := mm.finish
	mm.lock.Unlock()

	if finish != nil {
		finish(net.ErrClosed)
		return nil
	}

	return mm.conn.Close()
}

// Pipe attaches the connection to a named pipe. Lines written to the returned
// writer are written to the pipe, and lines that others write to the pipe are
// read from the returned reader: minimega doesn't send the connection's own
// lines back. Pipe returns nil, nil if the request cannot be sent; see Error.
//
// Closing the writer tells minimega that there is nothing more to write.
// minimega then writes the lines it has received to the pipe and closes the
// connection, which ends the reader with io.EOF. The caller must therefore
// read until the reader returns an error, also to learn that everything it
// wrote has reached the pipe. If the connection fails, or minimega closes it
// first, the reader and the writer return the error, as does Error.
//
// The connection cannot be used for anything else afterwards.
func (mm *Conn) Pipe(pipe string) (io.Reader, io.WriteCloser) {
	mm.lock.Lock()
	err := mm.enc.Encode(Request{
		PlumbPipe: pipe,
	})
	if err != nil {
		mm.err = fmt.Errorf("local pipe gob encode: %v", err)
	}
	mm.lock.Unlock()

	if err != nil {
		return nil, nil
	}

	// rr and rw carry lines from minimega to the caller, wr and ww carry
	// lines from the caller to minimega
	rr, rw := io.Pipe()
	wr, ww := io.Pipe()

	// set once the caller has closed the writer and we told minimega so
	var closed atomic.Bool

	// finish ends the session, once: it records err, if any, closes the
	// connection, and makes the reader and the writer return err (the reader
	// returns io.EOF if err is nil).
	var once sync.Once
	finish := func(err error) {
		once.Do(func() {
			if err != nil {
				mm.lock.Lock()
				mm.err = err
				mm.lock.Unlock()
			}

			mm.conn.Close()

			// the writer first: once the caller has seen the reader end, a
			// write must fail and not lose the line
			wr.CloseWithError(err)
			rw.CloseWithError(err)
		})
	}

	mm.lock.Lock()
	mm.finish = finish
	mm.lock.Unlock()

	go func() {
		for {
			var buf string

			err := mm.dec.Decode(&buf)
			if err == io.EOF && closed.Load() {
				// minimega has written everything we sent to the pipe
				finish(nil)
				return
			} else if err == io.EOF {
				finish(errors.New("server disconnected"))
				return
			} else if err != nil {
				finish(fmt.Errorf("local command gob decode: %v", err))
				return
			}

			if _, err := io.WriteString(rw, buf); err != nil {
				finish(fmt.Errorf("write: %v", err))
				return
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(wr)
		buf := make([]byte, 0, TOKEN_MAX)
		scanner.Buffer(buf, TOKEN_MAX)
		for scanner.Scan() {
			err := mm.enc.Encode(scanner.Text() + "\n")
			if err != nil {
				finish(fmt.Errorf("local command gob encode: %v", err))
				return
			}
		}

		// scanners don't return EOF errors
		if err := scanner.Err(); err != nil {
			finish(fmt.Errorf("read: %v", err))
			return
		}

		log.Debugln("client closed stdin")

		// Don't close the connection: minimega may not have read everything
		// we sent yet, and may still have lines for us. Close our side only,
		// and let the reader above finish when minimega closes the connection.
		closed.Store(true)

		c, ok := mm.conn.(interface{ CloseWrite() error })
		if !ok {
			// cannot close one side only
			finish(nil)
		} else if err := c.CloseWrite(); err != nil {
			finish(fmt.Errorf("local command close: %v", err))
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

	err := mm.enc.Encode(Request{Command: cmd, TTY: mm.TTY})
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

			// If this was a status update response then continue reading from the
			// socket no matter what.
			if r.Status {
				continue
			}

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
