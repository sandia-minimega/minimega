// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

// requireNetAdmin skips the test unless it runs as root and can create a tap.
func requireNetAdmin(tb testing.TB) {
	tb.Helper()

	if os.Geteuid() != 0 {
		tb.Skip("requires root with CAP_NET_ADMIN")
	}

	const probe = "mmtprobe"
	if err := addTuntap(probe); err != nil {
		tb.Skipf("requires CAP_NET_ADMIN and /dev/net/tun: %v", err)
	}
	if err := destroyTap(probe); err != nil {
		tb.Fatalf("destroy probe tap: %v", err)
	}
}

// cleanupLinks deletes the links after the test, if they still exist.
func cleanupLinks(tb testing.TB, names ...string) {
	tb.Cleanup(func() {
		for _, name := range names {
			if link, err := netlink.LinkByName(name); err == nil {
				netlink.LinkDel(link)
			}
		}
	})
}

func linkExists(tb testing.TB, name string) bool {
	tb.Helper()

	_, err := netlink.LinkByName(name)
	if err != nil && !isNoDevice(err) {
		tb.Fatalf("LinkByName(%v): %v", name, err)
	}

	return err == nil
}

func sysfsAttr(t *testing.T, name, attr string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("/sys/class/net", name, attr))
	if err != nil {
		t.Fatalf("read %v of %v: %v", attr, name, err)
	}

	return strings.TrimSpace(string(b))
}

// ipDetails returns `ip -d link show` for name without the values that
// differ between two taps.
func ipDetails(t *testing.T, name string) string {
	t.Helper()

	out, err := exec.Command("ip", "-d", "link", "show", name).CombinedOutput()
	if err != nil {
		t.Fatalf("ip -d link show %v: %v: %s", name, err, out)
	}

	s := strings.ReplaceAll(string(out), name, "NAME")
	s = regexp.MustCompile(`^\d+:`).ReplaceAllString(s, "N:")
	s = regexp.MustCompile(`([0-9a-f]{2}:){5}[0-9a-f]{2}`).ReplaceAllString(s, "MAC")

	return s
}

func TestCreateTapMatchesIproute(t *testing.T) {
	requireNetAdmin(t)

	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("requires the ip command")
	}

	const ours, theirs = "mmtparity0", "mmtparity1"
	cleanupLinks(t, ours, theirs)

	if err := createTap(ours); err != nil {
		t.Fatalf("createTap: %v", err)
	}
	if out, err := exec.Command("ip", "tuntap", "add", "mode", "tap", theirs).CombinedOutput(); err != nil {
		t.Fatalf("ip tuntap add: %v: %s", err, out)
	}

	for _, attr := range []string{"tun_flags", "owner", "group", "type", "flags", "tx_queue_len"} {
		got, want := sysfsAttr(t, ours, attr), sysfsAttr(t, theirs, attr)
		t.Logf("%v: createTap=%v ip=%v", attr, got, want)

		if got != want {
			t.Errorf("%v: createTap gives %v, ip tuntap add gives %v", attr, got, want)
		}
	}

	got, want := ipDetails(t, ours), ipDetails(t, theirs)
	t.Logf("ip -d link show:\n%s", got)

	if got != want {
		t.Errorf("ip -d link show differs:\ncreateTap:\n%s\nip tuntap add:\n%s", got, want)
	}
	if !strings.Contains(got, "persist on") {
		t.Errorf("tap does not persist after its fd closes:\n%s", got)
	}
}

func TestCreateTapPersistsAfterProcessExits(t *testing.T) {
	requireNetAdmin(t)

	const name = "mmtpersist"
	cleanupLinks(t, name)

	// create the tap in a child process that exits at once
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperCreateTap$")
	cmd.Env = append(os.Environ(), "MM_TEST_CREATE_TAP="+name)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child process: %v: %s", err, out)
	}

	if !linkExists(t, name) {
		t.Fatal("tap is gone after the creating process exited")
	}
}

// TestHelperCreateTap creates a tap for TestCreateTapPersistsAfterProcessExits.
func TestHelperCreateTap(t *testing.T) {
	name := os.Getenv("MM_TEST_CREATE_TAP")
	if name == "" {
		t.Skip("helper process only")
	}

	if err := createTap(name); err != nil {
		t.Fatalf("createTap: %v", err)
	}
}

