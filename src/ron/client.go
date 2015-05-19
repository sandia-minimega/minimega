// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"encoding/gob"
	"fmt"
	log "minilog"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const RETRY_TIMEOUT = 10

// dial over tcp to a ron server
func (c *Client) dial(parent string, port int) {
	log.Debug("ron dial: %v:%v", parent, port)

	go c.mux()
	go c.periodic()
	go c.commandHandler()

	go func() {
		retry := time.Duration(RETRY_TIMEOUT * time.Second)
		c.hold.Lock()
		for {
			conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", parent, port))
			if err != nil {
				log.Errorln(err)
			} else {
				c.conn = conn
				c.out = make(chan *Message, 1024) // remake the out channel to flush outstanding messages
				c.hold.Unlock()
				c.handler()
				c.hold.Lock()
				log.Error("client disconnected, retrying connection in %v", retry)
			}

			time.Sleep(retry)
		}
	}()
}

// Respond allows a client to post a *Response to a given command. The response
// will be queued until the next heartbeat.
func (c *Client) Respond(r *Response) {
	log.Debug("ron Respond: %v", r.ID)

	c.responseLock.Lock()
	c.Responses = append(c.Responses, r)
	c.responseLock.Unlock()
}

// commandHandler sorts and filters incoming commands from a ron server.
// Commands that the client has not yet processed and is eligible to run based
// on the filter are put in the Commands channel for consumption by the client.
func (c *Client) commandHandler() {
	for {
		commands := <-c.commands

		var ids []int
		for k, _ := range commands {
			ids = append(ids, k)
		}
		sort.Ints(ids)

		for _, id := range ids {
			log.Debug("ron commandHandler: %v", id)
			if id > c.CommandCounter {
				if !c.matchFilter(commands[id]) {
					continue
				}
				log.Debug("ron commandHandler match: %v", id)
				c.CommandCounter = id
				c.Commands <- commands[id]
			}
		}
	}
}

// client connection handler and transport. Messages on chan out are sent to
// the ron server. Incoming messages are put on the message queue to be routed
// by the mux. The entry to handler() also creates the tunnel transport.
func (c *Client) handler() {
	log.Debug("ron handler")

	// create a tunnel
	stop := make(chan bool)
	defer func() { stop <- true }()
	go c.handleTunnel(false, stop)

	enc := gob.NewEncoder(c.conn)
	dec := gob.NewDecoder(c.conn)

	// handle client i/o
	go func() {
		for {
			m := <-c.out
			err := enc.Encode(m)
			if err != nil {
				log.Errorln(err)
				return
			}
		}
	}()

	for {
		var m Message
		err := dec.Decode(&m)
		if err != nil {
			log.Errorln(err)
			return
		}
		c.in <- &m
	}
}

// client heartbeat sent periodically be periodic(). heartbeat() sends the
// client info and any queued responses.
func (c *Client) heartbeat() {
	c.hold.Lock()
	defer c.hold.Unlock()
	log.Debugln("heartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	cin := &Client{
		UUID:           c.UUID,
		Arch:           runtime.GOARCH,
		OS:             runtime.GOOS,
		Hostname:       hostname,
		CommandCounter: c.CommandCounter,
	}

	macs, ips := getNetworkInfo()
	cin.MAC = macs
	cin.IP = ips

	c.responseLock.Lock()
	cin.Responses = c.Responses
	c.Responses = []*Response{}
	c.responseLock.Unlock()

	m := &Message{
		Type:   MESSAGE_CLIENT,
		UUID:   c.UUID,
		Client: cin,
	}

	log.Debug("heartbeat %v", cin)

	c.out <- m
	c.lastHeartbeat = time.Now()
}

// periodically sent the client heartbeat.
func (c *Client) periodic() {
	rate := time.Duration(HEARTBEAT_RATE * time.Second)
	for {
		log.Debug("ron periodic")
		now := time.Now()
		if now.Sub(c.lastHeartbeat) > rate {
			// issue a heartbeat
			c.heartbeat()
		}
		sleep := rate - now.Sub(c.lastHeartbeat)
		time.Sleep(sleep)
	}
}

// mux routes incoming messages from the server based on message type
func (c *Client) mux() {
	for {
		m := <-c.in
		switch m.Type {
		case MESSAGE_TUNNEL:
			// handle a tunnel message
			log.Debugln("ron MESSAGE_TUNNEL")
			c.tunnelData <- m.Tunnel
		case MESSAGE_COMMAND:
			// process an incoming command list
			log.Debugln("ron MESSAGE_COMMAND")
			c.commands <- m.Commands
		case MESSAGE_FILE:
			// let GetFile know we have this file or an error
			c.files <- m
		default:
			log.Error("unknown message type: %v", m.Type)
			return
		}
	}
}

