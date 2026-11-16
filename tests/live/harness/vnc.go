// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// This file has an RFB (VNC) client that connects the way the browser
// console does: through the miniweb websocket and the minimega VNC shim. It
// writes the client messages itself, so that the shim and QEMU are the only
// minimega code between the test and the guest.

// RFB encodings that the tests ask for.
const (
	EncodingRaw               int32 = 0
	EncodingDesktopSize       int32 = -223
	EncodingQEMUExtendedKey   int32 = -258
	EncodingExtendedClipboard int32 = -0x3f5e1a32 // 0xc0a1e5ce
)

// keysym and XT scancode of each character that TypeText can type.
type key struct {
	keysym, scancode uint32
}

var keys = map[rune]key{
	'\n': {0xff0d, 0x1c}, ' ': {0x20, 0x39}, '-': {0x2d, 0x0c}, '.': {0x2e, 0x34}, '/': {0x2f, 0x35},
	'0': {'0', 0x0b}, '1': {'1', 0x02}, '2': {'2', 0x03}, '3': {'3', 0x04}, '4': {'4', 0x05},
	'5': {'5', 0x06}, '6': {'6', 0x07}, '7': {'7', 0x08}, '8': {'8', 0x09}, '9': {'9', 0x0a},
	'a': {'a', 0x1e}, 'b': {'b', 0x30}, 'c': {'c', 0x2e}, 'd': {'d', 0x20}, 'e': {'e', 0x12},
	'f': {'f', 0x21}, 'g': {'g', 0x22}, 'h': {'h', 0x23}, 'i': {'i', 0x17}, 'j': {'j', 0x24},
	'k': {'k', 0x25}, 'l': {'l', 0x26}, 'm': {'m', 0x32}, 'n': {'n', 0x31}, 'o': {'o', 0x18},
	'p': {'p', 0x19}, 'q': {'q', 0x10}, 'r': {'r', 0x13}, 's': {'s', 0x1f}, 't': {'t', 0x14},
	'u': {'u', 0x16}, 'v': {'v', 0x2f}, 'w': {'w', 0x11}, 'x': {'x', 0x2d}, 'y': {'y', 0x15},
	'z': {'z', 0x2c},
}

// KBName returns the kb file name of the key that types c: the name that
// "vnc play" reads.
func KBName(c rune) (string, error) {
	switch c {
	case '\n':
		return "Return", nil
	case ' ':
		return "space", nil
	case '-':
		return "minus", nil
	case '.':
		return "period", nil
	case '/':
		return "slash", nil
	}
	if _, ok := keys[c]; !ok {
		return "", fmt.Errorf("no key for %q", c)
	}
	return string(c), nil
}

// KBFile returns kb file lines that type text, with delay before each event.
func KBFile(text string, delay time.Duration) (string, error) {
	var b strings.Builder
	for _, c := range text {
		name, err := KBName(c)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%d:KeyEvent,true,%s\n%d:KeyEvent,false,%s\n", delay.Nanoseconds(), name, delay.Nanoseconds(), name)
	}
	return b.String(), nil
}

// VNC is an RFB 3.8 client connection with no security and a 32-bit true
// color pixel format.
type VNC struct {
	rw     io.ReadWriteCloser
	r      *bufio.Reader
	ws     *websocket.Conn // nil when the connection is not a websocket
	Width  uint16
	Height uint16
	Name   string
}

// DialVNC connects to the console of vm through the miniweb websocket, as
// the browser console does, and completes the RFB handshake.
func DialVNC(vm string) (*VNC, error) {
	u, err := url.Parse(MiniwebURL)
	if err != nil {
		return nil, err
	}
	origin := u.String()
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	u.Path = "/vm/" + vm + "/connect/ws"

	ws, err := websocket.Dial(u.String(), "", origin)
	if err != nil {
		return nil, fmt.Errorf("websocket %v: %w", u, err)
	}
	ws.PayloadType = websocket.BinaryFrame
	if err := ws.SetDeadline(time.Now().Add(time.Minute)); err != nil {
		ws.Close()
		return nil, err
	}

	v, err := NewVNC(ws)
	if err != nil {
		return nil, err
	}
	v.ws = ws
	return v, ws.SetDeadline(time.Time{})
}

// NewVNC completes the RFB 3.8 handshake on rw and sets the pixel format. It
// closes rw on an error.
func NewVNC(rw io.ReadWriteCloser) (*VNC, error) {
	v := &VNC{rw: rw, r: bufio.NewReader(rw)}
	if err := v.handshake(); err != nil {
		rw.Close()
		return nil, err
	}
	return v, nil
}

