package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	log "minilog"
	"ssh"
	"ssh/terminal"
	"time"
)

var id_rsa = `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAu+Yi5/0cK8PrCfoWlabrxgeUP7BRECb/bEh6c70X4HZ/CojX
JVCigyvrzlXc0oR42+lSxpRfMdcuAfK2hTCm7U6eLRa0BuniwirtJnomQU6raauN
Zil3a3SK8R+Eh/N5BUytMq1khTneo1lNbjzxvNTG/79wBcDUdhYqraCT1xnxFDLN
d9Vdn1H8cA8qGYKqOXyCu2j2OLTM5A8g68wRBcnn/HAszNVNTuJDYz932IBkiQc6
M5NtgBkTsG/Wlc9okEZ/BfsoO1UANhGdDTQVUzjvw5zqA4isWzwXJykuW5KFSxN2
+JVC2Dx5kuhnkSdtlI7/2hNm8TiSrLRXoQyQAwIDAQABAoIBAGQeM+s4ypHnSo1+
XBpRXr+dujhVUlww61hfJiAVWWuYuAh88WZImM8b0AGZbpgdTeifYiO9WxvLdBBF
q0s8qTU49r8/oZ0tck3TYZlm7ItPx7X+WtFctuzipEXmbU+jQ5C3UnH2QeFa/G49
Xyyl/IiJN599+cqW/J4PIZ5yOVQa7Cwbhma6v13oxAOIAZ8GFTQAYSwtLby718zm
g68NfslpADDcb7kyG8hsjEnOiycEtOxvDaYQ9+u94aBne3Y5J0CY+cMlhHzFXon1
rLrPVq+qc8HAjlfLqtzwzzf8d8VHwV6Krgtg6oZg/Bgj50qhX6/NCXvdShRzfd/f
yMm5+IECgYEA67vd27qCVabkTkjhfP5gmQ3SkGMaor276fZSODAqPiksC/TRFUqf
/oNX9yaukHnW0UwGewbxsLouual6MM6b665yGMzq2AhSu1Ol33Y7DBrBH4md9iOD
dOZvzr0ejgjOwOtzzWRbnihjvJG+VDUyU6ngA+Lac7p7WbQ8JxpCCQkCgYEAzA1/
rjgCHEIH+E/K0ON6XEinYwidbb2kz2SwaajqJyXAr1N6fbD/BYZVL77cpB41ey8h
/DCSRNyC20z6QKBmlsq4dyRdu1U+8iL2OAs+HOAEmltGy9DQ0vy3yLsgCOFmpLdF
9Jq+JTDJ4dPURvsBwJYx/sYzkclCsGmeimxhD6sCgYBbMZU1MKTFD8gYhNc+bIXg
D9naY4xlUrMEYncSJ4ff/jt88JuF+hWE9zircvprB8dTtm53X4tWS+BRkL+la/gj
p5uZ/oQHSMkAkO6FUQ6ssxjs42cJVlm/enncZ4sPdVbOiQeGeIF84LEcvOD9YIr0
lK4FstfBl22qmTAADIdpSQKBgGwkl0+Q9WVehXTPbRDKDnZMNxIgZbbcdDVKCsjk
sbwvoPAKkPd+T5nw+MLGJ49/Rx7S+vL6FvsR1vQ81sBbgiNWqu7Rwi9fXW3co5tO
MgwBmc7ooxuvvoyjTQ/ARJkQRGL1ksixHib9tXDO4EkCDIqxzytUhc402Pg/8bsw
9zvjAoGASKRVGhZu9ccGLiCmJdv5Vmzryap27I7IqdQc7b9VZBwaND0mMpQn1iiq
Oh65gWn5H1NDyAVnzcu1j3QDVGnh7LxxY73e2lRNBK4XzYYLx0F7TQxr7UM1xesb
Ju2wJ6nHR3zeAxwqnmhl3VPKTHjIQu0GkQ83Z9NcnYZsLSYnuz8=
-----END RSA PRIVATE KEY-----
`

type sshConn struct {
	Config    *ssh.ClientConfig
	Client    *ssh.ClientConn
	Session   *ssh.Session
	Host      string
	Stdin     io.Writer
	StdoutBuf bytes.Buffer
}

type sshPassword string

func (p sshPassword) Password(user string) (string, error) {
	return string(p), nil
}

var (
	sshConns []*sshConn
	PORT     = ":22"
)

