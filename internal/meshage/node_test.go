// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package meshage

import (
	"net"
	"testing"
)

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

// TestBroadcastRecipientsSeesNewPeer checks that BroadcastRecipients includes a
// peer as soon as its MSA arrives, before the periodic refresh that Mesh waits
// for; iomeshage relies on this to decide whether it is alone in the mesh.
func TestBroadcastRecipientsSeesNewPeer(t *testing.T) {
	n, _ := NewNode("a", t.Name(), 0, net.IPv4(127, 0, 0, 1), freePort(t), "test")

	// a is connected to b, and b's MSA says it is connected to a
	n.meshLock.Lock()
	n.network["a"] = []string{"b"}
	n.meshLock.Unlock()
	n.handleMSA(&Message{Source: "b", Body: []string{"a"}})

	if len(n.Mesh()) != 1 {
		t.Skipf("mesh already refreshed: %v", n.Mesh())
	}

	got := n.BroadcastRecipients()
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("BroadcastRecipients() = %v, want [b]", got)
	}
}
