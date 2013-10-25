package main

import (
	"bufio"
	"errors"
	log "minilog"
	"net"
	"strconv"
	"strings"
	"time"
)

func smtpClient() {
	log.Debugln("smtpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("smtp host %v from %v", h, o)
	}
}

type SMTPClientSession struct {
	conn  net.Conn
	bufin *bufio.Reader
	state int
}

const (
	INIT = iota
	COMMANDS
	STARTTLS
	DATA
)

var (
	timeout  = time.Duration(100)
	max_size = 131072
)

func NewSMTPClientSession(c net.Conn) *SMTPClientSession {
	ret := &SMTPClientSession{conn: c, state: INIT}
	ret.bufin = bufio.NewReader(c)
	return ret
}

func smtpServer() {
	log.Debugln("smtpServer")

	listener, err := net.Listen("tcp", "0.0.0.0:2005")
	if err != nil {
		log.Debugln(err)
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Debugln(err)
			continue
		}

		client := NewSMTPClientSession(conn)

		go client.Handler()
	}
}

func (s *SMTPClientSession) Close() {
	s.conn.Close()
}

func (s *SMTPClientSession) Handler() {
	defer s.Close()
	for {
		switch s.state {
		case INIT:

		}
	}
}

// Read a line from the client.
// Take directly from Flashmob's GoGuerrilla (MIT license, github.com/flashmob/go-guerrilla)
// Slight modification to be a method on SMTPClientSession
func (s *SMTPClientSession) readSmtp() (input string, err error) {
	var reply string
	// Command state terminator by default
	suffix := "\r\n"
	if s.state == DATA {
		suffix = "\r\n.\r\n"
	}
	for err == nil {
		s.conn.SetDeadline(time.Now().Add(timeout * time.Second))
		reply, err = s.bufin.ReadString('\n')
		if reply != "" {
			input = input + reply
			if len(input) > max_size {
				err = errors.New("Maximum DATA size exceeded (" + strconv.Itoa(max_size) + ")")
				return input, err
			}
		}
		if err != nil {
			break
		}
		if strings.HasSuffix(input, suffix) {
			break
		}
	}
	return input, err
}
