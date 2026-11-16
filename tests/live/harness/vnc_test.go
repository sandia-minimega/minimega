// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// rawUpdate returns a FramebufferUpdate with one Raw rectangle of the given
// pixels and one DesktopSize rectangle.
func rawUpdate(px ...uint32) []byte {
	b := []byte{0, 0, 0, 2}
	b = append(b, 0, 0, 0, 0, 0, byte(len(px)), 0, 1, 0, 0, 0, 0)
	for _, p := range px {
		b = binary.LittleEndian.AppendUint32(b, p)
	}
	return append(b, 0, 0, 0, 0, 2, 0, 1, 0, 0xff, 0xff, 0xff, 0x21) // -223
}

func TestVNCClient(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	server.SetDeadline(time.Now().Add(10 * time.Second))

	got := make(chan []byte, 1)
	go func() {
		io.WriteString(server, "RFB 003.008\n")
		ver := make([]byte, 12)
		io.ReadFull(server, ver)
		server.Write([]byte{1, 1})
		io.ReadFull(server, make([]byte, 1))
		binary.Write(server, binary.BigEndian, uint32(0))
		io.ReadFull(server, make([]byte, 1))
		binary.Write(server, binary.BigEndian, struct {
			W, H uint16
			PF   [16]byte
			N    uint32
		}{720, 400, [16]byte{}, 2})
		io.WriteString(server, "vm")
		pf := make([]byte, 20)
		io.ReadFull(server, pf)
		server.Write([]byte{3, 0, 0, 0, 0xff, 0xff, 0xff, 0xfc, 0, 0, 0, 1}) // extended, 4 bytes
		server.Write([]byte{3, 0, 0, 0, 0, 0, 0, 2, 'h', 'i'})
		server.Write(append([]byte{2}, rawUpdate(0x112233, 0x445566)...))
		got <- append(ver, pf...)
	}()

	v, err := NewVNC(client)
	if err != nil {
		t.Fatal(err)
	}
	if v.Width != 720 || v.Height != 400 || v.Name != "vm" {
		t.Errorf("server init = %dx%d %q", v.Width, v.Height, v.Name)
	}
	u, err := v.ReadUpdate(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if u.Rects != 2 || u.Pixels != 2 || u.Colors != 2 || u.CutTexts != 2 {
		t.Errorf("update = %+v, want 2 rectangles, 2 pixels, 2 colors, 2 cut texts", u)
	}
	b := <-got
	if string(b[:12]) != "RFB 003.008\n" || b[12] != 0 || b[16] != 32 || b[19] != 1 {
		t.Errorf("client sent % x", b)
	}
}

// writeFB writes a recording of data split into chunks of size n. With cut,
// the gzip stream ends before its trailer.
func writeFB(t *testing.T, data []byte, n int, cut bool) string {
	t.Helper()

	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	for len(data) > 0 {
		c := data[:min(n, len(data))]
		data = data[len(c):]
		fmt.Fprintf(z, "%d %d\r\n%s\r\n", 1000, len(c), c)
	}
	z.Close()

	out := b.Bytes()
	if cut {
		z.Flush()
		out = out[:len(out)-8]
	}

	path := filepath.Join(t.TempDir(), "x.fb")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFBRecording(t *testing.T) {
	data := append(rawUpdate(1, 2, 3), 2) // and a Bell
	data = append(data, rawUpdate(4)...)
	data = append(data, 3, 0, 0, 0, 0, 0, 0, 1, 'x') // ServerCutText

	for _, n := range []int{1, 7, len(data)} {
		rec, err := ReadFBRecording(writeFB(t, data, n, false))
		if err != nil {
			t.Fatal(err)
		}
		if rec.Updates != 2 || rec.Pixels != 4 || rec.Colors != 4 || rec.Partial || !rec.Closed {
			t.Errorf("chunks of %d: %+v, want 2 updates, 4 pixels, 4 colors, closed", n, rec)
		}
	}

	rec, err := ReadFBRecording(writeFB(t, data[:len(data)-3], 5, true))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Updates != 2 || !rec.Partial || rec.Closed {
		t.Errorf("cut recording: %+v, want 2 updates, partial, not closed", rec)
	}

	if _, err := ReadFBRecording(writeFB(t, []byte{9}, 1, false)); err == nil {
		t.Error("unknown message type: want an error")
	}
	bad := rawUpdate(1)
	bad[15] = 5 // Hextile, which the client does not ask for
	if _, err := ReadFBRecording(writeFB(t, bad, 4, false)); err == nil {
		t.Error("Hextile rectangle: want an error")
	}
}

func TestKBFile(t *testing.T) {
	got, err := KBFile("a /\n", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	want := "1000000:KeyEvent,true,a\n1000000:KeyEvent,false,a\n" +
		"1000000:KeyEvent,true,space\n1000000:KeyEvent,false,space\n" +
		"1000000:KeyEvent,true,slash\n1000000:KeyEvent,false,slash\n" +
		"1000000:KeyEvent,true,Return\n1000000:KeyEvent,false,Return\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if _, err := KBFile("A", time.Millisecond); err == nil {
		t.Error("upper case: want an error")
	}
}
