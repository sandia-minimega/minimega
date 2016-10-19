// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"encoding/gob"
	"io"
	log "minilog"
	"minitunnel"
	"net"
	"strings"
	"sync"
	"time"
)

type Client struct {
	UUID     string
	Arch     string
	OS       string
	Version  string
	Hostname string
	IPs      []string
	MACs     []string

	// Processes that are running in the background
	Processes map[int]*Process

	// Tags set via the command socket since the last heartbeat
	Tags map[string]string

	// CommandCounter shows the highest command ID that the client has
	// processed so far
	CommandCounter int

	// Responses for commands processed since the last heartbeat
	Responses []*Response

	// Files requested by the server since the last heartbeat
	Files []*File
}

type client struct {
	*Client    // embed
	sync.Mutex // embed

	Checkin time.Time // when the client last checked in

	tunnel *minitunnel.Tunnel

	// writeMu serializes calls to enc.Encode
	writeMu sync.Mutex

	conn io.ReadWriteCloser
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func (c *client) sendMessage(m *Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.enc.Encode(m)
}

// Matches tests whether all the filters match the client.
func (c *Client) Matches(f *Filter) bool {
	if f == nil {
		return true
	}

	if f.UUID != "" && f.UUID != c.UUID {
		log.Debug("failed match on UUID: %v != %v", f.UUID, c.UUID)
		return false
	}
	if f.Hostname != "" && f.Hostname != c.Hostname {
		log.Debug("failed match on hostname: %v != %v", f.Hostname, c.Hostname)
		return false
	}
	if f.Arch != "" && f.Arch != c.Arch {
		log.Debug("failed match on arch: %v != %v", f.Arch, c.Arch)
		return false
	}
	if f.OS != "" && f.OS != c.OS {
		log.Debug("failed match on os: %v != %v", f.OS, c.OS)
		return false
	}

	for k, v := range f.Tags {
		if c.Tags[k] != v {
			log.Debug("failed match on tag %v, %v != %v", k, v, c.Tags[k])
			return false
		}
	}

	return c.matchesIP(f) && c.matchesMAC(f)
}

// matchesIP tests whether the IP or CIDR specified in the filter matches at
// least one of the client's IPs.
func (c *Client) matchesIP(f *Filter) bool {
	if f.IP == "" {
		return true
	}

	// special case, IPs can match on CIDRs as well as full IPs
	if strings.Contains(f.IP, "/") {
		_, ipnet, err := net.ParseCIDR(f.IP)
		if err != nil {
			log.Error("invalid CIDR %v: %v", f.IP, err)
			return false
		}

		for _, ip := range c.IPs {
			if ipnet.Contains(net.ParseIP(ip)) {
				return true
			}
			log.Debug("failed match on CIDR %v %v", f.IP, ip)
		}

		return false
	}

	i := net.ParseIP(f.IP)
	if i == nil {
		log.Error("invalid IP: %v", f.IP)
		return false
	}

	for _, ip := range c.IPs {
		if i.Equal(net.ParseIP(ip)) {
			return true
		}
		log.Debug("failed match on ip %v %v", f.IP, ip)
	}

	return false
}

// matchesMAC tests whether the MAC specified in the filter matches at least
// one of the client's MACs.
func (c *Client) matchesMAC(f *Filter) bool {
	if f.MAC == "" {
		return true
	}

	for _, mac := range c.MACs {
		if f.MAC == mac {
			return true
		}

		log.Debug("failed match on mac %v %v", f.MAC, mac)
	}

	return false
}
