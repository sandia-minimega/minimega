// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import "testing"

func TestBindClientUUIDUsesSerialIdentity(t *testing.T) {
	want := "3b440429-067f-5b75-a0c3-f436519f8ccb"
	m := &Message{Client: &Client{}}
	if err := bindClientUUID(m, want); err != nil {
		t.Fatal(err)
	}
	if m.Client.UUID != want || m.UUID != want {
		t.Fatalf("serial identity was not adopted: %#v", m)
	}
}

func TestBindClientUUIDPreservesGuestIdentity(t *testing.T) {
	want := "a5ba6920-5bcf-4022-b8cf-015425f7b05c"
	m := &Message{Client: &Client{UUID: want}}
	if err := bindClientUUID(m, "different-host-identity"); err != nil {
		t.Fatal(err)
	}
	if m.Client.UUID != want {
		t.Fatalf("guest identity changed: %q", m.Client.UUID)
	}
}

func TestBindClientUUIDRejectsUnboundIdentity(t *testing.T) {
	for name, m := range map[string]*Message{
		"missing client": {},
		"missing UUID":   {Client: &Client{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := bindClientUUID(m, ""); err == nil {
				t.Fatal("expected an unbound client identity error")
			}
		})
	}
}
