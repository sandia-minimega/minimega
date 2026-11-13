// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type chunk struct {
	offset time.Duration
	data   []byte
}

// writeRecording writes chunks to path in the fb recording format.
func writeRecording(t testing.TB, path string, chunks ...chunk) {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	for _, c := range chunks {
		fmt.Fprintf(zw, "%d %d\r\n", c.offset.Nanoseconds(), len(c.data))
		zw.Write(c.data)
		io.WriteString(zw, "\r\n")
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func fbUpdate(rects ...[]byte) []byte {
	b := []byte{0, 0}
	b = binary.BigEndian.AppendUint16(b, uint16(len(rects)))
	for _, r := range rects {
		b = append(b, r...)
	}
	return b
}

func rectHeader(x, y, w, h int, enc int32) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(x))
	b = binary.BigEndian.AppendUint16(b, uint16(y))
	b = binary.BigEndian.AppendUint16(b, uint16(w))
	b = binary.BigEndian.AppendUint16(b, uint16(h))
	return binary.BigEndian.AppendUint32(b, uint32(enc))
}

// rawRect is a Raw rectangle of one color in the rfbplay pixel format.
func rawRect(x, y, w, h int, c color.RGBA) []byte {
	b := rectHeader(x, y, w, h, RawEncoding)
	for i := 0; i < w*h; i++ {
		b = append(b, c.B, c.G, c.R, 0)
	}
	return b
}

// trackFS is an http.FileSystem that closes closed when a file is closed.
type trackFS struct {
	http.FileSystem
	once   sync.Once
	closed chan struct{}
}

func newTrackFS(dir string) *trackFS {
	return &trackFS{FileSystem: http.Dir(dir), closed: make(chan struct{})}
}

func (fs *trackFS) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return &trackFile{File: f, fs: fs}, nil
}

type trackFile struct {
	http.File
	fs *trackFS
}

func (f *trackFile) Close() error {
	f.fs.once.Do(func() { close(f.fs.closed) })
	return f.File.Close()
}

// syncWriter is an http.ResponseWriter that is safe for concurrent use. It
// closes wrote at the first write.
type syncWriter struct {
	mu     sync.Mutex
	header http.Header
	code   int
	body   bytes.Buffer
	once   sync.Once
	wrote  chan struct{}
}

func newSyncWriter() *syncWriter {
	return &syncWriter{header: make(http.Header), code: http.StatusOK, wrote: make(chan struct{})}
}

func (w *syncWriter) Header() http.Header {
	return w.header
}

func (w *syncWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.code = code
}

func (w *syncWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.once.Do(func() { close(w.wrote) })
	return w.body.Write(b)
}

func (w *syncWriter) result() (int, []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.code, bytes.Clone(w.body.Bytes())
}

// serve runs h for name in a goroutine and returns a channel that is closed
// when h returns.
func serve(ctx context.Context, h http.Handler, w http.ResponseWriter, name string) <-chan struct{} {
	req := httptest.NewRequest(http.MethodGet, "/"+url.PathEscape(name), nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()
	return done
}

func wait(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-c:
	case <-time.After(testTimeout):
		t.Fatalf("timeout waiting for %s", what)
	}
}

// frames returns the bounds of each JPEG image in a multipart response.
func frames(t *testing.T, contentType string, body []byte) []image.Rectangle {
	t.Helper()

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("content type %q: %v", contentType, err)
	}

	var rs []image.Rectangle
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return rs
		} else if err != nil {
			t.Fatalf("next part: %v", err)
		}

		img, err := jpeg.Decode(p)
		if err != nil {
			t.Fatalf("decode part %d: %v", len(rs), err)
		}
		rs = append(rs, img.Bounds())
	}
}

// streamFile serves the recording made from chunks and returns the bounds of
// each frame.
func streamFile(t *testing.T, chunks ...chunk) []image.Rectangle {
	t.Helper()
	return streamFileFrom(t, 0, chunks...)
}

