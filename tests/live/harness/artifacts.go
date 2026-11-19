// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// This file reads the artifacts minimega writes (pcaps, screenshots, VNC recordings).

// CountICMPEcho counts ICMP echo requests to dst in a pcap file.
func CountICMPEcho(path, dst string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r, err := pcapgo.NewReader(f)
	if err != nil {
		return 0, err
	}
	want := net.ParseIP(dst)
	n := 0
	for {
		data, _, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			return n, nil
		} else if err != nil {
			return n, err
		}
		p := gopacket.NewPacket(data, r.LinkType(), gopacket.Default)
		ip, _ := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		icmp, _ := p.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)
		if ip != nil && icmp != nil && ip.DstIP.Equal(want) && icmp.TypeCode.Type() == layers.ICMPv4TypeEchoRequest {
			n++
		}
	}
}

// DistinctColors decodes a PNG and counts its distinct colors; a blank
// screen has one.
func DistinctColors(b []byte) (int, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	seen := map[[4]uint32]bool{}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			seen[[4]uint32{r, g, b, a}] = true
		}
	}
	return len(seen), nil
}

// ProcCPU returns the CPU time (user + system) from the contents of a
// /proc/<pid>/stat file, at the usual 100 clock ticks per second.
func ProcCPU(stat string) (time.Duration, error) {
	// the command name is in parentheses and may contain spaces
	i := strings.LastIndex(stat, ")")
	if i < 0 {
		return 0, fmt.Errorf("malformed stat: %q", stat)
	}
	// fields after the name start at field 3; utime and stime are 14 and 15
	f := strings.Fields(stat[i+1:])
	if len(f) < 13 {
		return 0, fmt.Errorf("short stat: %q", stat)
	}
	utime, err1 := strconv.ParseInt(f[11], 10, 64)
	stime, err2 := strconv.ParseInt(f[12], 10, 64)
	if err := errors.Join(err1, err2); err != nil {
		return 0, err
	}
	return time.Duration(utime+stime) * 10 * time.Millisecond, nil
}
