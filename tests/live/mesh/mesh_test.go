// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

// Package mesh checks a namespace that spans two minimega nodes (see
// topology.mm and setup.sh): scheduling onto the peer and a tar: container
// filesystem that only the first node has.
package mesh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
)

const head = "minimega"

var (
	peer      = envOr("LIVE_PEER_HOST", "minimega-peer")
	peerFiles = os.Getenv("LIVE_PEER_FILES")
)

// VMs from topology.mm and the host each is scheduled on
var placement = map[string]string{
	"ct-head1": head,
	"ct-head2": head,
	"ct-peer1": peer,
	"ct-peer2": peer,
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestMain(m *testing.M) {
	if err := harness.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// TestLive runs every check against the topology loaded from topology.mm.
func TestLive(t *testing.T) {
	p := func(name string, fn func(*testing.T), needs ...string) harness.Phase {
		return harness.Phase{Name: name, Fn: fn, Needs: needs}
	}

	harness.RunPhases(t, []harness.Phase{
		p("Mesh", testMesh),
		p("Namespace", testNamespace, "Mesh"),
		p("TarFilesystem", testTarFilesystem, "Namespace"),
		p("Placement", testPlacement, "Namespace"),
		p("PeerExtracted", testPeerExtracted, "Placement"),
		p("Running", testRunning, "Placement"),
		p("ClearNamespace", testClearNamespace),
	})
}

func testMesh(t *testing.T) {
	resps, err := harness.Raw("mesh status")
	if err != nil {
		t.Fatal(err)
	}

	rows := harness.ToRows(resps)
	if len(rows) != 1 || rows[0]["size"] != "2" {
		t.Fatalf("mesh status = %v, want one row with size 2", rows)
	}

	list := harness.MustText(t, "mesh list")
	if !strings.Contains(list, peer) {
		t.Errorf("mesh list does not include %v:\n%v", peer, list)
	}
}

func testNamespace(t *testing.T) {
	hosts := harness.MustText(t, "ns hosts")
	for _, h := range []string{head, peer} {
		if !strings.Contains(hosts, h) {
			t.Errorf("ns hosts = %q, missing %v", hosts, h)
		}
	}
}

// testTarFilesystem checks what the tar: prefix left in the VM configuration
// on the node that ran it.
func testTarFilesystem(t *testing.T) {
	want := filepath.Join(harness.MMFiles, "live", "containerfs")

	if got := strings.TrimSpace(harness.MustText(t, "vm config filesystem")); got != want {
		t.Errorf("vm config filesystem = %q, want %q", got, want)
	}

	if _, err := os.Stat(filepath.Join(want, "bin", "busybox")); err != nil {
		t.Errorf("filesystem not extracted on %v: %v", head, err)
	}
}

func testPlacement(t *testing.T) {
	vms := harness.VMInfo(t)

	for name, host := range placement {
		vm, ok := vms[name]
		if !ok {
			t.Errorf("%v missing from vm info", name)
			continue
		}
		if vm["host"] != host {
			t.Errorf("%v on %q, want %q", name, vm["host"], host)
		}
		if vm["type"] != "container" {
			t.Errorf("%v has type %q, want container", name, vm["type"])
		}
	}

	if len(vms) != len(placement) {
		t.Errorf("vm info has %v VMs, want %v: %v", len(vms), len(placement), harness.Keys(vms))
	}
}

// testPeerExtracted checks that the peer, which started with an empty files
// directory, fetched the tarball via iomeshage and extracted it itself.
func testPeerExtracted(t *testing.T) {
	if peerFiles == "" {
		t.Skip("LIVE_PEER_FILES not set")
	}

	for _, p := range []string{
		filepath.Join("live", "containerfs.tar.gz"),
		filepath.Join("live", "containerfs", "bin", "busybox"),
	} {
		if _, err := os.Stat(filepath.Join(peerFiles, p)); err != nil {
			t.Errorf("%v: %v", peer, err)
		}
	}
}

func testRunning(t *testing.T) {
	harness.Eventually(t, time.Minute, 2*time.Second, func() error {
		vms := harness.VMInfo(t)

		var bad []string
		for name := range placement {
			if vm := vms[name]; vm["state"] != "RUNNING" {
				bad = append(bad, fmt.Sprintf("%v on %v is %v %v", name, vm["host"], vm["state"], vm["tags"]))
			}
		}

		if len(bad) > 0 {
			return fmt.Errorf("containers not running: %v", strings.Join(bad, "; "))
		}
		return nil
	})
}

func testClearNamespace(t *testing.T) {
	if _, err := harness.Raw("clear namespace " + harness.Namespace); err != nil {
		t.Fatalf("clear namespace: %v", err)
	}

	// clearing the namespace on the first node clears it on the peer too
	harness.Eventually(t, time.Minute, 2*time.Second, func() error {
		for _, cmd := range []string{"namespace", "mesh send all namespace"} {
			resps, err := harness.Raw(cmd)
			if err != nil {
				return err
			}
			for _, r := range harness.ToRows(resps) {
				if r["namespace"] == harness.Namespace {
					return fmt.Errorf("namespace %v still exists on %v", harness.Namespace, r["host"])
				}
			}
		}
		return nil
	})
}
