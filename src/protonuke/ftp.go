// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bytes"
	"crypto/tls"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	log "minilog"
	"net"
	"strconv"
	"time"

	"github.com/dutchcoders/goftp"
	"github.com/goftp/server"
)

const (
	COMMAND_PORT = 21
	FILE_PATH    = ""
	USER         = "anonymous"
	PASS         = "anonymous"
	SERVER_NAME  = "protoFTP"
)

var (
	goftpServer *server.Server
	useTLS      = false
	tlsAuth     = false
	connected   = false
	auth        = false
	FTPImage    []byte
)

type FTPAuth struct{}

func (a FTPAuth) CheckPasswd(user, pass string) (bool, error) {
	return true, nil
}

func ftpClient() {
	var ftp *goftp.FTP
	var err error

	rand.Seed(time.Now().UnixNano())
	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)

	for {
		t.Tick()

		// Connect to a host
		if !connected {
			h, o := randomHost()
			log.Debug("ftp host %v from %v", h, o)
			host := h + ":" + strconv.Itoa(COMMAND_PORT)

			if ftp, err = goftp.Connect(host); err != nil {
				log.Errorln(err)
			} else {
				ftp.HitCallback(updateHitCount)
				connected = true
				log.Debug("Connected to host")
			}
			continue
		}

		// Authenticate
		if !auth {
			if err = ftp.Login(USER, PASS); err != nil {
				log.Errorln(err)
			} else {
				auth = true
				log.Debug("Logged in as %v", USER)
			}
			continue
		}

		// TLS Auth
		if useTLS && !tlsAuth {
			var version uint16
			if *f_tlsVersion != "" {
				switch *f_tlsVersion {
				case "tls1.0":
					version = tls.VersionTLS10
				case "tls1.1":
					version = tls.VersionTLS11
				case "tls1.2":
					version = tls.VersionTLS12
				}
			}

			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30,
				MaxVersion:         version,
			}

			if err = ftp.AuthTLS(tlsConfig); err != nil {
				log.Errorln(err)
				ftpQuit(ftp)
			} else {
				tlsAuth = true
			}
			log.Debug("TLS auth ok")
			continue
		}

		// Random ftp actions
		if connected && auth {
			switch rand.Intn(6) {
			case 0:
				// get current path
				var curpath string
				if curpath, err = ftp.Pwd(); err != nil {
					log.Errorln(err)
				}
				log.Debug("Current path: %v", curpath)
			case 1:
				// get system type of remote host
				var syst string
				if syst, err = ftp.Syst(); err != nil {
					log.Errorln(err)
				}
				log.Debug("System: %v", syst)
			case 2:
				// Get the filesize of the protonuke binary
				var size int
				if size, err = ftp.Size("/tmp/ftpimage"); err != nil {
					log.Errorln(err)
				}
				log.Debug("ftpimage file size: %v", size)
			case 3:
				// get directory listing
				var files []string
				if files, err = ftp.List("/tmp"); err != nil {
					log.Errorln(err)
				}
				log.Debug("Directory listing: %v", files)
			case 4:
				// request file transfer
				var s string
				var retrfunc func(io.Reader) error
				retrfunc = func(r io.Reader) error {
					_, err := io.Copy(ioutil.Discard, r)
					return err
				}
				if s, err = ftp.Retr("/tmp/ftpimage", retrfunc); err != nil {
					log.Errorln(err)
				}
				log.Debug("Retr: %v", s)
			case 5:
				// quit
				ftpQuit(ftp)
				log.Debug("Logged out")
			}
		}
	}
}

func ftpsClient() {
	useTLS = true
	ftpClient()
}

func ftpQuit(ftp *goftp.FTP) {
	// close connection
	if err := ftp.Quit(); err != nil {
		log.Errorln(err)
	}
	connected = false
	auth = false
	tlsAuth = false
}

func updateHitCount() {
	if useTLS {
		ftpTLSReportChan <- 1
	} else {
		ftpReportChan <- 1
	}
}

func ftpServer() {
	ftpMakeImage()

	var factory server.DriverFactory
	perm := server.NewSimplePerm(USER, PASS)
	factory = &FileDriverFactory{
		FILE_PATH,
		perm,
	}

	auth := FTPAuth{}

	// Get our ip address for PASV connection
	var ipv4 net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Errorln(err)
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			log.Errorln(err)
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && !ipnet.IP.IsLoopback() {
				if ip := ipnet.IP.To4(); ip != nil {
					ipv4 = ip
				}
			}
		}
	}

	if ipv4 == nil {
		log.Fatal("Unable to determine local IP for PASV connection.")
	}

	cert, key := generateCerts()

	opt := &server.ServerOpts{
		Factory:      factory,
		Auth:         auth,
		Name:         SERVER_NAME,
		PublicIp:     ipv4.String(),
		PassivePorts: "",
		Port:         COMMAND_PORT,
		TLS:          useTLS,
		CertFile:     cert,
		KeyFile:      key,
		ExplicitFTPS: useTLS,
		HitFunc:      updateHitCount,
	}
	goftpServer = server.NewServer(opt)

	go func() {
		err := goftpServer.ListenAndServe()
		if err != nil {
			log.Error("Error starting server: %v", err)
		}
	}()
}

func ftpsServer() {
	useTLS = true
	ftpServer()
}

func ftpMakeImage() {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	pixelcount := f_ftpFileSize / 4
	side := int(math.Sqrt(float64(pixelcount)))
	log.Debug("Image served will be %v by %v", side, side)

	m := image.NewRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(m.Pix); i++ {
		m.Pix[i] = uint8(r.Int())
	}

	buf := new(bytes.Buffer)
	png.Encode(buf, m)
	FTPImage = buf.Bytes()
}
