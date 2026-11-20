// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import "testing"

// Lines from "qemu-system-x86_64 -M help" of the Ubuntu 22.04 package (QEMU 6.2).
const jammyMachines = `Supported machines are:
microvm              microvm (i386)
pc-i440fx-jammy-maxcpus Ubuntu 22.04 PC (i440FX + PIIX, maxcpus=1024, 1996)
ubuntu               Ubuntu 22.04 PC (i440FX + PIIX, 1996) (alias of pc-i440fx-jammy)
pc-i440fx-jammy      Ubuntu 22.04 PC (i440FX + PIIX, 1996) (default)
pc-i440fx-jammy-hpb  Ubuntu 22.04 PC (i440FX + PIIX +host-phys-bits=true, 1996)
pc                   Standard PC (i440FX + PIIX, 1996) (alias of pc-i440fx-6.2)
pc-i440fx-6.2        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-6.1        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-6.0        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.12       Standard PC (i440FX + PIIX, 1996)
pc-i440fx-2.4        Standard PC (i440FX + PIIX, 1996)
pc-i440fx-1.4        Standard PC (i440FX + PIIX, 1996)
q35                  Standard PC (Q35 + ICH9, 2009) (alias of pc-q35-6.2)
pc-q35-6.2           Standard PC (Q35 + ICH9, 2009)
`

// Lines in the form of upstream QEMU, where the default machine type is the
// latest version and "pc" is its alias.
const upstreamMachines = `Supported machines are:
pc                   Standard PC (i440FX + PIIX, 1996) (alias of pc-i440fx-11.1)
pc-i440fx-11.1       Standard PC (i440FX + PIIX, 1996) (default)
pc-i440fx-11.0       Standard PC (i440FX + PIIX, 1996)
pc-i440fx-10.2       Standard PC (i440FX + PIIX, 1996)
pc-i440fx-4.2        Standard PC (i440FX + PIIX, 1996) (deprecated)
q35                  Standard PC (Q35 + ICH9, 2009) (alias of pc-q35-11.1)
pc-q35-11.1          Standard PC (Q35 + ICH9, 2009)
`

func TestOlderMachine(t *testing.T) {
	for _, tt := range []struct {
		name, help, want string
	}{
		{"jammy", jammyMachines, "pc-i440fx-6.1"},
		{"upstream", upstreamMachines, "pc-i440fx-11.0"},
		{"versions compare as numbers", "pc-i440fx-2.9 x\npc-i440fx-2.12 x\npc-i440fx-2.10 x\n", "pc-i440fx-2.12"},
		{"deprecated skipped", "pc-i440fx-9.0 x (deprecated)\npc-i440fx-8.0 x\n", "pc-i440fx-8.0"},
	} {
		got, err := OlderMachine(tt.help)
		if err != nil || got != tt.want {
			t.Errorf("%v: OlderMachine() = %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}

	for _, help := range []string{
		"",
		"pc   Standard PC (alias of pc-i440fx-6.2)\npc-i440fx-6.2 Standard PC\n",
		"pc-i440fx-2.3 Standard PC\npc-i440fx-1.7 Standard PC\n",
		"pc-q35-6.1 Standard PC\npc-i440fx-jammy Ubuntu\n",
	} {
		if got, err := OlderMachine(help); err == nil {
			t.Errorf("OlderMachine(%q) = %q, want an error", help, got)
		}
	}
}
