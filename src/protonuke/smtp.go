/**
Based largely on Go-Guerrilla SMTPd, whose license and copyright follows:
Copyright (c) 2012 Flashmob, GuerrillaMail.com

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
documentation files (the "Software"), to deal in the Software without restriction, including without limitation the
rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the
Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE
WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
**/

package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"math/rand"
	log "minilog"
	"net"
	"net/smtp"
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

		s := rand.NewSource(time.Now().UnixNano())
		r := rand.New(s)
		body := email[r.Intn(len(email))]

		toLen := r.Intn(10) + 1
		fromLen := r.Intn(10) + 1
		var to string
		var from string
		for i := 0; i < toLen; i++ {
			to += string(alphanum[r.Intn(len(alphanum))])
		}
		for i := 0; i < fromLen; i++ {
			from += string(alphanum[r.Intn(len(alphanum))])
		}

		to += "@" + h

		err := smtpSendMail(h, to, from, body)
		if err != nil {
			log.Errorln(err)
		}
	}
}

func smtpSendMail(server, to, rcpt, body string) error {
	c, err := smtp.Dial(server + smtpPort)
	if err != nil {
		return err
	}

	err = c.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		log.Warnln("could not start tls")
	}

	c.Mail(to)
	c.Rcpt(rcpt)
	wc, err := c.Data()
	if err != nil {
		return err
	}
	wc.Write([]byte(body))
	wc.Close()

	return nil
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
	tls_on    bool
}

const (
	INIT = iota
	COMMANDS
	STARTTLS
	DATA
	QUIT
)

const (
	alphanum = "01234567890abcdefghijklmnopqrstuvwxyz"
)

var (
	timeout   = time.Duration(100)
	max_size  = 131072
	myFQDN    = "protonuke.local"
	TLSconfig *tls.Config
	smtpPort  = ":2005"
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

	certfile, keyfile := generateCerts()
	cert, err := tls.LoadX509KeyPair(certfile, keyfile)
	if err != nil {
		log.Fatalln("couldn't get cert: ", err)
	}
	TLSconfig = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.VerifyClientCertIfGiven, ServerName: myFQDN}
	listener, err := net.Listen("tcp", "0.0.0.0"+smtpPort)
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
				return
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
			case strings.HasPrefix(cmd, "STARTTLS") && !s.tls_on:
				s.addResponse("220 They asked what do you know about theoretical physics? I said I had a theoretical degree in physics.")
				s.state = STARTTLS
			case strings.HasPrefix(cmd, "QUIT"):
				s.addResponse("221 Beware the Battle Cattle!")
				s.state = QUIT
			case strings.HasPrefix(cmd, "NOOP"):
				s.addResponse("250 Is it time?")
			case strings.HasPrefix(cmd, "RSET"):
				s.mail_from = ""
				s.rcpt_to = []string{}
				s.addResponse("250 I forgot to remember to forget")
			default:
				s.addResponse("500 unrecognized command")
			}
		case DATA:
			input, err := s.readSmtp()
			if err != nil {
				log.Debugln(err)
			}
			s.data = input
			log.Debugln("Got email message:")
			log.Debugln(s)
			s.addResponse("250 Ok: Now that is a delivery service you can count on")
			s.state = COMMANDS
		case STARTTLS:
			// I'm just going to pull this from GoGuerrilla, thanks guys
			var tlsConn *tls.Conn
			tlsConn = tls.Server(s.conn, TLSconfig)
			err := tlsConn.Handshake() // not necessary to call here, but might as well
			if err == nil {
				s.conn = net.Conn(tlsConn)
				s.bufin = bufio.NewReader(s.conn)
				s.bufout = bufio.NewWriter(s.conn)
				s.tls_on = true
			} else {
				log.Debugln("Could not TLS handshake:", err)
			}
			s.state = COMMANDS
		case QUIT:
			return
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
