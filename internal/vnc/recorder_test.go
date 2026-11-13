// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// kbLines returns the lines of a kb file without their delays.
func kbLines(t *testing.T, path string) []string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var res []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		delay, ev, ok := strings.Cut(line, ":")
		if _, err := strconv.ParseInt(delay, 10, 64); !ok || err != nil {
			t.Fatalf("malformed kb line %q", line)
		}
		res = append(res, ev)
	}

	return res
}

// T8: KeyEvent and 255/0 give the same line, and keysym 0 is skipped.
func TestKBRecordKeys(t *testing.T) {
	rec, path := newTestKBRecorder(t, "vm")
	logs := captureLog(t)

	for _, m := range []ClientMessage{
		&KeyEvent{DownFlag: 1, Key: 'a'},
		&QEMUExtendedKeyEvent{DownFlag: 1, Keysym: 'a', Keycode: 0x1e},
		&KeyEvent{DownFlag: 1, Key: 0},
		&QEMUExtendedKeyEvent{DownFlag: 1, Keysym: 0, Keycode: 0x56},
		&QEMUExtendedKeyEvent{DownFlag: 0, Keysym: 'a', Keycode: 0x1e},
		&KeyEvent{DownFlag: 0, Key: 'a'},
		&SetDesktopSize{Width: 800, Height: 600},
		&ExtendedClientCutText{Data: []byte{0, 0, 0, 1}},
		&ClientFence{},
		&QEMUAudioMessage{Operation: QEMUAudioEnable},
	} {
		rec.Route("vm", m)
	}
	if err := rec.StopKB("vm"); err != nil {
		t.Fatal(err)
	}

	want := []string{"KeyEvent,true,a", "KeyEvent,true,a", "KeyEvent,false,a", "KeyEvent,false,a"}
	if got := kbLines(t, path); !reflect.DeepEqual(got, want) {
		t.Errorf("kb lines = %q, want %q", got, want)
	}
	if n := len(logs.lines("keysym 0")); n != 2 {
		t.Errorf("got %d keysym 0 log lines, want 2", n)
	}
	if lines := logs.lines("unexpected"); len(lines) != 0 {
		t.Errorf("unexpected message logs: %q", lines)
	}
}

// T8: the 9-bit mask is written and parsed with bit size 16, including the
// Back and Forward bits.
func TestKBRecordPointerMask(t *testing.T) {
	rec, path := newTestKBRecorder(t, "vm")

	rec.Route("vm", &PointerEvent{ButtonMask: 0x05, XPosition: 1, YPosition: 2})
	rec.Route("vm", &ExtendedPointerEvent{ButtonMask: 0x81, XPosition: 3, YPosition: 4, HighButtons: 0x01})
	rec.Route("vm", &ExtendedPointerEvent{ButtonMask: 0x80, XPosition: 5, YPosition: 6, HighButtons: 0x02})
	rec.Route("vm", &ExtendedPointerEvent{ButtonMask: 0xff, XPosition: 7, YPosition: 8, HighButtons: 0x03})
	if err := rec.StopKB("vm"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"PointerEvent,5,1,2",
		"PointerEvent,129,3,4",
		"PointerEvent,256,5,6",
		"PointerEvent,511,7,8",
	}
	got := kbLines(t, path)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kb lines = %q, want %q", got, want)
	}

	for _, line := range got {
		e, err := parseEvent(line)
		if err != nil {
			t.Errorf("parse %q: %v", line, err)
			continue
		}
		if s := e.(fmt.Stringer).String(); s != line {
			t.Errorf("parse %q gives %q", line, s)
		}
	}
}

// T8: concurrent Route calls to one recording.
func TestKBRecordConcurrentRoute(t *testing.T) {
	rec, path := newTestKBRecorder(t, "vm")

	const workers, events = 8, 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < events; j++ {
				rec.Route("vm", &KeyEvent{DownFlag: 1, Key: 'a'})
			}
		}()
	}
	wg.Wait()

	if err := rec.StopKB("vm"); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != workers*events {
		t.Fatalf("got %d lines, want %d", len(lines), workers*events)
	}
	re := regexp.MustCompile(`^[0-9]+:KeyEvent,true,a$`)
	for _, line := range lines {
		if !re.MatchString(line) {
			t.Fatalf("malformed line %q", line)
		}
	}
}

// T9: Stop with a Close error still deletes the entry.
func TestRecorderStopCloseError(t *testing.T) {
	rec, _ := newTestKBRecorder(t, "vm")
	rec.kb["vm"].file.Close()

	if err := rec.StopKB("vm"); err == nil {
		t.Error("StopKB with a closed file gave no error")
	}
	if _, ok := rec.kb["vm"]; ok {
		t.Error("StopKB with a Close error did not delete the recording")
	}
	if err := rec.StopKB("vm"); err == nil {
		t.Error("second StopKB gave no error")
	}
}

