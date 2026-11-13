// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeKB(t *testing.T, path, lines string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newTestPlayer returns a Player with base and a paste directory that the
// test can check. Cleanup stops all playbacks.
func newTestPlayer(t *testing.T, base string) *Player {
	t.Helper()

	p := NewPlayer(base)
	p.tmpDir = t.TempDir()
	t.Cleanup(p.Clear)

	return p
}

// pasteFiles returns the names of the paste files of p.
func pasteFiles(t testing.TB, p *Player) []string {
	t.Helper()

	entries, err := os.ReadDir(p.tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var res []string
	for _, e := range entries {
		res = append(res, e.Name())
	}
	return res
}

// keyName returns the kb name of a key event, with "+" for a press and "-"
// for a release.
func keyName(k *KeyEvent) string {
	name, err := xKeysymToString(k.Key)
	if err != nil {
		name = err.Error()
	}
	if k.DownFlag != 0 {
		return "+" + name
	}
	return "-" + name
}

// T11: an injected LoadFile with no playback starts a playback, and a
// relative path resolves against the base directory.
func TestPlayerInjectLoadFileNoPlayback(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	base := t.TempDir()
	writeKB(t, filepath.Join(base, "steps.kb"), "0:KeyEvent,true,a\n")

	p := newTestPlayer(t, base)
	if err := within(t, "Inject", func() error { return p.Inject("vm", addr, "LoadFile,steps.kb") }); err != nil {
		t.Fatal(err)
	}
	if k := nextKey(t, f); k.Key != 'a' {
		t.Fatalf("server got %v, want a", k)
	}
}

// T11: an injected LoadFile with a running playback plays the file. A
// relative path resolves against the base directory.
func TestPlayerInjectLoadFileRunning(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	base := t.TempDir()
	writeKB(t, filepath.Join(base, "long.kb"), "3600s:KeyEvent,true,a\n")
	writeKB(t, filepath.Join(base, "nested.kb"), "0:KeyEvent,true,b\n")

	p := newTestPlayer(t, base)
	if err := p.Playback("vm", addr, filepath.Join(base, "long.kb")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "first step", func() bool {
		s, _ := p.GetStep("vm")
		return s != ""
	})

	if err := within(t, "Inject", func() error { return p.Inject("vm", addr, "LoadFile,nested.kb") }); err != nil {
		t.Fatal(err)
	}
	if k := nextKey(t, f); k.Key != 'b' {
		t.Fatalf("server got %v, want b", k)
	}
	if len(p.Info()) != 1 {
		t.Fatalf("info = %q, want the running playback", p.Info())
	}

	if err := within(t, "Inject", func() error { return p.Inject("vm", addr, "LoadFile,missing.kb") }); err == nil {
		t.Error("Inject of a missing file gave no error")
	}
}

// T14: a failed Start leaves no playback and no open connection. A missing
// file gives an error.
func TestPlayerPlaybackMissingFile(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := newTestPlayer(t, "")
	err := within(t, "Playback", func() error {
		return p.Playback("vm", addr, filepath.Join(t.TempDir(), "missing.kb"))
	})
	if err == nil {
		t.Fatal("Playback of a missing file gave no error")
	}
	if info := p.Info(); len(info) != 0 {
		t.Errorf("info = %q, want no playback", info)
	}
	waitUntil(t, "no open connection", func() bool { return f.active.Load() == 0 })
}

// T19: paste types printable ASCII, Tab, and Enter, skips other characters,
// and logs one warning with the count. The pasted text is not logged.
func TestPlayerPasteKeys(t *testing.T) {
	logs := captureLog(t)

	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := newTestPlayer(t, "")

	// "café" is what the shim gives for the Latin-1 bytes "caf\xe9".
	text := "aB~\t\r\nx\ny\r" + latin1([]byte("caf\xe9")) + "€\x01"
	if err := within(t, "Paste", func() error { return p.Paste("vm-paste", addr, text) }); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"+a", "-a",
		"+Shift_L", "+B", "-B", "-Shift_L",
		"+Shift_L", "+asciitilde", "-asciitilde", "-Shift_L",
		"+Tab", "-Tab",
		"+Return", "-Return",
		"+x", "-x",
		"+Return", "-Return",
		"+y", "-y",
		"+Return", "-Return",
		"+c", "-c", "+a", "-a", "+f", "-f",
	}
	var got []string
	for range want {
		got = append(got, keyName(nextKey(t, f)))
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("keys = %q\nwant   %q", got, want)
	}

	recv(t, f.ended, "paste connection end")
	waitUntil(t, "paste file removal", func() bool { return len(pasteFiles(t, p)) == 0 })

	if l := logs.lines("WARN", "vm-paste", "skipped 3 characters"); len(l) != 1 {
		t.Errorf("got %d skip warnings, want 1: %q", len(l), logs.lines("vm-paste"))
	}
	if l := logs.lines("DEBUG", "vm-paste", "paste of 16 characters"); len(l) != 1 {
		t.Errorf("paste length is not logged: %q", logs.lines("vm-paste"))
	}
	for _, s := range []string{"aB~", "caf", "KeyEvent"} {
		if l := logs.lines(s); len(l) != 0 {
			t.Errorf("log has the pasted text %q: %q", s, l)
		}
	}
}

