// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"github.com/krolaw/dhcp4"

	"fmt"
	"log"
	"minicli"
	"net"
	"time"
)

var dhcp4CLIHandlers = []minicli.Handler{
	{ // dhcp4Server
		HelpShort: "start a dhcpv4 server on a specified ip which starts serving from a specified range",
		HelpLong: `
Start a dhcp/dns server on a specified IP which starts serving from a specified range. For example,
to start a DHCP server on 192.168.0.1 and serves from 192.168.0.2, do:

	dhcp4 server 192.168.0.1 start 192.168.0.2

To specify an server interface, eth0, do the following:

	dhcp4 i eth0 server 192.168.0.1 start 192.168.0.2`,

		Patterns: []string{
			"dhcp4 server <server address> start <start of address ranges>",
			"dhcp4 i <interface> server <server address> start <start of address ranges>",
		},
		Call: wrapSimpleCLI(cliDhcp4Server),
	},
}

// Use DHCP with a single network interface device
func cliDhcp4Server(c *minicli.Command, resp *minicli.Response) error {
	serverIP := net.ParseIP(c.StringArgs["server"]).To4()
	if serverIP == nil {
		return fmt.Errorf("Server IP is not an IPv4 address")
	}

	startIP := net.ParseIP(c.StringArgs["start"]).To4()
	if serverIP == nil {
		return fmt.Errorf("Start of address ranges is not an IPv4 address")
	}

	iface, ok := c.StringArgs["interface"]
	if !ok {
		iface = "eth0"
	}

	handler := &DHCPHandler{
		ip:            serverIP,
		leaseDuration: 2 * time.Hour,
		start:         startIP,
		leaseRange:    50,
		leases:        make(map[int]lease, 10),
		options: dhcp4.Options{
			dhcp4.OptionSubnetMask: []byte{255, 255, 240, 0},
			// Presuming Server is also your router
			dhcp4.OptionRouter: []byte(serverIP),
			// Presuming Server is also your DNS server
			dhcp4.OptionDomainNameServer: []byte(serverIP),
		},
	}

	go func(i string, h *DHCPHandler) {
		log.Printf("binding DHCPv4 server to interface %v...\n", i)
		err := dhcp4.ListenAndServeIf(i, h)
		log.Printf("DHCPv4 Server error: %v\n", err)
	}(iface, handler)

	return nil
}
