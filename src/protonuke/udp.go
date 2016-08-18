// Copyright (2016) Sandia Corporations
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"net"
	"time"
	log "minilog"
	"math/rand"
)

type udpConn struct {
	Conn		net.Conn
	Host		string

}

var (
	udpConns []*udpConn
	U_PORT	 = ":10001"
)

/* Simple Error Checking */
func CheckError(err error) {
	if err  != nil {
		// Prints the error to the console
		log.Errorln(err)
		return
	}
}

// udp client events include connecting, disconnecting, aand writing messages
// in open connections it will send a random slice of integers to simulate
// sending messages
func udpClient(p string) {
	log.Debugln("udpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
		// connect a client if no connections
		if len(udpConns) == 0 {
			h, o := randomHost()
			log.Debug("udp host %v from %v", h, o)
			udpClientConnect(h, p)
		} else {
			s := rand.NewSource(time.Now().UnixNano())
			r := rand.New(s)
			switch r.Intn(10) {
				case 0: // new connection
					h, o := randomHost()
					for _, v := range udpConns {
						if v.Host == h {
							log.Debugln("udp: already connected")
							continue
						}
					}
					log.Debugln("udp host %v from %v", h, o)
					udpClientConnect(h, p)
				case 1: // disconnect
					i := r.Intn(len(udpConns))
					log.Debug("udp disconnect on %v", udpConns[i].Host)
					udpClientDisconnect(i)
				default: // send a message
					i := r.Intn(len(udpConns))
					log.Debug("udp activity on %v", udpConns[i].Host)
					udpClientActivity(i)
			}
		}
	}
}

func udpClientConnect(host string, protocol string) {
	uc := &udpConn{}
	
	// Check for ipv6
	dHost := host
	if isIPv6(dHost) {
		dHost = "[" + dHost + "]"
	}
	
	// establish the server anc create a connection
	ServerAddr, err := net.ResolveUDPAddr("udp", dHost + U_PORT )
	CheckError(err)
	// setting laddr to nil autodefines the local addr
	uc.Conn, err = net.DialUDP("udp", nil, ServerAddr)
	CheckError(err)

	uc.Host = host
	udpConns = append(udpConns, uc)
}

func udpClientDisconnect(index int) {
	udpConns[index].Conn.Close()
	udpConns = append(udpConns[:index], udpConns[index+1:]...)
}

func udpClientActivity(index int) {

	uc := udpConns[index]

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	// Generate a random bit slice to transmit
	l := r.Intn(128)
	b := make([]byte, l)
	for i, _ := range b {
		b[i] = byte(r.Int())
	}

	log.Debug("udp activity to %v with %v", uc.Host, b)
	
	// Transmit the data
	start := time.Now().UnixNano()
	_,err := uc.Conn.Write(b)
	if err != nil {
            log.Debugln(err)
        }
	time.Sleep(time.Millisecond * 100)

	udpReportChan <-uint64(len(b))

	stop := time.Now().UnixNano()
	log.Info("udp %v %vns", uc.Host, uint64(stop-start))
}

func udpServer(p string) {
	log.Debugln("udpServer")

	// Set the server address to the given port on local host
	ServerAddr,err := net.ResolveUDPAddr("udp", U_PORT)
	CheckError(err)

	// Listen on the selected port
	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	CheckError(err)

	buf := make([]byte, 1024)

	for {
		// Read and print the message
		n,_,err := ServerConn.ReadFromUDP(buf)
		

		if err != nil {
			log.Debugln(err)
		}

		udpReportChan <- uint64(n)
	}
}