// T19: pastes to one VM are typed in order, and the events are paced.
func TestPlayerPasteQueue(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	keys := make(chan time.Time, 64)
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*KeyEvent); ok {
			keys <- time.Now()
		}
		return nil
	}
	addr := f.listen(t)

	p := newTestPlayer(t, "")
	for _, s := range []string{"ab", "cd", "ef"} {
		if err := within(t, "Paste", func() error { return p.Paste("vm", addr, s) }); err != nil {
			t.Fatalf("paste %q: %v", s, err)
		}
	}

	var got []string
	for i := 0; i < 12; i++ {
		got = append(got, keyName(nextKey(t, f)))
	}
	if want := "+a -a +b -b +c -c +d -d +e -e +f -f"; strings.Join(got, " ") != want {
		t.Errorf("keys = %q, want %q", got, want)
	}

	first := recv(t, keys, "key time")
	var last time.Time
	for i := 1; i < 12; i++ {
		last = recv(t, keys, "key time")
	}
	if d, min := last.Sub(first), 10*pasteKeyDelay; d < min {
		t.Errorf("12 key events took %v, want at least %v", d, min)
	}

	waitUntil(t, "paste file removal", func() bool { return len(pasteFiles(t, p)) == 0 })
}

// T19: the paste file is removed when the playback fails, when it is
// stopped, and when the queue is cleared.
func TestPlayerPasteFileRemoved(t *testing.T) {
	t.Run("dial failure", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()

		p := newTestPlayer(t, "")
		if err := within(t, "Paste", func() error { return p.Paste("vm", addr, "abc") }); err == nil {
			t.Error("Paste to a closed port gave no error")
		}
		waitUntil(t, "paste file removal", func() bool { return len(pasteFiles(t, p)) == 0 })
		if info := p.Info(); len(info) != 0 {
			t.Errorf("info = %q, want no playback", info)
		}
	})

	t.Run("stop", func(t *testing.T) {
		f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
		addr := f.listen(t)

		p := newTestPlayer(t, "")
		for i := 0; i < 3; i++ {
			if err := within(t, "Paste", func() error { return p.Paste("vm", addr, strings.Repeat("a", 500)) }); err != nil {
				t.Fatal(err)
			}
		}
		nextKey(t, f)

		if err := within(t, "Stop", func() error { return p.Stop("vm") }); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "paste file removal", func() bool { return len(pasteFiles(t, p)) == 0 })
		waitUntil(t, "no playback", func() bool { return len(p.Info()) == 0 })
		recv(t, f.ended, "paste connection end")
		if n := f.active.Load(); n != 0 {
			t.Errorf("%d open connections after Stop, want 0", n)
		}
	})

	t.Run("clear", func(t *testing.T) {
		f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
		addr := f.listen(t)

		p := newTestPlayer(t, "")
		for i := 0; i < 3; i++ {
			if err := within(t, "Paste", func() error { return p.Paste("vm", addr, strings.Repeat("a", 500)) }); err != nil {
				t.Fatal(err)
			}
		}
		nextKey(t, f)

		within(t, "Clear", func() error { p.Clear(); return nil })
		waitUntil(t, "paste file removal", func() bool { return len(pasteFiles(t, p)) == 0 })
		waitUntil(t, "no open connection", func() bool { return f.active.Load() == 0 })
	})

	t.Run("other playback", func(t *testing.T) {
		f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
		addr := f.listen(t)

		dir := t.TempDir()
		writeKB(t, filepath.Join(dir, "long.kb"), "3600s:KeyEvent,true,a\n")

		p := newTestPlayer(t, "")
		if err := p.Playback("vm", addr, filepath.Join(dir, "long.kb")); err != nil {
			t.Fatal(err)
		}
		if err := within(t, "Paste", func() error { return p.Paste("vm", addr, "abc") }); err == nil {
			t.Error("Paste during a kb playback gave no error")
		}
		if files := pasteFiles(t, p); len(files) != 0 {
			t.Errorf("paste files = %q, want none", files)
		}
	})
}

