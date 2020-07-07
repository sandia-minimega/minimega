package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/minipager"
)

type Request struct {
	Command string
}

type Response struct {
	// Resp, Rendered, More are returned in response to a command
	Resp     minicli.Responses
	Rendered string
	More     bool // whether there are more responses coming
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

func Dial(path string) (*Conn, error) {
	c := &Conn{
		url: path,
	}

	// try to connect to the local minimega
	conn, err := net.Dial("unix", c.url)
	if err != nil {
		return nil, err
	}

	c.conn = conn
	c.enc = json.NewEncoder(c.conn)
	c.dec = json.NewDecoder(c.conn)

	return c, nil
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

// Run a command through a JSON pipe, hand back channel for responses.
func (c *Conn) Run(cmd string) chan *Response {
	out := make(chan *Response)

	if cmd == "" {
		// Language spec: "Receiving from a nil channel blocks forever."
		// Instead, make and immediately close the channel so that range
		// doesn't block and receives no values.
		close(out)

		return out
	}

	c.lock.Lock()

	err := c.enc.Encode(Request{Command: cmd})
	if err != nil {
		c.err = fmt.Errorf("local command gob encode: %v", err)

		// see above
		close(out)
		return out
	}
	log.Debugln("encoded command:", cmd)

	go func() {
		defer c.lock.Unlock()
		defer close(out)

		for {
			var r Response
			if err := c.dec.Decode(&r); err != nil {
				if err == io.EOF {
					c.err = errors.New("server disconnected")
					return
				}

				c.err = fmt.Errorf("local command gob decode: %v", err)
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
func (c *Conn) RunAndPrint(cmd string, page bool) {
	for resp := range c.Run(cmd) {
		if page && c.Pager != nil {
			c.Pager.Page(resp.Rendered)
		} else if resp.Rendered != "" {
			fmt.Println(resp.Rendered)
		}

		if err := resp.Resp.Error(); err != "" {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

func (c *Conn) Error() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.err
}