// T9: the recordings of a VM stop on kill and flush (StopVM), and a VM that
// is launched again with the same name records into a new file.
func TestRecorderStopVM(t *testing.T) {
	f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
	addr := f.listen(t)
	dir := t.TempDir()
	oldKB, oldFB, newKB := filepath.Join(dir, "old.kb"), filepath.Join(dir, "old.fb"), filepath.Join(dir, "new.kb")

	rec := NewRecorder()
	if err := rec.RecordKB("vm", addr, oldKB); err != nil {
		t.Fatal(err)
	}
	if err := rec.RecordFB("vm", addr, oldFB); err != nil {
		t.Fatal(err)
	}
	rec.Route("vm", &KeyEvent{DownFlag: 1, Key: 'a'})

	if err := rec.StopVM("vm"); err != nil {
		t.Fatalf("StopVM: %v", err)
	}
	if info := rec.Info(); len(info) != 0 {
		t.Fatalf("recordings after StopVM: %q", info)
	}
	if err := rec.StopVM("vm"); err != nil {
		t.Errorf("StopVM with no recordings: %v", err)
	}

	if err := rec.RecordKB("vm", addr, newKB); err != nil {
		t.Fatalf("record the relaunched VM: %v", err)
	}
	rec.Route("vm", &KeyEvent{DownFlag: 0, Key: 'a'})
	if err := rec.StopKB("vm"); err != nil {
		t.Fatal(err)
	}

	if got, want := kbLines(t, oldKB), []string{"KeyEvent,true,a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("old kb lines = %q, want %q", got, want)
	}
	if got, want := kbLines(t, newKB), []string{"KeyEvent,false,a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("new kb lines = %q, want %q", got, want)
	}
	readFBFile(t, oldFB)
}

// readFBFile reads an fb recording to EOF and returns the recorded server
// bytes.
func readFBFile(t *testing.T, path string) []byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("fb file is not gzip: %v", err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read fb file to EOF: %v", err)
	}

	var data []byte
	r := bufio.NewReader(bytes.NewReader(b))
	for {
		hdr, err := r.ReadString('\n')
		if err == io.EOF && hdr == "" {
			return data
		} else if err != nil {
			t.Fatalf("read chunk header: %v", err)
		}

		var offset, n int64
		if _, err := fmt.Sscanf(hdr, "%d %d\r\n", &offset, &n); err != nil || offset < 0 || n <= 0 {
			t.Fatalf("malformed chunk header %q: %v", hdr, err)
		}

		chunk := make([]byte, n+2)
		if _, err := io.ReadFull(r, chunk); err != nil {
			t.Fatalf("read chunk: %v", err)
		}
		if !bytes.HasSuffix(chunk, []byte("\r\n")) {
			t.Fatalf("chunk has no trailer")
		}
		data = append(data, chunk[:n]...)
	}
}

// fbServer is a fake server that answers each FramebufferUpdateRequest with
// one update. It closes the connection after limit updates, if limit > 0.
type fbServer struct {
	*fakeRFB

	limit int

	mu   sync.Mutex
	sent []byte
	reqs []FramebufferUpdateRequest
}

func newFBServer(t *testing.T, limit int) (*fbServer, string) {
	s := &fbServer{fakeRFB: &fakeRFB{width: 640, height: 480, pf: clientFormat}, limit: limit}
	s.handle = func(c net.Conn, m ClientMessage) error {
		req, ok := m.(*FramebufferUpdateRequest)
		if !ok {
			return nil
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		s.reqs = append(s.reqs, *req)
		n := len(s.reqs)
		b := fbUpdate(rawRect(0, 0, 4, 3, uint32(n)))
		if _, err := c.Write(b); err != nil {
			return err
		}
		s.sent = append(s.sent, b...)

		if s.limit > 0 && n >= s.limit {
			return io.EOF
		}
		return nil
	}

	return s, s.listen(t)
}

func (s *fbServer) state() ([]byte, []FramebufferUpdateRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]byte(nil), s.sent...), append([]FramebufferUpdateRequest(nil), s.reqs...)
}

// stopFB calls StopFB and fails the test if it does not return at once.
func stopFB(t *testing.T, rec *Recorder, id string) {
	t.Helper()

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- rec.StopFB(id) }()

	if err := recv(t, done, "StopFB"); err != nil {
		t.Errorf("StopFB: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("StopFB took %v", d)
	}
}

// T10: an fb recording of a server that sends a fixed number of frames holds
// every frame and is a complete gzip stream.
func TestFBRecordAllFrames(t *testing.T) {
	const frames = 5

	s, addr := newFBServer(t, frames)
	path := filepath.Join(t.TempDir(), "fb")

	rec := NewRecorder()
	if err := rec.RecordFB("vm", addr, path); err != nil {
		t.Fatal(err)
	}

	recv(t, s.ended, "server end")

	// the reader ends at EOF from the server, after it writes the last chunk
	rec.mu.RLock()
	fb := rec.fb["vm"]
	rec.mu.RUnlock()
	recv(t, fb.readDone, "fb reader end")

	stopFB(t, rec, "vm")

	sent, reqs := s.state()
	if got := readFBFile(t, path); !bytes.Equal(got, sent) {
		t.Errorf("recorded %d bytes, server sent %d bytes", len(got), len(sent))
	}
	if len(reqs) != frames {
		t.Fatalf("server got %d requests, want %d", len(reqs), frames)
	}
	for i, req := range reqs {
		if req.X != 0 || req.Y != 0 || req.Width != 0xffff || req.Height != 0xffff {
			t.Errorf("request %d = %+v, want 0,0,65535,65535", i, req)
		}
	}
}

// T10: vnc stop fb ends at once while the server sends frames, and the file
// is complete.
func TestFBRecordStop(t *testing.T) {
	s, addr := newFBServer(t, 0)
	path := filepath.Join(t.TempDir(), "fb")

	rec := NewRecorder()
	if err := rec.RecordFB("vm", addr, path); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(testTimeout)
	for {
		if _, reqs := s.state(); len(reqs) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not get 3 requests")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stopFB(t, rec, "vm")
	recv(t, s.ended, "server end")

	sent, _ := s.state()
	got := readFBFile(t, path)
	if len(got) == 0 || !bytes.HasPrefix(sent, got) {
		t.Errorf("recorded %d bytes that are not a prefix of the %d bytes sent", len(got), len(sent))
	}
	if info := rec.Info(); len(info) != 0 {
		t.Errorf("recordings after StopFB: %q", info)
	}
}