// ssh client events include connecting, disconnecting, or typing in an
// existing session.  it's not very representative to constantly connect and
// disconnect, so the event pump will randomly choose to issue the listed
// events with weights. 10% chance to connect/disconnect (100% to connect if no
// connections exist), and 90% chance to issue commands on existing
// connections.
func sshClient() {
	log.Debugln("sshClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()

		// special case - if we have no connections, make a connection
		if len(sshConns) == 0 {
			h, o := randomHost()
			log.Debug("ssh host %v from %v", h, o)
			sshClientConnect(h)
		} else {
			s := rand.NewSource(time.Now().UnixNano())
			r := rand.New(s)
			switch r.Intn(10) {
			case 0: // new connection
				h, o := randomHost()
				// make sure we're not already connected
				for _, v := range sshConns {
					if v.Host == h {
						log.Debugln("ssh: already connected")
						continue
					}
				}
				log.Debug("ssh host %v from %v", h, o)
				sshClientConnect(h)
			case 1: // disconnect
				i := r.Intn(len(sshConns))
				log.Debug("ssh disconnect on %v", sshConns[i].Host)
				sshClientDisconnect(i)
			default: // event on one of the existing connections
				i := r.Intn(len(sshConns))
				log.Debug("ssh activity on %v", sshConns[i].Host)
				sshClientActivity(i)
			}
		}
	}
}

func sshClientConnect(host string) {
	sc := &sshConn{}
	sc.Config = &ssh.ClientConfig{
		User: "protonuke",
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthPassword(sshPassword("password")),
		},
	}

	var err error
	sc.Client, err = ssh.Dial("tcp", host+PORT, sc.Config)
	if err != nil {
		log.Errorln(err)
		return
	}

	sc.Session, err = sc.Client.NewSession()
	if err != nil {
		log.Errorln(err)
		return
	}

	sc.Session.Stdout = &sc.StdoutBuf
	sc.Stdin, err = sc.Session.StdinPipe()
	if err != nil {
		log.Errorln(err)
		return
	}

	if err := sc.Session.Shell(); err != nil {
		log.Errorln(err)
		return
	}

	sc.Host = host

	sshConns = append(sshConns, sc)
}

func sshClientDisconnect(index int) {
	sshConns[index].Session.Close()
	sshConns = append(sshConns[:index], sshConns[index+1:]...)
}

func sshClientActivity(index int) {
	sc := sshConns[index]

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	// generate a random byte slice
	l := r.Intn(128)
	b := make([]byte, l)
	for i, _ := range b {
		b[i] = byte(r.Int())
	}

	data := base64.StdEncoding.EncodeToString(b)
	log.Debug("ssh activity to %v with %v", sc.Host, data)

	sc.Stdin.Write([]byte(data))
	sc.Stdin.Write([]byte{'\r', '\n'})
	sshReportChan <- uint64(len(data))

	expected := fmt.Sprintf("> %v\r\n%v\r\n> ", data, data)
	for i := 0; i < 10 && sc.StdoutBuf.String() != expected; i++ {
		time.Sleep(100 * time.Millisecond)
	}

	log.Debugln("ssh: ", sc.StdoutBuf.String())

	sc.StdoutBuf.Reset()
}

func sshServer() {
	log.Debugln("sshServer")

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn *ssh.ServerConn, user, pass string) bool {
			return user == "protonuke" && pass == "password"
		},
	}

	private, err := ssh.ParsePrivateKey([]byte(id_rsa))
	if err != nil {
		log.Fatalln(err)
	}

	config.AddHostKey(private)

	l, err := ssh.Listen("tcp", PORT, config)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorln(err)
			continue
		}

		if err := conn.Handshake(); err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			continue
		}

		go sshHandleConn(conn)
	}
}

func sshHandleConn(conn *ssh.ServerConn) {
	for {
		channel, err := conn.Accept()
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			return
		}

		if channel.ChannelType() != "session" {
			channel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel.Accept()

		term := terminal.NewTerminal(channel, "> ")
		serverTerm := &ssh.ServerTerminal{
			Term:    term,
			Channel: channel,
		}
		go func() {
			defer channel.Close()
			for {
				line, err := serverTerm.ReadLine()
				if err != nil {
					if err != io.EOF {
						log.Errorln(err)
					}
					return
				}
				sshReportChan <- uint64(len(line))
				// just echo the message
				log.Debugln("ssh received: ", line)
				serverTerm.Write([]byte(line))
				serverTerm.Write([]byte{'\r', '\n'})
			}
		}()
	}
}
