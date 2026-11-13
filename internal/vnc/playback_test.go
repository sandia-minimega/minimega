// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// startTestPlayback starts a playback of the kb lines against addr.
func startTestPlayback(t *testing.T, addr string, lines string) *playback {
	t.Helper()

	path := filepath.Join(t.TempDir(), "steps.kb")
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := newPlayback("vm", addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Start(path); err != nil {
		p.Conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	return p
}

// setEncodings returns the encoding lists in msgs.
func setEncodings(msgs []ClientMessage) [][]int32 {
	var res [][]int32
	for _, m := range msgs {
		if se, ok := m.(*SetEncodings); ok {
			res = append(res, se.Encodings)
		}
	}
	return res
}

// drainMessages returns the messages in ch that are queued now.
func drainMessages(ch chan ClientMessage) []ClientMessage {
	var res []ClientMessage
	for {
		select {
		case m := <-ch:
			res = append(res, m)
		default:
			return res
		}
	}
}

// T16: playback asks for Raw and DesktopSize and no cursor, and the
// screenshot reader keeps running after cursor rectangles.
func TestPlaybackEncodingsAndScreenshots(t *testing.T) {
	update := fbUpdate(
		cursorRect(16, 16),
		rect(0, 0, 640, 480, DesktopSizePseudoEncoding),
		rawRect(0, 0, 4, 3, 0x0000ff00),
	)

	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	started := make(chan net.Conn, 1)
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			select {
			case started <- c:
			default:
			}
		}
		return nil
	}
	addr := f.listen(t)

	p := startTestPlayback(t, addr, "3600000000000:KeyEvent,true,a\n")

	c := recv(t, started, "SetEncodings")
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			if _, err := c.Write(update); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()

	for i := 0; i < 3; i++ {
		img := recv(t, p.screenshots, "screenshot")
		if img.Rect != image.Rect(0, 0, 4, 3) {
			t.Fatalf("screenshot %d bounds = %v, want the raw rectangle", i, img.Rect)
		}
	}

	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	recv(t, f.ended, "playback connection end")

	encs := setEncodings(drainMessages(f.msgs))
	if len(encs) == 0 {
		t.Fatal("server got no SetEncodings")
	}
	for _, e := range encs {
		if !slices.Contains(e, RawEncoding) || !slices.Contains(e, DesktopSizePseudoEncoding) || slices.Contains(e, CursorPseudoEncoding) {
			t.Errorf("SetEncodings %v, want Raw and DesktopSize and no Cursor", e)
		}
	}
}

// T8: playback sends mask & 0x7f and logs one time when it drops bits.
func TestPlaybackPointerMask(t *testing.T) {
	logs := captureLog(t)

	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	startTestPlayback(t, addr, "0:PointerEvent,385,1,2\n0:PointerEvent,384,3,4\n0:PointerEvent,5,5,6\n")
	recv(t, f.ended, "playback connection end")

	var got []PointerEvent
	for _, m := range drainMessages(f.msgs) {
		if pe, ok := m.(*PointerEvent); ok {
			got = append(got, *pe)
		}
	}

	want := []PointerEvent{
		{ButtonMask: 0x01, XPosition: 1, YPosition: 2},
		{ButtonMask: 0x00, XPosition: 3, YPosition: 4},
		{ButtonMask: 0x05, XPosition: 5, YPosition: 6},
	}
	if !slices.Equal(got, want) {
		t.Errorf("server got %+v, want %+v", got, want)
	}
	if n := len(logs.lines("mask bits")); n != 1 {
		t.Errorf("got %d mask bits log lines, want 1", n)
	}
}

// waitUntil polls cond until it is true, or fails the test after testTimeout.
func waitUntil(t testing.TB, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// within runs fn and fails the test if fn does not return in testTimeout.
func within(t testing.TB, what string, fn func() error) error {
	t.Helper()

	res := make(chan error, 1)
	go func() { res <- fn() }()

	return recv(t, res, what)
}

// nextKey returns the next KeyEvent that the server gets.
func nextKey(t testing.TB, f *fakeRFB) *KeyEvent {
	t.Helper()

	deadline := time.After(testTimeout)
	for {
		select {
		case m := <-f.msgs:
			if k, ok := m.(*KeyEvent); ok {
				return k
			}
		case <-deadline:
			t.Fatal("timeout waiting for a KeyEvent")
			return nil
		}
	}
}

// noKey fails the test if the server gets a KeyEvent in d.
func noKey(t testing.TB, f *fakeRFB, d time.Duration) {
	t.Helper()

	deadline := time.After(d)
	for {
		select {
		case m := <-f.msgs:
			if k, ok := m.(*KeyEvent); ok {
				t.Fatalf("server got %v, want no KeyEvent", k)
			}
		case <-deadline:
			return
		}
	}
}

// remaining returns the remaining duration of p.
func remaining(p *playback) time.Duration {
	p.Lock()
	defer p.Unlock()

	return p.duration
}

// fakeClock is a clock that moves only when the test calls Advance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer

	armed chan time.Duration // gets the duration of each After call
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0), armed: make(chan time.Duration, 64)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, fakeTimer{at: c.now.Add(d), ch: ch})
	c.mu.Unlock()

	c.fire()
	select {
	case c.armed <- d:
	default:
	}

	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()

	c.fire()
}

