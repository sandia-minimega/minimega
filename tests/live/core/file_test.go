// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
)

// A second minimega node, started inside the container in its own network
// and UTS namespace, with its files directory on a small tmpfs.
const (
	peerName   = "minimega-peer"
	peerNetns  = "mmpeer"
	peerHostIP = "192.168.254.1"
	peerIP     = "192.168.254.2"

	// free space left on the peer once the guest kernel and initrd are there
	peerSpare = 48 << 20
)

// peerBase is the peer's -base, beside the main node's so its log is visible on the host.
var peerBase = filepath.Join(filepath.Dir(harness.MMFiles), "peer")

// testFileTransfer joins a peer to the mesh and pulls files to it with
// iomeshage: a guest kernel and initrd through "vm launch", a file that fits
// once but not twice (issue #1388), and one that does not fit at all.
func testFileTransfer(t *testing.T) {
	kernel := statSize(t, filepath.Join(harness.MMFiles, "live", "live.kernel"))
	initrd := statSize(t, filepath.Join(harness.MMFiles, "live", "live.initrd"))

	startPeer(t, kernel+initrd+peerSpare)
	t.Cleanup(func() { os.RemoveAll(filepath.Join(harness.MMFiles, "live", "xfer")) })

	t.Run("LaunchVM", func(t *testing.T) {
		// the peer has neither file, so launching pulls both over the mesh
		peerRun(t, "clear vm config")
		if q := os.Getenv("LIVE_QEMU"); q != "" {
			peerRun(t, "vm config qemu "+q)
		}
		if c := os.Getenv("LIVE_CPU"); c != "" {
			peerRun(t, "vm config cpu "+c)
		}
		peerRun(t, "vm config kernel live/live.kernel")
		peerRun(t, "vm config initrd live/live.initrd")
		peerRun(t, "vm config memory 384")
		peerRun(t, "vm config snapshot true")
		peerRun(t, "vm launch kvm xfer-vm")
		peerRun(t, "vm start all")

		harness.Eventually(t, 2*time.Minute, 2*time.Second, func() error {
			rows, err := peerTable(t, "vm info")
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0]["state"] != "RUNNING" {
				return fmt.Errorf("peer vm info: %v", rows)
			}
			return nil
		})

		for _, f := range []string{"live/live.kernel", "live/live.initrd"} {
			want := hostSum(t, filepath.Join(harness.MMFiles, f))
			if got := peerSum(t, f); got != want {
				t.Errorf("%v: peer sha256 %v, want %v", f, got, want)
			}
		}

		peerRun(t, "vm kill all")
		peerRun(t, "vm flush")
	})

	t.Run("FitsOnce", func(t *testing.T) {
		// the old reassembly needed room for the parts and the whole file
		// and silently truncated it; this file only fits once
		name := "live/xfer/fits.img"
		writeRandom(t, filepath.Join(harness.MMFiles, name), peerSpare*5/8+4321)

		if err := meshGet(name); err != nil {
			t.Fatal(err)
		}
		waitPeerTransfers(t)

		want := hostSum(t, filepath.Join(harness.MMFiles, name))
		if got := peerSum(t, name); got != want {
			t.Errorf("peer sha256 %v, want %v", got, want)
		}
	})

	t.Run("NoSpace", func(t *testing.T) {
		name := "live/xfer/huge.img"
		path := filepath.Join(harness.MMFiles, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// sparse, so it costs the host nothing
		err = f.Truncate(2 * peerSpare)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}

		err = meshGet(name)
		if err == nil || !strings.Contains(err.Error(), "no space left on device") {
			t.Fatalf("file get of a file larger than the disk: got %v, want a no space error", err)
		}

		// nothing is left behind and nothing is in flight
		waitPeerTransfers(t)
		out, err := harness.InContainer(t, "ls", "-A", filepath.Join(peerBase, "files"))
		if err != nil {
			t.Fatalf("listing peer files: %v: %s", err, out)
		}
		for _, f := range strings.Fields(out) {
			if strings.HasPrefix(f, "transfer_") {
				t.Errorf("leftover transfer directory on the peer: %v", f)
			}
		}
		if rows, err := peerTable(t, "file list "+name); err != nil || len(rows) > 0 {
			t.Errorf("partial file left on the peer: %v (%v)", rows, err)
		}
	})
}