func (v *VNC) handshake() error {
	version := make([]byte, 12)
	if _, err := io.ReadFull(v.r, version); err != nil {
		return fmt.Errorf("server version: %w", err)
	}
	if !strings.HasPrefix(string(version), "RFB 003.") {
		return fmt.Errorf("server version %q", version)
	}
	if _, err := io.WriteString(v.rw, "RFB 003.008\n"); err != nil {
		return err
	}

	n, err := v.r.ReadByte()
	if err != nil {
		return fmt.Errorf("security types: %w", err)
	}
	if n == 0 {
		return errors.New("server sent no security types")
	}
	types := make([]byte, n)
	if _, err := io.ReadFull(v.r, types); err != nil {
		return fmt.Errorf("security types: %w", err)
	}
	if !strings.ContainsRune(string(types), 1) {
		return fmt.Errorf("server does not offer security type None: %v", types)
	}
	if _, err := v.rw.Write([]byte{1}); err != nil {
		return err
	}
	var result uint32
	if err := binary.Read(v.r, binary.BigEndian, &result); err != nil {
		return fmt.Errorf("security result: %w", err)
	}
	if result != 0 {
		return fmt.Errorf("security result %d", result)
	}

	// ClientInit: shared
	if _, err := v.rw.Write([]byte{1}); err != nil {
		return err
	}

	var init struct {
		Width, Height uint16
		PixelFormat   [16]byte
		NameLength    uint32
	}
	if err := binary.Read(v.r, binary.BigEndian, &init); err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	if init.NameLength > 1<<16 {
		return fmt.Errorf("server name has %d bytes", init.NameLength)
	}
	name := make([]byte, init.NameLength)
	if _, err := io.ReadFull(v.r, name); err != nil {
		return fmt.Errorf("server name: %w", err)
	}
	v.Width, v.Height, v.Name = init.Width, init.Height, string(name)

	// SetPixelFormat: 32 bpp, depth 24, little endian, true color, 8 bits
	// for each of red (shift 16), green (8), and blue (0)
	_, err = v.rw.Write([]byte{0, 0, 0, 0, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0})
	return err
}

// Close closes the connection.
func (v *VNC) Close() error { return v.rw.Close() }

// SetEncodings sends a SetEncodings message.
func (v *VNC) SetEncodings(encs ...int32) error {
	b := []byte{2, 0}
	b = binary.BigEndian.AppendUint16(b, uint16(len(encs)))
	for _, e := range encs {
		b = binary.BigEndian.AppendUint32(b, uint32(e))
	}
	_, err := v.rw.Write(b)
	return err
}

// RequestUpdate asks for an update of the whole screen.
func (v *VNC) RequestUpdate(incremental bool) error {
	b := []byte{3, 0}
	if incremental {
		b[1] = 1
	}
	b = binary.BigEndian.AppendUint16(b, 0)
	b = binary.BigEndian.AppendUint16(b, 0)
	b = binary.BigEndian.AppendUint16(b, v.Width)
	b = binary.BigEndian.AppendUint16(b, v.Height)
	_, err := v.rw.Write(b)
	return err
}

// CutText sends a ClientCutText message, the message of a paste in the
// browser console.
func (v *VNC) CutText(text string) error {
	b := []byte{6, 0, 0, 0}
	b = binary.BigEndian.AppendUint32(b, uint32(len(text)))
	_, err := v.rw.Write(append(b, text...))
	return err
}

// TypeText types text with QEMU extended key events, as noVNC does when QEMU
// supports them, with delay before each event. Without a delay, the keys can
// overrun the keyboard buffer of the guest. Call SetEncodings with
// EncodingQEMUExtendedKey first.
func (v *VNC) TypeText(text string, delay time.Duration) error {
	for _, c := range text {
		k, ok := keys[c]
		if !ok {
			return fmt.Errorf("no key for %q", c)
		}
		for _, down := range []uint16{1, 0} {
			time.Sleep(delay)
			b := []byte{255, 0}
			b = binary.BigEndian.AppendUint16(b, down)
			b = binary.BigEndian.AppendUint32(b, k.keysym)
			b = binary.BigEndian.AppendUint32(b, k.scancode)
			if _, err := v.rw.Write(b); err != nil {
				return err
			}
		}
	}
	return nil
}

// FBUpdate is a FramebufferUpdate that the client read.
type FBUpdate struct {
	Rects  int // all rectangles
	Pixels int // pixels in Raw rectangles
	Colors int // distinct pixel values in Raw rectangles

	// CutTexts is the number of ServerCutText messages before the update,
	// extended (negative length) or not.
	CutTexts int
}

// ReadUpdate reads server messages until a FramebufferUpdate, or until the
// timeout.
func (v *VNC) ReadUpdate(timeout time.Duration) (FBUpdate, error) {
	if v.ws != nil {
		if err := v.ws.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return FBUpdate{}, err
		}
		defer v.ws.SetReadDeadline(time.Time{})
	}

	m := newServerMessages(v.r)
	for {
		u, isUpdate, err := m.next()
		if err != nil || isUpdate {
			u.CutTexts = m.cutTexts
			return u, err
		}
	}
}

// serverMessages reads RFB server messages in the pixel format that VNC and
// the minimega fb recorder set: 4 bytes for each pixel.
type serverMessages struct {
	r        io.Reader
	colors   map[uint32]bool
	cutTexts int // ServerCutText messages read
}

