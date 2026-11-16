// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/vnc"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// serveRFB is a minimal RFB 3.3 server: handshake, then swallow input.
func serveRFB(t *testing.T) (host string, port int) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				io.WriteString(c, "RFB 003.003\n")
				io.ReadFull(c, make([]byte, 12))
				c.Write([]byte{0, 0, 0, 1})
				io.ReadFull(c, make([]byte, 1))
				binary.Write(c, binary.BigEndian, uint16(640))
				binary.Write(c, binary.BigEndian, uint16(480))
				binary.Write(c, binary.BigEndian, vnc.PixelFormat{BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1})
				binary.Write(c, binary.BigEndian, uint32(4))
				io.WriteString(c, "test")
				io.Copy(io.Discard, c)
			}()
		}
	}()

	a := l.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}

// `vnc getstep` on a VM with an active playback must succeed, not report
// "unreachable", and list the VM.
func TestVNCGetStep(t *testing.T) {
	host, port := serveRFB(t)

	file := filepath.Join(t.TempDir(), "p.kbr")
	if err := os.WriteFile(file, []byte("30000000000:KeyEvent,true,z\n"), 0644); err != nil {
		t.Fatal(err)
	}

	vm := &KvmVM{BaseVM: &BaseVM{Name: "vm0", Host: host}}
	vm.VNCPort = port
	ns := &Namespace{
		VMs:    VMs{m: map[int]VM{0: vm}},
		Player: vnc.NewPlayer(),
	}

	if err := ns.Player.Playback("vm0", net.JoinHostPort(host, strconv.Itoa(port)), file); err != nil {
		t.Fatal(err)
	}
	defer ns.Player.Stop("vm0")

	c := &minicli.Command{
		StringArgs: map[string]string{"vm": "vm0", "filename": file},
		BoolArgs:   map[string]bool{"getstep": true},
	}
	resp := &minicli.Response{}

	if err := cliVNCPlay(ns, c, resp); err != nil {
		t.Fatalf("vnc getstep: %v", err)
	}
	if len(resp.Tabular) != 1 || resp.Tabular[0][0] != "vm0" {
		t.Fatalf("unexpected tabular: %v", resp.Tabular)
	}
}
