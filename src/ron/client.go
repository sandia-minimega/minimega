// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	log "minilog"
	"net"
	"os"
)

func (c *Client) Dial(parent string, port int) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", parent, port))
	if err != nil {
		return err
	}

	c.conn = conn

	go c.handler()
	go c.mux()
	go c.periodic()
	go c.commandHandler()
	c.heartbeat()

	return nil
}

func (c *Client) DialSerial(path string) error {
	conn, err := os.OpenFile(r.serialPath, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	
	c.conn = conn

	go c.handler()
	go c.mux()
	go c.periodic()
	go c.commandHandler()
	c.heartbeat()

	return nil
}

func (c *Client) Respond(r *Response) {
	c.responseLock.Lock()
	c.Responses = append(c.Responses, r)
	c.responseLock.Unlock()
}

func (c *Client) commandHandler() {
	for {
		commands := <-c.commands

		var ids []int
		for k, _ := range commands {
			ids = append(ids, k)
		}
		sort.Ints(ids)

		for _, id := range ids {
			if id > c.commandCounter {
				if !r.matchFilter(commands[id]) {
					continue
				}
				c.commandCounter = id
				c.Commands <- commands[id]
			}
		}
	}
}

func (c *Client) handler() {
	enc := gob.NewEncoder(c.conn)
	dec := gob.NewDecoder(c.conn)

	// handle client i/o
	go func() {
		for {
			m := <-c.out
			err := enc.Encode(m)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}()

	for {
		var m Message
		err := dec.Decode(&m)
		if err != nil {
			log.Fatalln(err)
		}
		c.in <- &m
	}
}

func (c *Client) heartbeat() {
	log.Debugln("heartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	cin := &Client{
		UUID:     c.UUID,
		Arch:     runtime.GOARCH,
		OS:       runtime.GOOS,
		Hostname: hostname,
	}

	macs, ips := getNetworkInfo()
	cin.MAC = macs
	cin.IP = ips

	c.responseLock.Lock()
	cin.Responses = c.Responses
	c.Responses = []*Response
	c.responseLock.Unlock()

	m := &Message{
		UUID:         c.UUID,
		Client:       cin,
	}

	log.Debug("heartbeat %v", cin)

	c.out <- m
	c.lastHeartbeat = time.Now()
}

func (c *Client) periodic() {
	rate := time.Duration(HEARTBEAT_RATE * time.Second)
	for {
		now := time.Now()
		if c.lastHeartbeat.Sub(now) > rate {
			// issue a heartbeat
			c.heartbeat()
		}
		sleep := rate - c.lastHeartbeat.Sub(now)
		time.Sleep(sleep)
	}
}

func (c *Client) mux() {
	for {
		m := <-c.in
		switch m.Type {
		case MESSAGE_TUNNEL:
			// handle a tunnel message
		case MESSAGE_COMMAND:
			// process an incoming command list
			c.commands <- m.Commands	
		default:
			log.Error("unknown message type: %v", m.Type)
			return
		}
	}
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
			continue
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
			continue
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
