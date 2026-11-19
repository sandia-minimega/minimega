// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qemu

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Values from QEMU's migration/savevm.h. They are the same in QEMU 6.2 and
// QEMU 11.1.
const (
	stateMagic         = 0x5145564d // "QEVM"
	stateVersion       = 3
	stateConfiguration = 0x07 // QEMU_VM_CONFIGURATION
)

// MaxStateMachineLen is the longest machine name that StateMachine accepts.
const MaxStateMachineLen = 256

// StateHeaderLen is the number of bytes in front of the machine name in a
// migration stream: magic (4), version (4), section type (1), and name
// length (4).
const StateHeaderLen = 13

// StateMachine reads the machine type name from the header of a QEMU
// migration stream, such as a state file that "vm save" writes.
//
// The stream starts with the magic "QEVM", a big-endian version, and, when
// the source QEMU sends it, a configuration section. That section is the byte
// 0x07, a big-endian 32-bit name length, and the name without a terminator.
// Other data follows the name.
//
// StateMachine returns "" and no error when the stream has no machine name:
// the magic is not "QEVM", the version is not 3, or the configuration section
// is missing. QEMU machine types 2.3 and older do not send that section.
// QEMU itself reports an error for a stream that it cannot load.
//
// StateMachine returns an error when it cannot read the full header after
// the magic, or when the name length is 0 or more than MaxStateMachineLen. It
// also returns an error when the name has a byte that is not printable ASCII
// or is a space.
func StateMachine(r io.Reader) (string, error) {
	var head [StateHeaderLen]byte

	if _, err := io.ReadFull(r, head[:4]); err != nil {
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			// too short to be a migration stream
			return "", nil
		}
		return "", err
	}
	if binary.BigEndian.Uint32(head[0:4]) != stateMagic {
		return "", nil
	}

	if _, err := io.ReadFull(r, head[4:8]); err != nil {
		return "", fmt.Errorf("read migration stream version: %w", eofIsUnexpected(err))
	}
	if binary.BigEndian.Uint32(head[4:8]) != stateVersion {
		return "", nil
	}

	if _, err := io.ReadFull(r, head[8:9]); err != nil {
		return "", fmt.Errorf("read migration stream section: %w", eofIsUnexpected(err))
	}
	if head[8] != stateConfiguration {
		return "", nil
	}

	if _, err := io.ReadFull(r, head[9:13]); err != nil {
		return "", fmt.Errorf("read machine name length: %w", eofIsUnexpected(err))
	}
	size := binary.BigEndian.Uint32(head[9:13])
	if size == 0 || size > MaxStateMachineLen {
		return "", fmt.Errorf("invalid machine name length in migration stream: %d", size)
	}

	name := make([]byte, size)
	if _, err := io.ReadFull(r, name); err != nil {
		return "", fmt.Errorf("read machine name: %w", eofIsUnexpected(err))
	}
	for _, b := range name {
		if b < 0x21 || b > 0x7e {
			return "", fmt.Errorf("invalid machine name in migration stream: %q", name)
		}
	}

	return string(name), nil
}

func eofIsUnexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// upstreamMachines maps machine types that Ubuntu adds to its QEMU packages
// to the upstream QEMU machine types of the same QEMU release.
//
// The table has only the Ubuntu 22.04 (jammy, QEMU 6.2) and Ubuntu 24.04
// (noble, QEMU 8.2) types: the minimega Docker image used these releases, so
// state files from that image can have these names. A restore on upstream
// QEMU with the jammy to 6.2 replacement was tested. The noble entries use
// the same rule and are not tested.
//
// Some Ubuntu types change more than the name, so they are not equal to an
// upstream type and are not in the table: "-hpb" sets host-phys-bits,
// "-maxcpus" sets maxcpus=1024, and "pc-i440fx-noble-v2" and
// "pc-q35-noble-v2" add an arch_caps fix.
var upstreamMachines = map[string]string{
	"pc-i440fx-jammy": "pc-i440fx-6.2",
	"pc-q35-jammy":    "pc-q35-6.2",
	"pc-i440fx-noble": "pc-i440fx-8.2",
	"pc-q35-noble":    "pc-q35-8.2",
}

// UpstreamMachine returns the upstream QEMU machine type for a machine type
// that a Linux distribution adds to its QEMU package. It returns false when
// it does not know the name.
func UpstreamMachine(name string) (string, bool) {
	v, ok := upstreamMachines[name]
	return v, ok
}