// startPeer starts the peer with a tmpfs of size bytes and joins it to the mesh.
func startPeer(t *testing.T, size int64) {
	t.Helper()

	// paths go through the environment so that the script needs no quoting
	script := fmt.Sprintf(`set -eu
ip netns add %[1]s
ip link add %[1]s0 type veth peer name %[1]s1 netns %[1]s
ip addr add %[2]s/30 dev %[1]s0
ip link set %[1]s0 up
ip -n %[1]s addr add %[3]s/30 dev %[1]s1
ip -n %[1]s link set %[1]s1 up
ip -n %[1]s link set lo up
mkdir -p "$PEER_BASE/files"
mount -t tmpfs -o size=%[4]d tmpfs "$PEER_BASE/files"
setsid ip netns exec %[1]s unshare --uts sh -c 'hostname %[5]s && exec minimega -nostdin -force -degree 0 -base "$PEER_BASE" -filepath "$PEER_BASE/files" -context live-peer -logfile "$PEER_BASE/peer-minimega.log" -level info' </dev/null >"$PEER_BASE/peer-stdout.log" 2>&1 &
`, peerNetns, peerHostIP, peerIP, size, peerName)

	t.Cleanup(func() { stopPeer(t) })

	if out, err := peerScript(t, script); err != nil {
		t.Fatalf("starting peer: %v: %s", err, out)
	}

	harness.Eventually(t, time.Minute, time.Second, func() error {
		out, err := harness.InContainer(t, "minimega", "-base", peerBase, "-e", "version")
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		return nil
	})

	if _, err := harness.Raw("mesh dial " + peerIP); err != nil {
		t.Fatalf("mesh dial: %v", err)
	}
	harness.Eventually(t, time.Minute, time.Second, func() error {
		resps, err := harness.Raw("mesh list")
		if err != nil {
			return err
		}
		for _, r := range resps {
			if strings.Contains(r.Response, peerName) {
				return nil
			}
		}
		return fmt.Errorf("%v not in mesh list: %v", peerName, resps)
	})
}

// stopPeer removes the peer and everything startPeer created.
func stopPeer(t *testing.T) {
	harness.Raw("mesh hangup " + peerName)

	if t.Failed() {
		harness.Keep(t, filepath.Join(peerBase, "peer-minimega.log"))
		harness.Keep(t, filepath.Join(peerBase, "peer-stdout.log"))
	}

	// qemu outlives a killed minimega, and a stray one fails ClearNamespace
	script := fmt.Sprintf(`
pid=$(cat "$PEER_BASE/minimega.pid" 2>/dev/null)
minimega -base "$PEER_BASE" -e vm kill all >/dev/null 2>&1
minimega -base "$PEER_BASE" -e quit >/dev/null 2>&1
for i in $(seq 1 20); do [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
[ -n "$pid" ] && kill -9 "$pid" 2>/dev/null
pkill -9 -f "qmp unix:$PEER_BASE/"
umount "$PEER_BASE/files"
ip netns del %[1]s
rm -rf "$PEER_BASE"
true
`, peerNetns)
	if out, err := peerScript(t, script); err != nil {
		t.Logf("stopping peer: %v: %s", err, out)
	}

	// broadcasts fail with "no such client" until the next MSA drops the peer from the mesh
	harness.Eventually(t, time.Minute, time.Second, func() error {
		_, err := harness.Raw("mesh send all version")
		return err
	})
}

// peerScript runs a bash script in the container with PEER_BASE set.
func peerScript(t *testing.T, script string) (string, error) {
	t.Helper()
	return harness.InContainer(t, "env", "PEER_BASE="+peerBase, "bash", "-c", script)
}

// peerTable runs a command on the peer through its own command socket.
func peerTable(t *testing.T, cmd string) ([]harness.Row, error) {
	t.Helper()
	// the container's MM_LOGLEVEL makes the client log too, so keep only the JSON lines
	out, err := harness.InContainer(t, "minimega", "-level", "error", "-base", peerBase, "-e", ".json true "+cmd)
	if err != nil {
		return nil, fmt.Errorf("%q: %v: %s", cmd, err, out)
	}

	var all minicli.Responses
	var errs []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "[") {
			continue
		}
		var resps minicli.Responses
		if err := json.Unmarshal([]byte(line), &resps); err != nil {
			return nil, fmt.Errorf("%q: %v: %s", cmd, err, out)
		}
		for _, r := range resps {
			if r.Error != "" {
				errs = append(errs, r.Error)
			}
		}
		all = append(all, resps...)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%q: %v", cmd, strings.Join(errs, "; "))
	}

	return harness.ToRows(all), nil
}

func peerRun(t *testing.T, cmd string) {
	t.Helper()
	if _, err := peerTable(t, cmd); err != nil {
		t.Fatalf("peer: %v", err)
	}
}

// meshGet asks the peer to fetch a file through the mesh, as "mesh send all file get" does.
func meshGet(name string) error {
	_, err := harness.Raw(fmt.Sprintf("mesh send %v file get %v", peerName, name))
	return err
}

// waitPeerTransfers waits until the peer has no file transfers in flight.
func waitPeerTransfers(t *testing.T) {
	t.Helper()
	harness.Eventually(t, 2*time.Minute, time.Second, func() error {
		rows, err := peerTable(t, "file status")
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return fmt.Errorf("transfers in flight: %v", rows)
		}
		return nil
	})
}

// peerSum returns the sha256 of a file in the peer's files directory.
func peerSum(t *testing.T, name string) string {
	t.Helper()
	out, err := harness.InContainer(t, "sha256sum", filepath.Join(peerBase, "files", name))
	if err != nil {
		t.Fatalf("sha256sum on the peer: %v: %s", err, out)
	}
	return strings.Fields(out)[0]
}

func hostSum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func statSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func writeRandom(t *testing.T, path string, n int) {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}
