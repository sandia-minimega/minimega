// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"log"
	"minicli"
	"net"

	"github.com/mdlayher/dhcp6"
	"golang.org/x/net/ipv6"
)

var dhcp6CLIHandlers = []minicli.Handler{
	{ // dhcp6Server
		HelpShort: "start a dhcpv6 server which serves a specified ip",
		HelpLong: `
Start a dhcp/dns server that serves a specified IP. For example,
to start a DHCP server which serves IP address dead:beef:d34d:b33f::10, do:

	dhcp6 ip dead:beef:d34d:b33f::10

To specify an server interface, do the following:

	dhcp6 i eth0 ip dead:beef:d34d:b33f::10`,

		Patterns: []string{
			"dhcp6 ip <ipv6 address>",
			"dhcp6 i <interface> ip <ipv6 address>",
		},
		Call: wrapSimpleCLI(cliDhcp6Server),
	},
}

func cliDhcp6Server(c *minicli.Command, resp *minicli.Response) error {
	// Only accept a single IPv6 address
	ip := net.ParseIP(c.StringArgs["ipv6"]).To16()
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("IP is not an IPv6 address")
	}

	// Set a default interface if not specified
	iface, ok := c.StringArgs["interface"]
	if !ok {
		iface = "eth0"
	}

	// Make Handler to assign ip and use handle for requests
	h := &Handler{
		ip:      ip,
		handler: handle,
	}

	// Listen and serve.
	log.Printf("binding DHCPv6 server to interface %s...", iface)
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return err
	}

	server := &dhcp6.Server{
		Iface:   ifi,
		Addr:    "[::]:547",
		Handler: h,
		MulticastGroups: []*net.IPAddr{
			dhcp6.AllRelayAgentsAndServersAddr,
			dhcp6.AllServersAddr,
		},
	}
	conn, err := net.ListenPacket("udp6", server.Addr)
	if err != nil {
		return err
	}

	go func(s *dhcp6.Server, c net.PacketConn) {
		defer conn.Close()
		err = s.Serve(ipv6.NewPacketConn(conn))
		log.Printf("DHCPv6 Server error: %v\n", err)
	}(server, conn)

	return nil
}