// streamFileFrom is streamFile with the playback offset start.
func streamFileFrom(t *testing.T, start time.Duration, chunks ...chunk) []image.Rectangle {
	t.Helper()

	dir := t.TempDir()
	writeRecording(t, filepath.Join(dir, "rec.fb"), chunks...)

	fs := newTrackFS(dir)
	w := newSyncWriter()
	req := httptest.NewRequest(http.MethodGet, "/rec.fb?offset="+start.String(), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&playbackServer{fs}).ServeHTTP(w, req)
	}()
	wait(t, done, "the handler")
	wait(t, fs.closed, "the file to close")

	code, body := w.result()
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	return frames(t, w.Header().Get("Content-Type"), body)
}

// T21: a zero-rectangle update does not panic and is not a frame.
func TestStreamZeroRectangles(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}

	got := streamFile(t,
		chunk{0, fbUpdate()},
		chunk{0, fbUpdate(rawRect(0, 0, 4, 3, red))},
		chunk{0, fbUpdate()},
	)

	if want := []image.Rectangle{image.Rect(0, 0, 4, 3)}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("frames = %v, want %v", got, want)
	}
}

// T21: the canvas has the framebuffer size, not the size of the last
// rectangle.
func TestStreamCanvasSize(t *testing.T) {
	c := color.RGBA{0, 0, 255, 255}

	got := streamFile(t,
		// the last rectangle is smaller than the screen
		chunk{0, fbUpdate(rawRect(4, 2, 4, 4, c), rawRect(0, 0, 4, 6, c), rawRect(2, 2, 1, 1, c))},
		chunk{0, fbUpdate(rawRect(1, 1, 2, 2, c))},
		// a resize to a larger screen
		chunk{0, fbUpdate(rectHeader(0, 0, 16, 10, DesktopSize), rawRect(0, 0, 16, 10, c), rawRect(0, 0, 1, 1, c))},
		// a resize to a smaller screen
		chunk{0, fbUpdate(rectHeader(0, 0, 8, 8, DesktopSize))},
	)

	want := []image.Rectangle{
		image.Rect(0, 0, 8, 6),
		image.Rect(0, 0, 8, 6),
		image.Rect(0, 0, 16, 10),
		image.Rect(0, 0, 8, 8),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("frames = %v, want %v", got, want)
	}
}

// The recording time of an update is the sum of the chunk delays up to the
// chunk with its first byte, also when updates share a chunk or an update
// spans chunks. The playback offset skips the updates before it.
func TestStreamChunkTime(t *testing.T) {
	const ms = time.Millisecond
	c := color.RGBA{0, 255, 0, 255}
	split := fbUpdate(rawRect(0, 0, 2, 2, c))

	tests := []struct {
		name   string
		start  time.Duration
		chunks []chunk
	}{
		{
			// the updates are at 10ms, 10ms, and 20ms
			name:  "shared chunk",
			start: 15 * ms,
			chunks: []chunk{
				{10 * ms, append(fbUpdate(rawRect(0, 0, 2, 2, c)), fbUpdate(rawRect(0, 0, 3, 3, c))...)},
				{10 * ms, fbUpdate(rawRect(0, 0, 4, 4, c))},
			},
		},
		{
			// the updates are at 10ms and 40ms
			name:  "split update",
			start: 30 * ms,
			chunks: []chunk{
				{10 * ms, split[:6]},
				{20 * ms, split[6:]},
				{10 * ms, fbUpdate(rawRect(0, 0, 4, 4, c))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := streamFileFrom(t, tt.start, tt.chunks...)
			if want := []image.Rectangle{image.Rect(0, 0, 4, 4)}; fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("frames = %v, want %v", got, want)
			}
		})
	}
}

// T21: the handler returns an error for a bad file and closes the file.
func TestStreamBadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.fb"), []byte("not a gzip file"), 0644); err != nil {
		t.Fatal(err)
	}

	fs := newTrackFS(dir)
	w := newSyncWriter()
	wait(t, serve(context.Background(), &playbackServer{fs}, w, "bad.fb"), "the handler")
	wait(t, fs.closed, "the file to close")

	if code, _ := w.result(); code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", code, http.StatusInternalServerError)
	}
}