// A paste above maxPasteChars gives an error and writes no file. A paste at
// the limit starts.
func TestPlayerPasteCharLimit(t *testing.T) {
	if maxPasteChars < 10000 {
		t.Fatalf("maxPasteChars = %d, want at least 10000", maxPasteChars)
	}

	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := newTestPlayer(t, "")
	err := within(t, "Paste", func() error { return p.Paste("vm", addr, strings.Repeat("a", maxPasteChars+1)) })
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("Paste of %d characters gave %v, want a limit error", maxPasteChars+1, err)
	}
	if files := pasteFiles(t, p); len(files) != 0 {
		t.Errorf("paste files = %q, want none", files)
	}
	if n := f.active.Load(); n != 0 {
		t.Errorf("%d open connections, want 0", n)
	}

	if err := within(t, "Paste", func() error { return p.Paste("vm", addr, strings.Repeat("a", maxPasteChars)) }); err != nil {
		t.Errorf("Paste of %d characters: %v", maxPasteChars, err)
	}
}

// A VM has at most maxQueuedPastes pastes that wait. A paste above the cap
// gives an error and writes no file.
func TestPlayerPasteQueueLimit(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := newTestPlayer(t, "")
	for i := 0; i < maxQueuedPastes+1; i++ {
		if err := within(t, "Paste", func() error { return p.Paste("vm", addr, strings.Repeat("a", 500)) }); err != nil {
			t.Fatalf("paste %d: %v", i, err)
		}
	}

	err := within(t, "Paste", func() error { return p.Paste("vm", addr, "b") })
	if err == nil || !strings.Contains(err.Error(), "paste queue full") {
		t.Errorf("Paste to a full queue gave %v, want a queue full error", err)
	}
	if files := pasteFiles(t, p); len(files) != maxQueuedPastes+1 {
		t.Errorf("got %d paste files, want %d", len(files), maxQueuedPastes+1)
	}

	// another VM has its own queue
	if err := within(t, "Paste", func() error { return p.Paste("vm-2", addr, "c") }); err != nil {
		t.Errorf("paste to another VM: %v", err)
	}
}

// writePasteFile streams the kb lines. testdata/paste-sample.kb is the
// output of the earlier code, which joined a slice of lines.
func TestWritePasteFileSample(t *testing.T) {
	var ascii []byte
	for c := byte(0x20); c < 0x7f; c++ {
		ascii = append(ascii, c)
	}
	text := "aB~\t\r\nx\ny\r" + latin1([]byte("caf\xe9")) + "€\x01" + string(ascii) + "\r\n"

	want, err := os.ReadFile(filepath.Join("testdata", "paste-sample.kb"))
	if err != nil {
		t.Fatal(err)
	}

	name, typed, skipped, err := writePasteFile(t.TempDir(), text)
	if err != nil {
		t.Fatal(err)
	}
	if typed != 108 || skipped != 3 {
		t.Errorf("typed, skipped = %d, %d; want 108, 3", typed, skipped)
	}

	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("paste file differs from testdata/paste-sample.kb:\n%s", got)
	}

	dir := t.TempDir()
	if name, typed, _, err := writePasteFile(dir, "\x01€"); err != nil || name != "" || typed != 0 {
		t.Errorf("paste of no typed characters = %q, %d, %v; want no file", name, typed, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files after a paste of no typed characters: %v", entries)
	}
}
