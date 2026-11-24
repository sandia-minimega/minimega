// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qemu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// stateHeader returns the start of a migration stream with the given magic,
// version, section type, name length, and name.
func stateHeader(magic string, version uint32, section byte, size uint32, name string) []byte {
	var b bytes.Buffer
	b.WriteString(magic)
	binary.Write(&b, binary.BigEndian, version)
	b.WriteByte(section)
	binary.Write(&b, binary.BigEndian, size)
	b.WriteString(name)
	return b.Bytes()
}

// subsection is the start of the configuration/target-page-bits subsection
// that follows the machine name in a real stream.
var subsection = []byte("\x05\x1dconfiguration/target-page-bits\x00\x00\x00\x01\x00\x00\x00\x0c")

func TestStateMachine(t *testing.T) {
	good := func(name string) []byte {
		return append(stateHeader("QEVM", 3, 0x07, uint32(len(name)), name), subsection...)
	}

	tests := []struct {
		name    string
		in      []byte
		want    string
		wantErr string
	}{
		{name: "upstream", in: good("pc-i440fx-6.2"), want: "pc-i440fx-6.2"},
		{name: "ubuntu", in: good("pc-i440fx-jammy"), want: "pc-i440fx-jammy"},
		{name: "name only", in: stateHeader("QEVM", 3, 0x07, 3, "q35"), want: "q35"},
		{name: "longest name", in: good(strings.Repeat("m", MaxStateMachineLen)), want: strings.Repeat("m", MaxStateMachineLen)},
		{name: "empty", in: nil, want: ""},
		{name: "short magic", in: []byte("QE"), want: ""},
		{name: "wrong magic", in: stateHeader("QEVX", 3, 0x07, 13, "pc-i440fx-6.2"), want: ""},
		{name: "text file", in: []byte("this is not a migration stream"), want: ""},
		{name: "version 2", in: stateHeader("QEVM", 2, 0x07, 13, "pc-i440fx-6.2"), want: ""},
		{name: "version 4", in: stateHeader("QEVM", 4, 0x07, 13, "pc-i440fx-6.2"), want: ""},
		// QEMU_VM_SECTION_START: machine types 2.3 and older send no configuration
		{name: "no configuration section", in: stateHeader("QEVM", 3, 0x01, 0, ""), want: ""},
		{name: "length 0", in: stateHeader("QEVM", 3, 0x07, 0, ""), wantErr: "invalid machine name length in migration stream: 0"},
		{name: "length 257", in: stateHeader("QEVM", 3, 0x07, 257, strings.Repeat("m", 257)), wantErr: "invalid machine name length in migration stream: 257"},
		{name: "control bytes in name", in: good("pc\x1b[2J\nfake"), wantErr: `invalid machine name in migration stream: "pc\x1b[2J\nfake"`},
		{name: "space in name", in: good("pc 6.2"), wantErr: `invalid machine name in migration stream: "pc 6.2"`},
		{name: "truncated name", in: stateHeader("QEVM", 3, 0x07, 13, "pc-i440fx"), wantErr: "read machine name: unexpected EOF"},
		{name: "truncated version", in: []byte("QEVM\x00\x00"), wantErr: "read migration stream version: unexpected EOF"},
		{name: "no version", in: []byte("QEVM"), wantErr: "read migration stream version: unexpected EOF"},
		{name: "no section", in: []byte("QEVM\x00\x00\x00\x03"), wantErr: "read migration stream section: unexpected EOF"},
		{name: "truncated length", in: []byte("QEVM\x00\x00\x00\x03\x07\x00\x00"), wantErr: "read machine name length: unexpected EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StateMachine(bytes.NewReader(tt.in))
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("StateMachine() = %q, %v; want error %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("StateMachine() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("StateMachine() = %q, want %q", got, tt.want)
			}
		})
	}
}

type failReader struct {
	data []byte
	err  error
}

func (r *failReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestStateMachineReadError(t *testing.T) {
	boom := errors.New("boom")

	for _, prefix := range []string{"", "QE", "QEVM", "QEVM\x00\x00\x00\x03\x07\x00\x00\x00\x0dpc-"} {
		_, err := StateMachine(&failReader{data: []byte(prefix), err: boom})
		if !errors.Is(err, boom) {
			t.Errorf("prefix %q: error = %v, want %v", prefix, err, boom)
		}
	}

	// a reader that returns EOF after the full header still gives the name
	in := stateHeader("QEVM", 3, 0x07, 3, "q35")
	got, err := StateMachine(&failReader{data: in, err: io.EOF})
	if err != nil || got != "q35" {
		t.Errorf("StateMachine() = %q, %v; want q35", got, err)
	}
}

func TestUpstreamMachine(t *testing.T) {
	for in, want := range map[string]string{
		"pc-i440fx-jammy": "pc-i440fx-6.2",
		"pc-q35-jammy":    "pc-q35-6.2",
		"pc-i440fx-noble": "pc-i440fx-8.2",
		"pc-q35-noble":    "pc-q35-8.2",
	} {
		if got, ok := UpstreamMachine(in); !ok || got != want {
			t.Errorf("UpstreamMachine(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}

	for _, in := range []string{"", "pc", "q35", "pc-i440fx-6.2", "pc-i440fx-jammy-hpb", "pc-q35-noble-hpb", "pc-i440fx-jammy-maxcpus", "pc-i440fx-noble-v2", "pc-q35-noble-v2", "pc-i440fx-focal", "ubuntu", "PC-I440FX-JAMMY"} {
		if got, ok := UpstreamMachine(in); ok {
			t.Errorf("UpstreamMachine(%q) = %q, true; want no mapping", in, got)
		}
	}
}