// GetFile asks a ron server to transmit a named file to the client. If
// successful, the file contents will be returned. A nil byte slice response
// with a nil error implies the file was empty.
func (c *Client) GetFile(file string) ([]byte, error) {
	m := &Message{
		Type:     MESSAGE_FILE,
		UUID:     c.UUID,
		Filename: file,
	}
	c.out <- m

	resp := <-c.files
	if resp.Filename != file {
		return nil, fmt.Errorf("filename mismatch: %v : %v", file, resp.Filename)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("%v", resp.Error)
	}

	return resp.File, nil
}

func getNetworkInfo() ([]string, []string) {
	// process network info
	var macs []string
	var ips []string

	ints, err := net.Interfaces()
	if err != nil {
		log.Errorln(err)
	}
	for _, v := range ints {
		if v.HardwareAddr.String() == "" {
			// skip localhost and other weird interfaces
			continue
		}
		log.Debug("found mac: %v", v.HardwareAddr)
		macs = append(macs, v.HardwareAddr.String())
		addrs, err := v.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, w := range addrs {
			// trim the cidr from the end
			var ip string
			i := strings.Split(w.String(), "/")
			if len(i) != 2 {
				if !isIPv4(w.String()) {
					log.Error("malformed ip: %v", i, w)
					continue
				}
				ip = w.String()
			} else {
				ip = i[0]
			}
			log.Debug("found ip: %v", ip)
			ips = append(ips, ip)
		}
	}
	return macs, ips
}

func (c *Client) matchFilter(command *Command) bool {
	if command.Filter == nil {
		return true
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	f := command.Filter

	if f.UUID != "" && f.UUID != c.UUID {
		log.Debug("failed match on UUID %v %v", f.UUID, c.UUID)
		return false
	}
	if f.Hostname != "" && f.Hostname != hostname {
		log.Debug("failed match on hostname %v %v", f.Hostname, hostname)
		return false
	}
	if f.Arch != "" && f.Arch != runtime.GOARCH {
		log.Debug("failed match on arch %v %v", f.Arch, runtime.GOARCH)
		return false
	}
	if f.OS != "" && f.OS != runtime.GOOS {
		log.Debug("failed match on os %v %v", f.OS, runtime.GOOS)
		return false
	}

	macs, ips := getNetworkInfo()

	if len(f.IP) != 0 {
		// special case, IPs can match on CIDRs as well as full IPs
		match := false
	MATCH_FILTER_IP:
		for _, i := range f.IP {
			for _, ip := range ips {
				if i == ip || matchCIDR(i, ip) {
					log.Debug("match on ip %v %v", i, ip)
					match = true
					break MATCH_FILTER_IP
				}
				log.Debug("failed match on ip %v %v", i, ip)
			}
		}
		if !match {
			return false
		}
	}
	if len(f.MAC) != 0 {
		match := false
	MATCH_FILTER_MAC:
		for _, m := range f.MAC {
			for _, mac := range macs {
				if mac == m {
					log.Debug("match on mac %v %v", m, mac)
					match = true
					break MATCH_FILTER_MAC
				}
				log.Debug("failed match on mac %v %v", m, mac)
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func matchCIDR(cidr string, ip string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}

	d := strings.Split(cidr, "/")
	log.Debugln("subnet ", d)
	if len(d) != 2 {
		return false
	}
	if !isIPv4(d[0]) {
		return false
	}

	netmask, err := strconv.Atoi(d[1])
	if err != nil {
		return false
	}
	network := toInt32(d[0])
	ipmask := toInt32(ip) & ^((1 << uint32(32-netmask)) - 1)
	log.Debug("got network %v and ipmask %v", network, ipmask)
	if ipmask == network {
		return true
	}
	return false
}

func isIPv4(ip string) bool {
	d := strings.Split(ip, ".")
	if len(d) != 4 {
		return false
	}

	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 255 {
			return false
		}
	}

	return true
}

func toInt32(ip string) uint32 {
	d := strings.Split(ip, ".")

	var ret uint32
	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}

		ret <<= 8
		ret |= uint32(octet) & 0x000000ff
	}
	return ret
}
