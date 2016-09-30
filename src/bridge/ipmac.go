// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// ipmac attempts to learn about active ip addresses associated with mac
// addresses on a particular interface, usually a bridge that can see data from
// many other interfaces. ipmac is used by creating a new ipmac object on a
// particular interface, and providing one or more MAC addresses to filter on.
package bridge

import (
	"gopacket"
	"gopacket/layers"
	"io"
	log "minilog"
	"net"
)

func (b *Bridge) Close() {
	b.handle.Close()
}

func (b *Bridge) snooper() {
	var (
		dot1q layers.Dot1Q
		eth   layers.Ethernet
		ip4   layers.IPv4
		ip6   layers.IPv6
		arp   layers.ARP
	)

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet,
		&dot1q,
		&eth,
		&ip4,
		&ip6,
		&arp,
	)

	decodedLayers := []gopacket.LayerType{}

	for {
		data, _, err := b.handle.ReadPacketData()
		if err != nil {
			if err != io.EOF {
				log.Error("error reading packet data: ", err)
			}
			break
		}

		if err := parser.DecodeLayers(data, &decodedLayers); err != nil {
			if err2, ok := err.(gopacket.UnsupportedLayerType); ok {
				switch gopacket.LayerType(err2) {
				case layers.LayerTypeICMPv6, gopacket.LayerTypePayload:
					// ignore
					err = nil
				default:
					continue
				}
			}

			if err != nil {
				log.Error("error parsing packet: %v", err)
				continue
			}
		}

		for _, layerType := range decodedLayers {
			switch layerType {
			case layers.LayerTypeICMPv6:
				b.updateIP(eth.SrcMAC.String(), ip6.SrcIP)
			case layers.LayerTypeARP:
				b.updateIP(eth.SrcMAC.String(), net.IP(arp.SourceProtAddress))
			}
		}
	}

	log.Info("%v snoop out", b.Name)
}

func (b *Bridge) updateIP(mac string, ip net.IP) {
	if ip == nil || ip.IsLinkLocalUnicast() {
		return
	}

	log.Debug("got mac/ip pair:", mac, ip)

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, tap := range b.taps {
		if tap.Defunct || tap.MAC != mac {
			continue
		}

		if ip := ip.To4(); ip != nil {
			tap.IP4 = ip.String()
		} else {
			tap.IP6 = ip.String()
		}
	}
}
