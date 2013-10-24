package main

import (
	"io"
	log "minilog"
	"ssh"
	"ssh/terminal"
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

func sshClient() {
	log.Debugln("sshClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("ssh host %v from %v", h, o)
	}
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

	l, err := ssh.Listen("tcp", ":2022", config)
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
					break
				}
				sshReportChan <- uint64(len(line))
				// just echo the message
				serverTerm.Write([]byte(line))
				serverTerm.Write([]byte{'\r', '\n'})
			}
		}()
	}
}
