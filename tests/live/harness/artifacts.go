// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
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

// FBChunks counts the framebuffer chunks in a VNC recording: a gzip stream of
// "<offset> <n>\r\n<n bytes>\r\n". A recording that is still being written
// counts the chunks complete so far.
func FBChunks(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	z, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	r := bufio.NewReader(z)
	n := 0
	for {
		var offset, size int64
		if _, err := fmt.Fscanf(r, "%d %d\r\n", &offset, &size); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return n, nil
			}
			return n, fmt.Errorf("chunk %d header: %w", n, err)
		}
		if _, err := io.CopyN(io.Discard, r, size+2); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return n, nil
			}
			return n, err
		}
		n++
	}
}
