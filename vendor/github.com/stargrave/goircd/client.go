/*
goircd -- minimalistic simple Internet Relay Chat (IRC) server
Copyright (C) 2014-2016 Sergey Matveev <stargrave@stargrave.org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package goircd

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	BufSize   = 1500
	MaxOutBuf = 1 << 12
)

var (
	CRLF []byte = []byte{'\x0d', '\x0a'}
)

type Client struct {
	conn          net.Conn
	registered    bool
	nickname      *string
	username      *string
	realname      *string
	password      *string
	away          *string
	recvTimestamp time.Time
	sendTimestamp time.Time
	outBuf        chan *string
	alive         bool
	sync.Mutex
}

func (c *Client) Host() string {
	addr := c.conn.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	if domains, err := net.LookupAddr(addr); err == nil {
		addr = strings.TrimSuffix(domains[0], ".")
	}
	return addr
}

func (c *Client) String() string {
	return *c.nickname + "!" + *c.username + "@" + c.Host()
}

func NewClient(conn net.Conn) *Client {
	nickname := "*"
	username := ""
	c := Client{
		conn:          conn,
		nickname:      &nickname,
		username:      &username,
		recvTimestamp: time.Now(),
		sendTimestamp: time.Now(),
		alive:         true,
		outBuf:        make(chan *string, MaxOutBuf),
	}
	go c.MsgSender()
	return &c
}

func (c *Client) SetDead() {
	c.outBuf <- nil
	c.alive = false
}

func (c *Client) Close() {
	c.Lock()
	if c.alive {
		c.SetDead()
	}
	c.Unlock()
}

// Client processor blockingly reads everything remote client sends,
// splits messages by CRLF and send them to Daemon gorouting for processing
// it futher. Also it can signalize that client is unavailable (disconnected).
func (c *Client) Processor(sink chan ClientEvent) {
	sink <- ClientEvent{c, EventNew, ""}
	log.Debugln(c, "New client")
	buf := make([]byte, BufSize*2)
	var n int
	var prev int
	var i int
	var err error
	for {
		if prev == BufSize {
			log.Debugln(c, "input buffer size exceeded, kicking him")
			break
		}
		n, err = c.conn.Read(buf[prev:])
		if err != nil {
			break
		}
		prev += n
	CheckMore:
		i = bytes.Index(buf[:prev], CRLF)
		if i == -1 {
			continue
		}
		sink <- ClientEvent{c, EventMsg, string(buf[:i])}
		copy(buf, buf[i+2:prev])
		prev -= (i + 2)
		goto CheckMore
	}
	c.Close()
	sink <- ClientEvent{c, EventDel, ""}
}

func (c *Client) MsgSender() {
	for msg := range c.outBuf {
		if msg == nil {
			c.conn.Close()
			break
		}
		c.conn.Write(append([]byte(*msg), CRLF...))
	}
}

// Send message as is with CRLF appended.
func (c *Client) Msg(text string) {
	c.Lock()
	defer c.Unlock()
	if !c.alive {
		return
	}
	if len(c.outBuf) == MaxOutBuf {
		log.Debugln(c, "output buffer size exceeded, kicking him")
		if c.alive {
			c.SetDead()
		}
		return
	}
	c.outBuf <- &text
}

// Send message from server. It has ": servername" prefix.
func (c *Client) Reply(text string) {
	c.Msg(":" + *hostname + " " + text)
}

// Send server message, concatenating all provided text parts and
// prefix the last one with ":".
func (c *Client) ReplyParts(code string, text ...string) {
	parts := []string{code}
	for _, t := range text {
		parts = append(parts, t)
	}
	parts[len(parts)-1] = ":" + parts[len(parts)-1]
	c.Reply(strings.Join(parts, " "))
}

// Send nicknamed server message. After servername it always has target
// client's nickname. The last part is prefixed with ":".
func (c *Client) ReplyNicknamed(code string, text ...string) {
	c.ReplyParts(code, append([]string{*c.nickname}, text...)...)
}

// Reply "461 not enough parameters" error for given command.
func (c *Client) ReplyNotEnoughParameters(command string) {
	c.ReplyNicknamed("461", command, "Not enough parameters")
}

// Reply "403 no such channel" error for specified channel.
func (c *Client) ReplyNoChannel(channel string) {
	c.ReplyNicknamed("403", channel, "No such channel")
}

func (c *Client) ReplyNoNickChan(channel string) {
	c.ReplyNicknamed("401", channel, "No such nick/channel")
}