func newServerMessages(r io.Reader) *serverMessages {
	return &serverMessages{r: r, colors: map[uint32]bool{}}
}

// next reads one server message. It returns true for a FramebufferUpdate.
// A message that is cut off gives io.ErrUnexpectedEOF.
func (m *serverMessages) next() (FBUpdate, bool, error) {
	var typ [1]byte
	if _, err := io.ReadFull(m.r, typ[:]); err != nil {
		return FBUpdate{}, false, err
	}

	read := func(n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := io.ReadFull(m.r, b)
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return b, err
	}

	switch typ[0] {
	case 0: // FramebufferUpdate
		hdr, err := read(3)
		if err != nil {
			return FBUpdate{}, false, err
		}
		u := FBUpdate{Rects: int(binary.BigEndian.Uint16(hdr[1:]))}
		for i := 0; i < u.Rects; i++ {
			rh, err := read(12)
			if err != nil {
				return u, false, err
			}
			w, h := int(binary.BigEndian.Uint16(rh[4:])), int(binary.BigEndian.Uint16(rh[6:]))
			switch enc := int32(binary.BigEndian.Uint32(rh[8:])); enc {
			case EncodingRaw:
				px, err := read(w * h * 4)
				if err != nil {
					return u, false, err
				}
				u.Pixels += w * h
				for j := 0; j+4 <= len(px) && len(m.colors) < 1024; j += 4 {
					m.colors[binary.LittleEndian.Uint32(px[j:])&0xffffff] = true
				}
			case EncodingDesktopSize, EncodingQEMUExtendedKey:
				// no data
			default:
				return u, false, fmt.Errorf("rectangle %d has encoding %d, which was not asked for", i, enc)
			}
		}
		u.Colors = len(m.colors)
		return u, true, nil
	case 1: // SetColourMapEntries
		hdr, err := read(5)
		if err != nil {
			return FBUpdate{}, false, err
		}
		_, err = read(6 * int(binary.BigEndian.Uint16(hdr[3:])))
		return FBUpdate{}, false, err
	case 2: // Bell
		return FBUpdate{}, false, nil
	case 3: // ServerCutText
		hdr, err := read(7)
		if err != nil {
			return FBUpdate{}, false, err
		}
		n := int64(int32(binary.BigEndian.Uint32(hdr[3:])))
		if n < 0 {
			n = -n // ExtendedClipboard
		}
		if n > 1<<20 {
			return FBUpdate{}, false, fmt.Errorf("server cut text has %d bytes", n)
		}
		m.cutTexts++
		_, err = read(int(n))
		return FBUpdate{}, false, err
	default:
		return FBUpdate{}, false, fmt.Errorf("unknown server message type %d", typ[0])
	}
}

// FBRecording is what a VNC framebuffer recording holds.
type FBRecording struct {
	Updates int  // complete FramebufferUpdates
	Pixels  int  // pixels in Raw rectangles
	Colors  int  // distinct pixel values in Raw rectangles
	Partial bool // the recording ends inside a message
	Closed  bool // the gzip stream is complete, so minimega closed the file
}

// ReadFBRecording decodes the server messages in a VNC framebuffer recording:
// a gzip stream of "<offset> <n>\r\n<n bytes>\r\n" chunks. A recording that
// is still being written gives the complete updates so far.
func ReadFBRecording(path string) (FBRecording, error) {
	var rec FBRecording

	b, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}

	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return rec, err
	}
	m := newServerMessages(&chunkReader{r: bufio.NewReader(z)})

	for {
		u, isUpdate, err := m.next()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			rec.Partial = errors.Is(err, io.ErrUnexpectedEOF)
			break
		} else if err != nil {
			return rec, err
		}
		if isUpdate {
			rec.Updates++
			rec.Pixels += u.Pixels
			rec.Colors = u.Colors
		}
	}

	// gzip checks the length and the checksum at the end of the stream
	z, err = gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return rec, err
	}
	_, err = io.Copy(io.Discard, z)
	rec.Closed = err == nil

	return rec, nil
}

// chunkReader reads the data of the chunks of a recording. It gives io.EOF at
// the end, and also where a chunk or the gzip stream is cut off.
type chunkReader struct {
	r    *bufio.Reader
	left int64
	err  error
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.left == 0 {
		if c.err != nil {
			return 0, c.err
		}
		var offset int64
		if _, err := fmt.Fscanf(c.r, "%d %d\r\n", &offset, &c.left); err != nil {
			c.left = 0
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				c.err = io.EOF
			} else {
				c.err = fmt.Errorf("chunk header: %w", err)
			}
			return 0, c.err
		}
		if c.left == 0 {
			c.err = errors.New("chunk with no data")
			return 0, c.err
		}
	}

	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	if err != nil {
		c.left, c.err = 0, io.EOF
		return n, nil
	}
	if c.left == 0 {
		if _, err := c.r.Discard(2); err != nil {
			c.err = io.EOF
		}
	}
	return n, nil
}
