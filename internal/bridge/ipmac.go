// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"net"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

// snoopParser decodes the ARP and Neighbor Solicitation frames that the
// bridge snooper reads.
type snoopParser struct {
	dot1q layers.Dot1Q
	eth   layers.Ethernet
	ip4   layers.IPv4
	ip6   layers.IPv6
	arp   layers.ARP

	parser  *gopacket.DecodingLayerParser
	decoded []gopacket.LayerType
}

func newSnoopParser() *snoopParser {
	p := &snoopParser{}
	p.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet,
		&p.dot1q,
		&p.eth,
		&p.ip4,
		&p.ip6,
		&p.arp,
	)

	return p
}

// parse returns the source MAC and IP address in a snooped frame, or a nil IP
// when the frame has none.
func (p *snoopParser) parse(data []byte) (string, net.IP) {
	if err := p.parser.DecodeLayers(data, &p.decoded); err != nil {
		if err2, ok := err.(gopacket.UnsupportedLayerType); ok {
			switch gopacket.LayerType(err2) {
			case layers.LayerTypeICMPv6, gopacket.LayerTypePayload:
				// ignore
				err = nil
			default:
				return "", nil
			}
		}

		if err != nil {
			log.Error("error parsing packet: %v", err)
			return "", nil
		}
	}

	// ICMPv6 has no decoder here, so a Neighbor Solicitation ends at IPv6.
	// Duplicate Address Detection sends it from the unspecified address.
	for _, layerType := range p.decoded {
		switch layerType {
		case layers.LayerTypeIPv6:
			if p.ip6.NextHeader == layers.IPProtocolICMPv6 && !p.ip6.SrcIP.IsUnspecified() {
				return p.eth.SrcMAC.String(), p.ip6.SrcIP
			}
		case layers.LayerTypeARP:
			return p.eth.SrcMAC.String(), net.IP(p.arp.SourceProtAddress)
		}
	}

	return "", nil
}

func (b *Bridge) snooper() {
	parser := newSnoopParser()

	for !b.destroyed() {
		data, _, err := b.handle.ReadPacketData()
		if err == pcap.NextErrorTimeoutExpired {
			continue
		} else if err != nil {
			// only log error if it's not because we've been destroyed
			if !b.destroyed() {
				log.Error("error reading packet data: %v", err)
			}
			break
		}

		if mac, ip := parser.parse(data); ip != nil {
			b.updateIP(mac, ip)
		}
	}

	log.Info("%v snoop out", b.Name)
}

func (b *Bridge) updateIP(mac string, ip net.IP) {
	if ip == nil || ip.IsLinkLocalUnicast() {
		return
	}

	log.Debug("got mac/ip pair: %v, %v", mac, ip)

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
