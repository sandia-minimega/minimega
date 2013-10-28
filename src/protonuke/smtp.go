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
	conn      net.Conn
	bufin     *bufio.Reader
	bufout    *bufio.Writer
	state     int
	response  string
	mail_from string
	rcpt_to   []string
	data      string
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
	myFQDN   = "protonuke.local"
)

func NewSMTPClientSession(c net.Conn) *SMTPClientSession {
	ret := &SMTPClientSession{conn: c, state: INIT}
	ret.bufin = bufio.NewReader(c)
	ret.bufout = bufio.NewWriter(c)
	ret.rcpt_to = []string{}
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
			s.addResponse("220 protonuke: Less for outcasts, more for weirdos.")
			s.state = COMMANDS
		case COMMANDS:
			input, err := s.readSmtp()
			input = strings.Trim(input, "\r\n")
			cmd := strings.ToUpper(input)
			if err != nil {
				log.Debugln(err)
				continue
			}
			switch {
			case strings.HasPrefix(cmd, "HELO"):
				s.addResponse("250 You're all so great and we're gonna keep you listening all day")
			case strings.HasPrefix(cmd, "EHLO"):
				r := "250-" + myFQDN + " Serving mail almost makes you wish for a nuclear winter\r\n"
				r += "250-8BITMIME\r\n250-STARTTLS\r\n250 HELP"
				s.addResponse(r)
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				if len(input) > 10 {
					s.mail_from = input[10:]
				}
				s.addResponse("250 Ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				if len(input) > 8 {
					s.rcpt_to = append(s.rcpt_to, input[8:])
				}
				s.addResponse("250 Ok")
			case strings.HasPrefix(cmd, "DATA"):
				s.addResponse("354 End data with <CR><LF>.<CR><LF>")
				s.state = DATA
			}
		case DATA:
			input, err := s.readSmtp()
			if err != nil {
				log.Debugln(err)
			}
			s.data = input
			s.state = COMMANDS
			log.Debugln("Got email message:")
			log.Debugln(s)
		}
		size, _ := s.bufout.WriteString(s.response)
		s.bufout.Flush()
		s.response = s.response[size:]
	}
}

func (s *SMTPClientSession) addResponse(r string) {
	s.response += r + "\r\n"
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