func TestCreateUpDestroyTap(t *testing.T) {
	requireNetAdmin(t)

	const name = "mmtlife"
	cleanupLinks(t, name)

	if err := createTap(name); err != nil {
		t.Fatalf("createTap: %v", err)
	}

	if err := createTap(name); !errors.Is(err, errAlreadyExists) {
		t.Errorf("second createTap: got %v, want %v", err, errAlreadyExists)
	}

	if err := upInterface(name, true); err != nil {
		t.Fatalf("upInterface: %v", err)
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatalf("LinkByName: %v", err)
	}
	if attrs := link.Attrs(); attrs.Flags&net.FlagUp == 0 || attrs.Promisc == 0 {
		t.Errorf("after upInterface: flags %v promisc %v, want up and promisc", attrs.Flags, attrs.Promisc)
	}

	if err := downInterface(name); err != nil {
		t.Fatalf("downInterface: %v", err)
	}

	link, err = netlink.LinkByName(name)
	if err != nil {
		t.Fatalf("LinkByName: %v", err)
	}
	if link.Attrs().Flags&net.FlagUp != 0 {
		t.Errorf("after downInterface: flags %v, want down", link.Attrs().Flags)
	}

	if err := destroyTap(name); err != nil {
		t.Fatalf("destroyTap: %v", err)
	}
	if linkExists(t, name) {
		t.Error("tap exists after destroyTap")
	}

	if err := destroyTap(name); !isNoDevice(err) {
		t.Errorf("destroyTap of a missing tap: got %v, want ENODEV", err)
	}
}

func TestDestroyTapsGroup(t *testing.T) {
	requireNetAdmin(t)

	ours := []string{"mmtgrp0", "mmtgrp1", "mmtgrp2"}
	const other = "mmtother"
	const otherGroup = 7

	cleanupLinks(t, append(ours, other)...)

	for _, name := range append(ours, other) {
		if err := createTap(name); err != nil {
			t.Fatalf("createTap(%v): %v", name, err)
		}
	}

	link, err := netlink.LinkByName(other)
	if err != nil {
		t.Fatalf("LinkByName(%v): %v", other, err)
	}
	if err := netlink.LinkSetGroup(link, otherGroup); err != nil {
		t.Fatalf("LinkSetGroup(%v): %v", other, err)
	}

	// a name that does not exist counts as destroyed
	gone, err := destroyTaps(append(ours, "mmtmissing"))
	if err != nil {
		t.Fatalf("destroyTaps: %v", err)
	}
	if len(gone) != len(ours)+1 {
		t.Errorf("destroyTaps gone: got %v, want %v and mmtmissing", gone, ours)
	}

	for _, name := range ours {
		if linkExists(t, name) {
			t.Errorf("%v exists after destroyTaps", name)
		}
	}

	link, err = netlink.LinkByName(other)
	if err != nil {
		t.Fatalf("%v is gone after destroyTaps: %v", other, err)
	}
	if link.Attrs().Group != otherGroup {
		t.Errorf("%v group: got %v, want %v", other, link.Attrs().Group, otherGroup)
	}
}

func TestDestroyTapsForeignLinkInGroup(t *testing.T) {
	requireNetAdmin(t)

	ours := []string{"mmtown0", "mmtown1"}
	const foreign = "mmtforeign"

	cleanupLinks(t, append(ours, foreign)...)

	for _, name := range append(ours, foreign) {
		if err := createTap(name); err != nil {
			t.Fatalf("createTap(%v): %v", name, err)
		}
	}

	link, err := netlink.LinkByName(foreign)
	if err != nil {
		t.Fatalf("LinkByName(%v): %v", foreign, err)
	}
	if err := netlink.LinkSetGroup(link, reapGroup); err != nil {
		t.Fatalf("LinkSetGroup(%v): %v", foreign, err)
	}

	gone, err := destroyTaps(ours)
	if err != nil {
		t.Fatalf("destroyTaps: %v", err)
	}
	if len(gone) != len(ours) {
		t.Errorf("destroyTaps gone: got %v, want %v", gone, ours)
	}

	for _, name := range ours {
		if linkExists(t, name) {
			t.Errorf("%v exists after destroyTaps", name)
		}
	}
	if !linkExists(t, foreign) {
		t.Errorf("destroyTaps deleted %v, which it does not own", foreign)
	}
}

// benchTaps is the number of taps that each benchmark iteration tears down.
const benchTaps = 500

func createBenchTaps(b *testing.B) []string {
	b.Helper()

	names := make([]string, benchTaps)
	for i := range names {
		names[i] = fmt.Sprintf("mmtbench%d", i)

		if err := addTuntap(names[i]); err != nil {
			b.Fatalf("addTuntap(%v): %v", names[i], err)
		}
	}

	cleanupLinks(b, names...)

	return names
}

// BenchmarkTapTeardown compares one LinkDel for each tap with destroyTaps,
// which deletes all of the taps with one RTM_DELLINK message.
func BenchmarkTapTeardown(b *testing.B) {
	requireNetAdmin(b)

	b.Run(fmt.Sprintf("LinkDel/%d", benchTaps), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			names := createBenchTaps(b)
			b.StartTimer()

			for _, name := range names {
				link, err := netlink.LinkByName(name)
				if err == nil {
					err = netlink.LinkDel(link)
				}
				if err != nil {
					b.Fatalf("delete %v: %v", name, err)
				}
			}
		}
	})

	b.Run(fmt.Sprintf("Group/%d", benchTaps), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			names := createBenchTaps(b)
			b.StartTimer()

			if gone, err := destroyTaps(names); err != nil || len(gone) != len(names) {
				b.Fatalf("destroyTaps: %v taps gone, err %v", len(gone), err)
			}
		}
	})
}
