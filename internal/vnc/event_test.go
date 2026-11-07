// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStringParse(t *testing.T) {
	vals := []fmt.Stringer{
		&KeyEvent{
			DownFlag: 0, Key: 0xff08,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0x0030,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0x0041,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0xffe1,
		},
		&PointerEvent{
			ButtonMask: 0, XPosition: 100, YPosition: 200,
		},
		&PointerEvent{
			ButtonMask: 0, XPosition: 105, YPosition: 200,
		},
		&PointerEvent{
			ButtonMask: 1, XPosition: 105, YPosition: 205,
		},
	}

	for _, want := range vals {
		got, err := parseEvent(want.String())

		if err != nil {
			t.Errorf("parse failed -- %v", err)
			continue
		}

		if s := got.(fmt.Stringer).String(); s != want.String() {
			t.Errorf("parse %q gives %q", want.String(), s)
		}

		var gotWire, wantWire bytes.Buffer
		if err := got.(Event).Write(&gotWire); err != nil {
			t.Fatal(err)
		}
		if err := want.(Event).Write(&wantWire); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotWire.Bytes(), wantWire.Bytes()) {
			t.Errorf("%v: wire bytes = %x, want %x", want, gotWire.Bytes(), wantWire.Bytes())
		}
	}
}

// T8: a 9-bit PointerEvent mask parses with bit size 16, and playback sends
// mask & 0x7f in a 6-byte PointerEvent.
func TestParsePointerEventMask(t *testing.T) {
	tests := []struct {
		line string
		wire []byte
	}{
		{"PointerEvent,0,1,2", []byte{TypePointerEvent, 0x00, 0, 1, 0, 2}},
		{"PointerEvent,127,1,2", []byte{TypePointerEvent, 0x7f, 0, 1, 0, 2}},
		{"PointerEvent,129,3,4", []byte{TypePointerEvent, 0x01, 0, 3, 0, 4}},
		{"PointerEvent,256,5,6", []byte{TypePointerEvent, 0x00, 0, 5, 0, 6}},
		{"PointerEvent,511,65535,0", []byte{TypePointerEvent, 0x7f, 0xff, 0xff, 0, 0}},
	}

	for _, tt := range tests {
		e, err := parseEvent(tt.line)
		if err != nil {
			t.Errorf("parse %q: %v", tt.line, err)
			continue
		}
		if s := e.(fmt.Stringer).String(); s != tt.line {
			t.Errorf("parse %q gives %q", tt.line, s)
		}

		var b bytes.Buffer
		if err := e.(Event).Write(&b); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b.Bytes(), tt.wire) {
			t.Errorf("%q: wire bytes = %x, want %x", tt.line, b.Bytes(), tt.wire)
		}
	}

	if _, err := parseEvent("PointerEvent,65536,1,2"); err == nil {
		t.Error("parse of a 17-bit mask gave no error")
	}
}

// T15: ClickIt and ClickItEvent parse as a click. WaitForIt does not click.
func TestParseClickIt(t *testing.T) {
	tmpl := b64PNG(t, noiseImage(3, 2, 1))

	for name, click := range map[string]bool{"ClickIt": true, "ClickItEvent": true, "WaitForIt": false} {
		e, err := parseEvent(name + ",2s," + tmpl)
		if err != nil {
			t.Errorf("parse %v: %v", name, err)
			continue
		}
		w, ok := e.(*WaitForItEvent)
		if !ok {
			t.Errorf("parse %v gives %T", name, e)
			continue
		}
		if w.Click != click || w.Timeout != 2*time.Second || w.Template.Bounds() != image.Rect(0, 0, 3, 2) {
			t.Errorf("parse %v = click %v, timeout %v, bounds %v", name, w.Click, w.Timeout, w.Template.Bounds())
		}
	}
}

// T14: a template path that is also valid base64 opens the file first.
func TestParseTemplatePathFirst(t *testing.T) {
	t.Chdir(t.TempDir())

	var b bytes.Buffer
	if err := png.Encode(&b, noiseImage(5, 4, 2)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("abcd", b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	e, err := parseEvent("WaitForIt,1s,abcd")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	w := e.(*WaitForItEvent)
	if w.Template.Bounds() != image.Rect(0, 0, 5, 4) || w.Source != "abcd" {
		t.Errorf("template %v from %q, want 5x4 from abcd", w.Template.Bounds(), w.Source)
	}

	if _, err := parseEvent("WaitForIt,1s,missing.png"); err == nil || !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("parse of a missing file: %v, want an error that names the file", err)
	}
}

// T13: getDuration parses each delta as a duration and skips comments.
func TestGetDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steps.kb")
	lines := "1s:KeyEvent,true,a\n500ms:KeyEvent,false,a\n#: comment: with a colon\n# another: comment\n250000000:KeyEvent,true,b\n\n"
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if d := getDuration(f); d != 1750*time.Millisecond {
		t.Errorf("getDuration = %v, want 1.75s", d)
	}
	if off, _ := f.Seek(0, io.SeekCurrent); off != 0 {
		t.Errorf("file offset after getDuration = %d, want 0", off)
	}
}

// T24: the kb parser does not panic on any line.
func FuzzParseEvent(f *testing.F) {
	tmpl := b64PNG(f, noiseImage(2, 2, 1))
	for _, s := range []string{
		"KeyEvent,true,a", "KeyEvent,false,U+1F600", "KeyEvent,1,Shift_L", "KeyEvent,true",
		"PointerEvent,511,65535,0", "PointerEvent,65536,1,2", "PointerEvent,-1,0,0",
		"LoadFile,foo.kb", "LoadFile,",
		"WaitForIt,1s," + tmpl, "ClickIt,10ms," + tmpl, "ClickItEvent,1000," + tmpl,
		"WaitForIt,1s,abcd", "ClickIt,x,y", "WaitForIt,1s,iVBORw0KGgo=",
		"", ",", "KeyEvent,,", "Bogus,1,2",
	} {
		f.Add(s)
	}

	// Do not open files that the fuzzer names.
	open := openTemplate
	openTemplate = func(string) (*os.File, error) { return nil, fs.ErrNotExist }
	f.Cleanup(func() { openTemplate = open })

	f.Fuzz(func(t *testing.T, line string) {
		e, err := parseEvent(line)
		if err != nil {
			return
		}

		if s, ok := e.(fmt.Stringer); ok {
			again, err := parseEvent(s.String())
			if err != nil {
				t.Fatalf("parse %q gives %q, which does not parse: %v", line, s, err)
			}
			if got := again.(fmt.Stringer).String(); got != s.String() {
				t.Fatalf("parse %q gives %q, then %q", line, s, got)
			}
		}
	})
}
