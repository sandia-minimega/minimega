// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

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
        log"minilog"
	"net"
	"strconv"
	"time"

	"github.com/goftp/server"
	"github.com/dutchcoders/goftp"
)

const (
	FILE_PATH	= ""
	USER		= "anonymous"
	PASS		= "anonymous"
	SERVER_NAME	= "protoFTP"
	CERT		= "-----BEGIN CERTIFICATE-----\nMIIFUDCCAzigAwIBAgIJAPytkJuQUEc7MA0GCSqGSIb3DQEBDQUAMD0xCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJDQTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMB4XDTE3MDExMTE4MzYxN1oXDTI3MDEwOTE4MzYxN1owPTELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNBMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDCzdI7i9t4F8PlMShxVfiC9AERKJdWqfNs4t+xZheQH+eleniNO7F5se2IpBVStnj5sMRwsjBHUZs16UK9zKtvNf/b9QGFY0u5ZRPZOZEnTrsQIrFPL7r0og33g2ZZgFh8hmx0uxKGmCetVAsloyy9yaHX2L/5h7GgXPrldbLFbNrok3e0uQtdjbYF5nPhGP68tcXRNzPZGGmZ2qNUvrE4jweBKKxfL9KuDVPW4QkKu0UFHGl8txPBo9KjyF2D3tyNRFetRvEtNlKx+6w4hpPmDrxkyXUrcuYCTztVHbNhGUJfeEDvpA0Xrl62MhJWJ0wWWQ0DPxxfR6B7GkkSPlLbelUCALTskjFd+ZsIfAslEgcDB22O+MwWcUTJvx/1OAswtXYJZ1HJ6xyZRBMf175USZjoHPhWd/g+BZRPpaAb6dqh/UYLrkI+OuUCD3WoUYt14g8y0geFQx8HvTZ+Hlv6S4dOIcFFD1Ipvnv8VkCQXf54e3HvqcixsqdxYR5v3hrTCOVu32IPysfTE+MIAtjvSJpTc/5vkOcVqmhEw+g/tGITTCqyWV2sIgJsfEvaCtpW/OjbAwrKFzngC2xeUG1wT737K5afT7kiENF9yz4cbztbrAxW/wa9sSqhHvqmWrotGC0ozvIVDtecSLmmuWCtu451ZM1iyW/7DUJe2FTSgQIDAQABo1MwUTAdBgNVHQ4EFgQUaiJnMwXWfzG8dQrI738tIIhECBQwHwYDVR0jBBgwFoAUaiJnMwXWfzG8dQrI738tIIhECBQwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQ0FAAOCAgEAr/n81IPWeyD9X0UcheAnVPJI5oMyUA0lPF9nfVcyjuy0tI8x0GYM91B+5/CTHiy//724ZgK/cLN63O1H+cm6C9x1VlDuuIxnt+ht9+Saf1B0CO65zZKzi4k9uuWkmE50Fsa7cyoL3XyhZhnj1LtCWBW1s3Z5MPPH3ouYv/NSRfOC8bDoPGMGBzL4EMhdSwoeeEWYYrVB1IPVsQsrn/Pg9fpCOLp0O6A3gay4O0BiUAKCg4K7rvoD1osg/WOiUN/cNw+RIoQ49BjZ6QIBHEgPFc+6vygM//C8zh9u45QuARV+V/G7CUX+ouIEbZ/JqQOP2H88z3rIJlx0LRdae8Xx851eRM7Z1Biaiwv5DQWKPW4Gs1N0YkRPXomxq9Im6xt2BQPNrqe4PsbgUqZ5tYFPP8iHyHza8RAifLZ0D6FHwNqyZLIF92M4hcZoPA7zO+BXuyQ0IK4RoAdXAW4EY9QWr6msT0QCKcEVsuQzXNY0h5mDPVLGghcDhz+sfkR3WHsFqo3oyfJjD7hX9nMOOq6khf2PHZbo3+uCfTdpU7LzPH4kAFXD6fotG1pV8/y2tpkpXVMdZYoa9uuL9FBHgMVv5K8nH4ZFvgCFQAsly3S+ka1GO9ZY24S39HHxGfgzIAeUBS42bMKhzazWgfkMqf/KwsoeFdJHxVnS3LxAJ5HWCso=\n-----END CERTIFICATE-----"
	KEY		= "-----BEGIN PRIVATE KEY-----\n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\n-----END PRIVATE KEY-----"
)

var (
	useTLS	= false
	tlsAuth = false
	connected = false
	auth = false
)

type FTPAuth struct {}

func (a FTPAuth) CheckPasswd(user,pass string) (bool, error) {
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
			host := h + ":" + strconv.Itoa(*f_ftpport)

			if ftp, err = goftp.Connect(host); err != nil {
				log.Errorln(err)
			} else {
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
				log.Debug("Logged in as", USER)
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
			InsecureSkipVerify:	true,
			MinVersion:		tls.VersionSSL30,
			MaxVersion:		version,
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
				log.Debug("Current path:", curpath)
			case 1:
				// get system type of remote host
				var syst string
				if syst, err = ftp.Syst(); err != nil {
					log.Errorln(err)
				}
				log.Debug("System:", syst)
			case 2:
				// Get the filesize of the protonuke binary
				var size int
				if size, err = ftp.Size("/tmp/ftpimage"); err != nil {
					log.Errorln("Size error:",err)
				}
				log.Debug("ftpimage file size:", size)
			case 3:
				// get directory listing
				var files []string
				if files, err = ftp.List("/tmp"); err != nil {
					log.Errorln(err)
				}
				log.Debug("Directory listing:", files)
			case 4:
				// request file transfer
				var s string
				var retrfunc func(io.Reader) error
				retrfunc = func(r io.Reader) error {
					_,err := io.Copy(ioutil.Discard, r)
					return err
				}
				if s, err = ftp.Retr("/tmp/ftpimage", retrfunc); err != nil {
					log.Errorln("Retr err:", err)
				}
				log.Debug("Retr:", s)
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


func ftpServer() {
	ftpMakeImage()
	ftpMakeTLSPem()

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

	opt := &server.ServerOpts{
		Factory:	factory,
		Auth:		auth,
		Name:		SERVER_NAME,
		PublicIp:	ipv4.String(),
		PassivePorts:	*f_ftppassiveports,
		Port:		*f_ftpport,
		TLS:		useTLS,
		CertFile:	"/tmp/ftpcert.pem",
		KeyFile:	"/tmp/ftpkey.pem",
		ExplicitFTPS:   useTLS,
	}
	ftpServer := server.NewServer(opt)

	go func() {
		err := ftpServer.ListenAndServe()
		if err != nil {
			log.Error("Error starting server: ", err)
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
	ftpImage := buf.Bytes()
	err := ioutil.WriteFile("/tmp/ftpimage", ftpImage, 0644)
	if err != nil {
		log.Errorln(err)
	}
}

func ftpMakeTLSPem() {
	err := ioutil.WriteFile("/tmp/ftpcert.pem", []byte(CERT), 0644)
	if err != nil {
		log.Errorln(err)
	}
	err = ioutil.WriteFile("/tmp/ftpkey.pem", []byte(KEY), 0644)
	if err != nil {
		log.Errorln(err)
	}
}

