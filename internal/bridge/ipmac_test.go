// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func serialize(t *testing.T, l ...gopacket.SerializableLayer) []byte {
	t.Helper()

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, l...); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	return buf.Bytes()
}

func TestSnoopParser(t *testing.T) {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")

	arp := serialize(t,
		&layers.Ethernet{SrcMAC: mac, DstMAC: layers.EthernetBroadcast, EthernetType: layers.EthernetTypeARP},
		&layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			HwAddressSize:     6,
			ProtAddressSize:   4,
			Operation:         layers.ARPRequest,
			SourceHwAddress:   mac,
			SourceProtAddress: net.IPv4(10, 0, 0, 7).To4(),
			DstHwAddress:      make([]byte, 6),
			DstProtAddress:    net.IPv4(10, 0, 0, 1).To4(),
		},
	)

	taggedARP := serialize(t,
		&layers.Ethernet{SrcMAC: mac, DstMAC: layers.EthernetBroadcast, EthernetType: layers.EthernetTypeDot1Q},
		&layers.Dot1Q{VLANIdentifier: 101, Type: layers.EthernetTypeARP},
		&layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			HwAddressSize:     6,
			ProtAddressSize:   4,
			Operation:         layers.ARPRequest,
			SourceHwAddress:   mac,
			SourceProtAddress: net.IPv4(10, 0, 0, 8).To4(),
			DstHwAddress:      make([]byte, 6),
			DstProtAddress:    net.IPv4(10, 0, 0, 1).To4(),
		},
	)

	src6 := net.ParseIP("fd00::7")
	ip6 := &layers.IPv6{Version: 6, NextHeader: layers.IPProtocolICMPv6, HopLimit: 255, SrcIP: src6, DstIP: net.ParseIP("ff02::1:ff00:1")}
	icmp6 := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborSolicitation, 0)}
	if err := icmp6.SetNetworkLayerForChecksum(ip6); err != nil {
		t.Fatal(err)
	}
	ns := serialize(t,
		&layers.Ethernet{SrcMAC: mac, DstMAC: layers.EthernetBroadcast, EthernetType: layers.EthernetTypeIPv6},
		ip6,
		icmp6,
		gopacket.Payload(make([]byte, 20)),
	)

	dad6 := &layers.IPv6{Version: 6, NextHeader: layers.IPProtocolICMPv6, HopLimit: 255, SrcIP: net.IPv6unspecified, DstIP: net.ParseIP("ff02::1:ff00:7")}
	dadICMP6 := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborSolicitation, 0)}
	if err := dadICMP6.SetNetworkLayerForChecksum(dad6); err != nil {
		t.Fatal(err)
	}
	dad := serialize(t,
		&layers.Ethernet{SrcMAC: mac, DstMAC: layers.EthernetBroadcast, EthernetType: layers.EthernetTypeIPv6},
		dad6,
		dadICMP6,
		gopacket.Payload(make([]byte, 20)),
	)

	tests := []struct {
		name string
		data []byte
		ip   string
	}{
		{"arp", arp, "10.0.0.7"},
		{"tagged arp", taggedARP, "10.0.0.8"},
		{"neighbor solicitation", ns, "fd00::7"},
		{"duplicate address detection", dad, ""},
		{"garbage", []byte{1, 2, 3}, ""},
	}

	p := newSnoopParser()

	for _, tc := range tests {
		gotMAC, gotIP := p.parse(tc.data)

		if tc.ip == "" {
			if gotIP != nil {
				t.Errorf("%s: got IP %v, want none", tc.name, gotIP)
			}
			continue
		}

		if gotMAC != mac.String() || !gotIP.Equal(net.ParseIP(tc.ip)) {
			t.Errorf("%s: got %v %v, want %v %v", tc.name, gotMAC, gotIP, mac, tc.ip)
		}
	}
}