func (c *fakeClock) fire() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var rest []fakeTimer
	for _, tm := range c.timers {
		if tm.at.After(c.now) {
			rest = append(rest, tm)
			continue
		}
		tm.ch <- c.now
	}
	c.timers = rest
}

// T12: control calls race with the end of the file. No call panics or
// blocks.
func TestPlaybackControlAtEnd(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	for i := 0; i < 50; i++ {
		p := startTestPlayback(t, addr, "0:KeyEvent,true,a\n0:KeyEvent,false,a\n")

		err := within(t, "control calls at the end of the file", func() error {
			for !p.Closed() {
				_ = p.Pause()
				_ = p.Step()
				_ = p.Continue()
				_ = p.Inject("KeyEvent,true,b")
				_ = p.Inject("LoadFile,missing.kb")
				_ = p.Info()
				_, _ = p.GetStep()
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		recv(t, p.ended, "playback end")
		if err := p.Pause(); err == nil {
			t.Error("Pause after the end gave no error")
		}
		if err := p.Inject("KeyEvent,true,b"); err == nil {
			t.Error("Inject after the end gave no error")
		}
	}
}

// T12: Pause and Info do not block while a WaitForIt waits for a screenshot.
func TestPlaybackPauseDuringWaitForIt(t *testing.T) {
	f := &fakeRFB{width: 64, height: 48, pf: clientFormat}
	addr := f.listen(t)

	tmpl := b64PNG(t, noiseImage(4, 4, 1))
	p := startTestPlayback(t, addr, "0:WaitForIt,30s,"+tmpl+"\n")

	waitUntil(t, "WaitForIt step", func() bool {
		s, _ := p.GetStep()
		return s != ""
	})

	if err := within(t, "Pause", p.Pause); err != nil {
		t.Fatal(err)
	}
	info := within(t, "Info", func() error {
		if info := p.Info(); !slices.Contains(info, "PAUSED") {
			return fmt.Errorf("info = %q, want PAUSED", info)
		}
		return nil
	})
	if info != nil {
		t.Fatal(info)
	}
	if err := within(t, "Stop", p.Stop); err != nil {
		t.Fatal(err)
	}
	recv(t, p.ended, "playback end")
}

// T12: Inject while paused keeps the pause and the injected event. The event
// runs after Continue.
func TestPlaybackInjectWhilePaused(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	nested := filepath.Join(t.TempDir(), "nested.kb")
	if err := os.WriteFile(nested, []byte("0:KeyEvent,true,b\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := startTestPlayback(t, addr, "3600s:KeyEvent,true,a\n")
	waitUntil(t, "first step", func() bool {
		s, _ := p.GetStep()
		return s != ""
	})

	if err := within(t, "Pause", p.Pause); err != nil {
		t.Fatal(err)
	}
	if err := within(t, "Inject", func() error { return p.Inject("LoadFile," + nested) }); err != nil {
		t.Fatal(err)
	}

	noKey(t, f, 200*time.Millisecond)
	if info := p.Info(); !slices.Contains(info, "PAUSED") {
		t.Fatalf("info after Inject = %q, want PAUSED", info)
	}

	if err := within(t, "Continue", p.Continue); err != nil {
		t.Fatal(err)
	}
	if k := nextKey(t, f); k.Key != 'b' || k.DownFlag != 1 {
		t.Fatalf("server got %v, want the injected b press", k)
	}
	if p.Closed() {
		t.Fatal("playback ended, want it to wait for the next event")
	}
}

// T12: Step sends the next event at once.
func TestPlaybackStep(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := startTestPlayback(t, addr, "3600s:KeyEvent,true,a\n3600s:KeyEvent,true,b\n")

	for _, want := range []uint32{'a', 'b'} {
		waitUntil(t, "step", func() bool {
			s, _ := p.GetStep()
			return strings.HasSuffix(s, string(rune(want)))
		})
		if err := within(t, "Step", p.Step); err != nil {
			t.Fatal(err)
		}
		if k := nextKey(t, f); k.Key != want {
			t.Fatalf("server got %v, want %c", k, want)
		}
	}

	recv(t, p.ended, "playback end")
}

// T12: the server closes the connection during playback, as QEMU does when
// the VM is killed (minimega#1090). The playback stops with no panic.
func TestPlaybackServerClose(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	p := startTestPlayback(t, addr, "3600s:KeyEvent,true,a\n")
	waitUntil(t, "first step", func() bool {
		s, _ := p.GetStep()
		return s != ""
	})

	f.closeConns()

	waitUntil(t, "playback stop", p.Closed)
	recv(t, p.ended, "playback end")

	if err := within(t, "Pause", p.Pause); err == nil {
		t.Error("Pause after the close gave no error")
	}
	if err := within(t, "Inject", func() error { return p.Inject("KeyEvent,true,b") }); err == nil {
		t.Error("Inject after the close gave no error")
	}
}

// T13: the time that passes before a pause counts one time, and the time of
// the pause does not count.
func TestPlaybackWaitAfterPause(t *testing.T) {
	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	addr := f.listen(t)

	path := filepath.Join(t.TempDir(), "steps.kb")
	if err := os.WriteFile(path, []byte("10s:KeyEvent,true,a\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := newPlayback("vm", addr)
	if err != nil {
		t.Fatal(err)
	}
	clk := newFakeClock()
	p.clock = clk
	if err := p.Start(path); err != nil {
		p.Conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	if d := recv(t, clk.armed, "timer"); d != 10*time.Second {
		t.Fatalf("timer = %v, want 10s", d)
	}

	clk.Advance(3 * time.Second)
	if err := within(t, "Pause", p.Pause); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "7s remaining", func() bool { return remaining(p) == 7*time.Second })

	clk.Advance(100 * time.Second)
	if err := within(t, "Continue", p.Continue); err != nil {
		t.Fatal(err)
	}
	if d := recv(t, clk.armed, "timer after Continue"); d != 7*time.Second {
		t.Fatalf("timer after Continue = %v, want 7s", d)
	}
	if d := remaining(p); d != 7*time.Second {
		t.Fatalf("remaining after Continue = %v, want 7s", d)
	}

	noKey(t, f, 50*time.Millisecond)
	clk.Advance(7 * time.Second)
	if k := nextKey(t, f); k.Key != 'a' {
		t.Fatalf("server got %v, want a", k)
	}
	recv(t, p.ended, "playback end")
	if d := remaining(p); d != 0 {
		t.Errorf("remaining at the end = %v, want 0", d)
	}
}

// noiseImage returns a w x h opaque image of random pixels.
func noiseImage(w, h int, seed uint64) *image.RGBA {
	r := rand.New(rand.NewPCG(seed, seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(r.Uint32())
		img.Pix[i+1] = uint8(r.Uint32())
		img.Pix[i+2] = uint8(r.Uint32())
		img.Pix[i+3] = 0xff
	}
	return img
}

// b64PNG returns img as a base64 PNG.
func b64PNG(t testing.TB, img image.Image) string {
	t.Helper()

	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// imageRect returns a Raw rectangle at 0,0 with the pixels of img, in
// clientFormat.
func imageRect(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	b := rect(0, 0, uint16(w), uint16(h), RawEncoding)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			b = binary.LittleEndian.AppendUint32(b, uint32(c.R)<<16|uint32(c.G)<<8|uint32(c.B))
		}
	}
	return b
}

// T14, T15: a ClickIt line with a base64 template longer than 64 KB is read.
// The template is found, and the click sends a press and then a release.
func TestPlaybackClickItLongLine(t *testing.T) {
	screen := noiseImage(170, 165, 7)
	tmpl := b64PNG(t, screen.SubImage(image.Rect(7, 3, 167, 163)))
	if len(tmpl) <= 64*1024 {
		t.Fatalf("template line has %d bytes, want more than 64 KB", len(tmpl))
	}
	update := fbUpdate(imageRect(screen))

	f := &fakeRFB{width: 170, height: 165, pf: clientFormat}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*FramebufferUpdateRequest); ok {
			_, err := c.Write(update)
			return err
		}
		return nil
	}
	addr := f.listen(t)

	startTestPlayback(t, addr, "0:ClickIt,10s,"+tmpl+"\n")
	recv(t, f.ended, "playback connection end")

	var got []PointerEvent
	for _, m := range drainMessages(f.msgs) {
		if pe, ok := m.(*PointerEvent); ok {
			got = append(got, *pe)
		}
	}
	want := []PointerEvent{
		{ButtonMask: 1, XPosition: 87, YPosition: 83},
		{ButtonMask: 0, XPosition: 87, YPosition: 83},
	}
	if !slices.Equal(got, want) {
		t.Errorf("server got %+v, want %+v", got, want)
	}
}
