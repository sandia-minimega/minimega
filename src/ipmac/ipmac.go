// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// ipmac attempts to learn about active ip addresses associated with mac
// addresses on a particular interface, usually a bridge that can see data from
// many other interfaces. ipmac is used by creating a new ipmac object on a
// particular interface, and providing one or more MAC addresses to filter on.
package ipmac

import (
	"gopacket"
	"gopacket/layers"
	"gopacket/pcap"
	"io"
	log "minilog"
	"net"
	"sync"
)

type IPMacLearner struct {
	handle *pcap.Handle
	pairs  map[string]chan net.IP
	lock   sync.Mutex
}

// NewLearner returns an IPMacLearner object bound to a particular interface.
func NewLearner(dev string) (*IPMacLearner, error) {
	handle, err := pcap.OpenLive(dev, 1600, true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	// Interested in:
	//  * ARP
	//  * Neighbor Solicitation (NDP)
	if err := handle.SetBPFFilter("(arp or (icmp6 and ip6[40] == 135))"); err != nil {
		handle.Close()
		return nil, err
	}

	iml := &IPMacLearner{
		handle: handle,
		pairs:  make(map[string]chan net.IP),
	}

	go iml.learner()

	return iml, nil
}

// Add a MAC address to the list of addresses to search for. IPMacLearner will
// not gather information on MAC addresses not in the list.
func (iml *IPMacLearner) AddMac(mac string, out chan net.IP) {
	iml.lock.Lock()
	defer iml.lock.Unlock()

	log.Debugln("adding mac to filter:", mac)
	iml.pairs[mac] = out
}

// Delete a MAC address from the list of addresses to search for.
func (iml *IPMacLearner) DelMac(mac string) {
	iml.lock.Lock()
	defer iml.lock.Unlock()

	// Ensure channel exists before trying to close it
	if _, ok := iml.pairs[mac]; ok {
		close(iml.pairs[mac])
	}

	delete(iml.pairs, mac)
}

// Stop searching for IP addresses.
func (iml *IPMacLearner) Close() {
	iml.lock.Lock()
	defer iml.lock.Unlock()

	for _, chn := range iml.pairs {
		close(chn)
	}

	iml.handle.Close()
}

func (iml *IPMacLearner) learner() {
	var packet struct {
		dot1q layers.Dot1Q
		eth   layers.Ethernet
		ip4   layers.IPv4
		ip6   layers.IPv6
		arp   layers.ARP
	}

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet,
		&packet.dot1q,
		&packet.eth,
		&packet.ip4,
		&packet.ip6,
		&packet.arp,
	)

	decodedLayers := []gopacket.LayerType{}

	for {
		data, _, err := iml.handle.ReadPacketData()
		if err != nil {
			if err != io.EOF {
				log.Error("Error reading packet data: ", err)
			}
			break
		}

		if err := parser.DecodeLayers(data, &decodedLayers); err != nil {
			log.Error("Error parsing packet: %v", err)
		}

		for _, layerType := range decodedLayers {
			switch layerType {
			case layers.LayerTypeICMPv6:
				iml.sendUpdate(packet.eth.SrcMAC, packet.ip6.SrcIP)
			case layers.LayerTypeARP:
				iml.sendUpdate(packet.eth.SrcMAC, net.IP(packet.arp.SourceProtAddress))
			}
		}
	}
}

func (iml *IPMacLearner) sendUpdate(mac net.HardwareAddr, ip net.IP) {
	log.Debug("got mac/ip pair:", mac, ip)

	iml.lock.Lock()
	defer iml.lock.Unlock()

	// skip macs we aren't tracking
	if _, ok := iml.pairs[mac.String()]; !ok {
		return
	}

	iml.pairs[mac.String()] <- ip
}
