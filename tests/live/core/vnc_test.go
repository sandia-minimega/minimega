// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
)

// The VNC phases type into the shell on the VGA console (tty1) of vncVM and
// check the effect with cc. No other phase uses vncVM.
const vncVM = "host-d3"

// vncRows returns the "vnc" rows (recordings and playbacks) of vm.
func vncRows(t *testing.T, vm string) []harness.Row {
	t.Helper()

	var rows []harness.Row
	for _, r := range harness.MustTable(t, "vnc") {
		if r["name"] == vm {
			rows = append(rows, r)
		}
	}
	return rows
}

// clearVNC stops the recordings and playbacks that a failed phase left.
func clearVNC(t *testing.T) {
	t.Cleanup(func() {
		if _, err := harness.Run("clear vnc"); err != nil {
			t.Logf("clear vnc: %v", err)
		}
	})
}

// guestFile waits until the shell on the console of vm made path.
func guestFile(t *testing.T, vm, path string) {
	t.Helper()
	harness.CCEventually(t, vm, "test -e "+path, 2*time.Minute)
}

// writeKB writes a kb file to the files directory and returns its path
// relative to the files directory.
func writeKB(t *testing.T, name, content string) string {
	t.Helper()

	rel := filepath.Join("live", name)
	if err := os.WriteFile(filepath.Join(harness.MMFiles, rel), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return rel
}

// typeKB returns kb file lines that type text.
func typeKB(t *testing.T, text string) string {
	t.Helper()

	kb, err := harness.KBFile(text, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return kb
}

// testVNCShim connects through miniweb and the VNC shim the way the browser
// console does, with the noVNC encodings, and reads a frame from QEMU. QEMU
// sends a ServerCutText with its clipboard capabilities when SetEncodings
// has ExtendedClipboard, so no ServerCutText shows that the shim removed it.
func testVNCShim(t *testing.T) {
	v, err := harness.DialVNC(vncVM)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	if v.Width == 0 || v.Height == 0 {
		t.Fatalf("server init size %dx%d", v.Width, v.Height)
	}
	t.Logf("QEMU desktop %q is %dx%d", v.Name, v.Width, v.Height)

	if err := v.SetEncodings(harness.EncodingRaw, harness.EncodingDesktopSize, harness.EncodingQEMUExtendedKey, harness.EncodingExtendedClipboard); err != nil {
		t.Fatal(err)
	}
	if err := v.RequestUpdate(false); err != nil {
		t.Fatal(err)
	}

	pixels := 0
	for pixels == 0 {
		u, err := v.ReadUpdate(30 * time.Second)
		if err != nil {
			t.Fatalf("read update: %v", err)
		}
		pixels += u.Pixels
		if u.CutTexts > 0 {
			t.Errorf("QEMU sent %d ServerCutText messages, so the shim did not remove ExtendedClipboard", u.CutTexts)
		}
		if pixels > 0 && u.Colors < 2 {
			t.Errorf("frame has %d colors, want a console with text", u.Colors)
		}
	}
}

// testVNCKeyboard types on the console through the shim with QEMU extended
// key events and records the keys, plays the recording back, and pastes text
// that the shim types as keys.
func testVNCKeyboard(t *testing.T) {
	clearVNC(t)
	harness.CCOK(t, vncVM, "rm -f /tmp/vnc-kb /tmp/vnc-paste")

	host, path := harness.Artifact(t, vncVM+".kb")
	harness.MustRun(t, fmt.Sprintf("vnc record kb %v %v", vncVM, path))

	v, err := harness.DialVNC(vncVM)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if err := v.SetEncodings(harness.EncodingRaw, harness.EncodingDesktopSize, harness.EncodingQEMUExtendedKey); err != nil {
		t.Fatal(err)
	}
	if err := v.TypeText("\ntouch /tmp/vnc-kb\n", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	guestFile(t, vncVM, "/tmp/vnc-kb")

	harness.MustRun(t, "vnc stop kb "+vncVM)
	b, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	kb := string(b)
	for _, want := range []string{":KeyEvent,true,t\n", ":KeyEvent,false,slash\n", ":KeyEvent,true,Return\n"} {
		if !strings.Contains(kb, want) {
			t.Errorf("kb recording has no %q line:\n%s", want, kb)
		}
	}
	harness.Keep(t, host)

	// the recording types the same command again
	harness.CCOK(t, vncVM, "rm /tmp/vnc-kb")
	harness.MustRun(t, fmt.Sprintf("vnc play %v %v", vncVM, path))
	guestFile(t, vncVM, "/tmp/vnc-kb")
	harness.Eventually(t, time.Minute, time.Second, func() error {
		if rows := vncRows(t, vncVM); len(rows) != 0 {
			return fmt.Errorf("playback still listed: %v", rows)
		}
		return nil
	})

	// with bidirectional-copy-paste false, the shim types a paste as keys
	if err := v.CutText("touch /tmp/vnc-paste\n"); err != nil {
		t.Fatal(err)
	}
	guestFile(t, vncVM, "/tmp/vnc-paste")
}

// testVNCType types with "vnc type" and "vnc inject" with no playback.
func testVNCType(t *testing.T) {
	clearVNC(t)
	harness.CCOK(t, vncVM, "rm -f /tmp/vnc-type")

	harness.MustRun(t, "vnc type "+vncVM+" touch /tmp/vnc-type")
	harness.Eventually(t, time.Minute, time.Second, func() error {
		if rows := vncRows(t, vncVM); len(rows) != 0 {
			return fmt.Errorf("vnc type still listed: %v", rows)
		}
		return nil
	})
	harness.MustRun(t, "vnc inject "+vncVM+" KeyEvent,true,Return")
	harness.MustRun(t, "vnc inject "+vncVM+" KeyEvent,false,Return")
	guestFile(t, vncVM, "/tmp/vnc-type")
}

// testVNCInjectLoadFile injects a LoadFile with no playback, with a path
// relative to the files directory, and into a running playback.
func testVNCInjectLoadFile(t *testing.T) {
	clearVNC(t)
	harness.CCOK(t, vncVM, "rm -f /tmp/vnc-load /tmp/vnc-nest")

	load := writeKB(t, "vnc-load.kb", typeKB(t, "\ntouch /tmp/vnc-load\n"))
	harness.MustRun(t, fmt.Sprintf("vnc inject %v LoadFile,%v", vncVM, load))
	guestFile(t, vncVM, "/tmp/vnc-load")

	// a playback that waits an hour, then releases a key that is not down
	wait := writeKB(t, "vnc-wait.kb", "3600s:KeyEvent,false,Shift_L\n")
	harness.MustRun(t, fmt.Sprintf("vnc play %v %v", vncVM, wait))
	nest := writeKB(t, "vnc-nest.kb", typeKB(t, "\ntouch /tmp/vnc-nest\n"))
	harness.MustRun(t, fmt.Sprintf("vnc inject %v LoadFile,%v", vncVM, filepath.Join(harness.MMFiles, nest)))
	guestFile(t, vncVM, "/tmp/vnc-nest")

	if rows := vncRows(t, vncVM); len(rows) != 1 || !strings.Contains(rows[0]["filename"], "vnc-wait.kb") {
		t.Errorf("after the nested file, vnc = %v; want the waiting playback", rows)
	}
	harness.MustRun(t, "vnc stop "+vncVM)
	harness.Eventually(t, 30*time.Second, time.Second, func() error {
		if rows := vncRows(t, vncVM); len(rows) != 0 {
			return fmt.Errorf("playback still listed after stop: %v", rows)
		}
		return nil
	})
}

// testVNCRecordFB records the framebuffer and decodes the recording.
func testVNCRecordFB(t *testing.T) {
	clearVNC(t)

	host, path := harness.Artifact(t, "host-a1.fb")
	harness.MustRun(t, "vnc record fb host-a1 "+path)
	if rows := vncRows(t, "host-a1"); len(rows) != 1 || !strings.Contains(rows[0]["filename"], "host-a1.fb") {
		t.Errorf("vnc does not list the recording: %v", harness.MustTable(t, "vnc"))
	}

	// the gzip writer writes the frames out as they fill its buffer
	harness.Eventually(t, time.Minute, time.Second, func() error {
		rec, err := harness.ReadFBRecording(host)
		if err != nil {
			return err
		}
		if rec.Updates < 2 {
			return fmt.Errorf("recording has %d complete updates so far", rec.Updates)
		}
		return nil
	})
	harness.MustRun(t, "vnc stop fb host-a1")

	rec, err := harness.ReadFBRecording(host)
	if err != nil {
		t.Fatalf("decode %v: %v", host, err)
	}
	t.Logf("recording: %+v", rec)
	if !rec.Closed {
		t.Errorf("the gzip stream is not complete after vnc stop fb")
	}
	if rec.Updates < 2 || rec.Colors < 2 {
		t.Errorf("recording has %d updates with %d colors, want 2 or more of each", rec.Updates, rec.Colors)
	}
	harness.Keep(t, host)
}

// openFiles returns the files that minimega has open.
func openFiles(t *testing.T) string {
	t.Helper()

	out, err := harness.InContainer(t, "sh", "-c", "ls -l /proc/$(pgrep -o -x minimega)/fd")
	if err != nil {
		t.Fatalf("list the open files of minimega: %v: %s", err, out)
	}
	return out
}

// launchVNCVM launches a VM with no network from the saved configuration and
// waits until it runs.
func launchVNCVM(t *testing.T, name string) {
	t.Helper()

	harness.MustRun(t, "vm config restore live-base")
	harness.MustRun(t, "vm config append live.role=host live.hostname="+name+" live.dhcp=0")
	harness.MustRun(t, "vm launch kvm "+name)
	harness.MustRun(t, "vm launch")
	harness.MustRun(t, "clear vm config")
	harness.MustRun(t, "vm start "+name)
	harness.Eventually(t, time.Minute, 2*time.Second, func() error {
		if s := harness.VMInfo(t)[name]["state"]; s != "RUNNING" {
			return fmt.Errorf("%v state = %v", name, s)
		}
		return nil
	})
}

// killVNCVM kills and flushes a VM.
func killVNCVM(t *testing.T, name string) {
	t.Helper()

	harness.MustRun(t, "vm kill "+name)
	harness.Eventually(t, 30*time.Second, time.Second, func() error {
		if s := harness.VMInfo(t)[name]["state"]; s != "QUIT" {
			return fmt.Errorf("%v state = %v", name, s)
		}
		return nil
	})
}

// testVNCKill kills a VM while it records and plays. The kill stops the
// recordings and the playback and closes their files, and a VM that is
// launched again with the same name can record.
func testVNCKill(t *testing.T) {
	const vm = "vnc-kill"
	clearVNC(t)

	launchVNCVM(t, vm)
	kbHost, kbPath := harness.Artifact(t, vm+".kb")
	fbHost, fbPath := harness.Artifact(t, vm+".fb")
	wait := writeKB(t, "vnc-kill-wait.kb", "3600s:KeyEvent,false,Shift_L\n")
	harness.MustRun(t, fmt.Sprintf("vnc record kb %v %v", vm, kbPath))
	harness.MustRun(t, fmt.Sprintf("vnc record fb %v %v", vm, fbPath))
	harness.MustRun(t, fmt.Sprintf("vnc play %v %v", vm, wait))
	if rows := vncRows(t, vm); len(rows) != 3 {
		t.Fatalf("vnc lists %d entries for %v, want 3: %v", len(rows), vm, rows)
	}

	killVNCVM(t, vm)
	if rows := vncRows(t, vm); len(rows) != 0 {
		t.Errorf("vnc entries after the kill: %v", rows)
	}
	if fds := openFiles(t); strings.Contains(fds, kbHost) || strings.Contains(fds, fbHost) {
		t.Errorf("minimega has the recordings open after the kill:\n%s", fds)
	}
	if rec, err := harness.ReadFBRecording(fbHost); err != nil || !rec.Closed {
		t.Errorf("fb recording after the kill: %+v, %v; want a complete gzip stream", rec, err)
	}

	harness.MustRun(t, "vm flush "+vm)
	launchVNCVM(t, vm)
	harness.MustRun(t, fmt.Sprintf("vnc record kb %v %v", vm, kbPath))
	if rows := vncRows(t, vm); len(rows) != 1 {
		t.Errorf("vnc lists %v for the launched again VM, want the new recording", rows)
	}
	killVNCVM(t, vm)
	harness.MustRun(t, "vm flush "+vm)
}