// waitGoroutines waits until no more than n goroutines run.
func waitGoroutines(t *testing.T, n int) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for runtime.NumGoroutine() > n {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			t.Fatalf("%d goroutines, want %d or fewer:\n%s", runtime.NumGoroutine(), n, buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// T21: the handler stops, closes the file, and leaks no goroutine when the
// client disconnects.
func TestStreamClientDisconnect(t *testing.T) {
	var chunks []chunk
	for i := 0; i < 1000; i++ {
		chunks = append(chunks, chunk{20 * time.Millisecond, fbUpdate(rawRect(0, 0, 2, 2, testColor(i)))})
	}
	dir := t.TempDir()
	writeRecording(t, filepath.Join(dir, "long.fb"), chunks...)

	base := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs := newTrackFS(dir)
	w := newSyncWriter()
	done := serve(ctx, &playbackServer{fs}, w, "long.fb")

	wait(t, w.wrote, "the first frame")
	cancel()

	wait(t, done, "the handler")
	wait(t, fs.closed, "the file to close")
	waitGoroutines(t, base)
}

// T21: the handler stops on a real client disconnect.
func TestStreamHTTPDisconnect(t *testing.T) {
	var chunks []chunk
	for i := 0; i < 1000; i++ {
		chunks = append(chunks, chunk{20 * time.Millisecond, fbUpdate(rawRect(0, 0, 2, 2, testColor(i)))})
	}
	dir := t.TempDir()
	writeRecording(t, filepath.Join(dir, "long.fb"), chunks...)

	fs := newTrackFS(dir)
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		(&playbackServer{fs}).ServeHTTP(w, r)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: testTimeout}
	resp, err := client.Get(srv.URL + "/long.fb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(resp.Body).ReadByte(); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	wait(t, handlerDone, "the handler")
	wait(t, fs.closed, "the file to close")
}

// B25: transcode starts after the listener is ready, and the URL escapes the
// file name.
func TestServeTranscode(t *testing.T) {
	dir := t.TempDir()
	name := "a b#1?%.fb"
	writeRecording(t, filepath.Join(dir, name), chunk{0, fbUpdate(rawRect(0, 0, 2, 2, testColor(1)))})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var src string
	var got []image.Rectangle
	err = serveTranscode(ln, dir, name, "out.mp4", func(in, out string) error {
		src = in
		if out != "out.mp4" {
			t.Errorf("out = %q, want out.mp4", out)
		}

		resp, err := (&http.Client{Timeout: testTimeout}).Get(in)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %v", resp.Status)
		}
		got = frames(t, resp.Header.Get("Content-Type"), body)
		return nil
	})
	if err != nil {
		t.Fatalf("serveTranscode: %v", err)
	}

	u, err := url.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/"+name || u.RawQuery != "" || u.Fragment != "" {
		t.Errorf("URL %q has path %q, want %q", src, u.Path, "/"+name)
	}
	if len(got) != 1 {
		t.Errorf("got %d frames, want 1", len(got))
	}

	if c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		c.Close()
		t.Error("the listener is open after serveTranscode returns")
	}
}

// B25: the reader keeps the bytes that are left at the end of the file.
func TestRecordingReaderLeftover(t *testing.T) {
	var buf bytes.Buffer
	io.WriteString(&buf, "0 5\r\nabcde\r\n")
	// a chunk that the end of the file cuts short
	io.WriteString(&buf, "0 10\r\nfgh")

	r := NewRecordingReader(bufio.NewReader(&buf))

	b := make([]byte, 2)
	if n, err := r.Read(b); n != 2 || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}

	got, _ := io.ReadAll(r)
	if string(got) != "cdefgh" {
		t.Errorf("read %q, want %q", got, "cdefgh")
	}
}
