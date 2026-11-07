// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// RFB security type None. See RFC 6143 Section 7.2.1.
const securityNone = 1

// shimHandshakeTimeout is the time that the client and the server have to
// complete the handshake through the shim.
const shimHandshakeTimeout = 30 * time.Second

var (
	// ErrClientClosed is the reason that Proxy gives when the client closes
	// the connection.
	ErrClientClosed = errors.New("client closed the connection")

	// ErrServerClosed is the reason that Proxy gives when the server closes
	// the connection.
	ErrServerClosed = errors.New("server closed the connection")
)

// ProxyOptions configures Proxy.
type ProxyOptions struct {
	// Name identifies the connection in log messages, for example a VM name.
	Name string

	// ExtendedClipboard keeps the ExtendedClipboard pseudo-encoding in each
	// SetEncodings. When it is false, the shim removes that encoding, so the
	// client sends plain cut text and OnPaste receives it.
	ExtendedClipboard bool

	// OnMessage, if set, receives each client message that the shim frames,
	// after the shim forwards it. A SetEncodings is given as forwarded.
	OnMessage func(ClientMessage)

	// OnPaste, if set, receives the text of each plain, non-empty
	// ClientCutText, decoded as Latin-1. The shim calls it only when
	// ExtendedClipboard is false.
	OnPaste func(text string)

	// handshakeTimeout replaces shimHandshakeTimeout when it is not zero.
	handshakeTimeout time.Duration
}

// EncodingRule is a policy for one encoding in a client SetEncodings. The
// shim removes the encoding when Keep returns false.
type EncodingRule struct {
	Value int32
	Name  string
	Keep  func(ProxyOptions) bool
}

// EncodingRules is the SetEncodings policy of the shim. The shim forwards
// encodings that have no rule.
var EncodingRules = []EncodingRule{
	{
		Value: ExtendedClipboardPseudoEncoding,
		Name:  "ExtendedClipboard",
		Keep:  func(o ProxyOptions) bool { return o.ExtendedClipboard },
	},
}

// filterEncodings applies EncodingRules to encs. It returns the encodings to
// forward and the rules that removed an encoding.
func (o ProxyOptions) filterEncodings(encs []int32) ([]int32, []EncodingRule) {
	var removed []EncodingRule

	res := make([]int32, 0, len(encs))

outer:
	for _, e := range encs {
		for _, r := range EncodingRules {
			if r.Value == e && !r.Keep(o) {
				removed = append(removed, r)
				continue outer
			}
		}

		res = append(res, e)
	}

	return res, removed
}

type proxy struct {
	client, server net.Conn
	opts           ProxyOptions

	once   sync.Once
	reason error
}

// stop closes both connections. The first call gives the reason.
func (p *proxy) stop(reason error) {
	p.once.Do(func() {
		p.reason = reason
		p.client.Close()
		p.server.Close()
	})
}

// Proxy connects a VNC client to a VNC server, for example a QEMU VNC
// socket. Server-to-client bytes go through unchanged. Proxy follows the RFB
// handshake, and then frames each client message, applies EncodingRules to
// SetEncodings, and forwards the message in one write. When the handshake
// uses a security type other than None, or when the client sends a message
// that the shim does not know, Proxy logs one warning and forwards the rest
// of the stream unchanged. The handshake has a deadline of
// shimHandshakeTimeout.
//
// An error or a close on one side closes both sides. Proxy returns when both
// directions stop. It closes both connections and returns the reason, which
// is ErrClientClosed or ErrServerClosed for a normal close.
func Proxy(ctx context.Context, client, server net.Conn, opts ProxyOptions) error {
	p := &proxy{client: client, server: server, opts: opts}

	stopCtx := context.AfterFunc(ctx, func() { p.stop(context.Cause(ctx)) })
	defer stopCtx()

	version, framed, err := p.timedHandshake()
	if err != nil {
		p.stop(err)
		log.Info("vnc shim %v: client %v: handshake ended: %v", opts.Name, client.RemoteAddr(), p.reason)
		return p.reason
	}

	log.Info("vnc shim %v: client %v connected, RFB %v", opts.Name, client.RemoteAddr(), version)

	done := make(chan struct{})
	go func() {
		defer close(done)

		if _, err := io.Copy(client, server); err != nil {
			p.stop(fmt.Errorf("server to client: %w", err))
		} else {
			p.stop(ErrServerClosed)
		}
	}()

	if framed {
		p.stop(p.forwardMessages())
	} else {
		p.stop(p.passthrough())
	}

	<-done

	log.Info("vnc shim %v: client %v disconnected: %v", opts.Name, client.RemoteAddr(), p.reason)

	return p.reason
}

// relay reads n bytes from src, writes them to dst, and returns them.
func relay(dst, src net.Conn, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(src, b); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	if _, err := dst.Write(b); err != nil {
		return nil, err
	}

	return b, nil
}

// parseVersion parses a client ProtocolVersion message. It returns the
// version that the server uses, or an empty string for a version that the
// shim does not follow. Versions 3.4 and 3.5 are handled as 3.3 (RFC 6143
// Section 7.1.1).
func parseVersion(b []byte) string {
	if len(b) != 12 || string(b[:4]) != "RFB " || b[7] != '.' || b[11] != '\n' {
		return ""
	}

	major, err1 := strconv.Atoi(string(b[4:7]))
	minor, err2 := strconv.Atoi(string(b[8:11]))
	if err1 != nil || err2 != nil || major != 3 {
		return ""
	}

	switch minor {
	case 3, 4, 5:
		return "3.3"
	case 7:
		return "3.7"
	case 8:
		return "3.8"
	}

	return ""
}

// timedHandshake runs handshake with a deadline on both connections, and
// clears the deadline after it.
func (p *proxy) timedHandshake() (string, bool, error) {
	timeout := p.opts.handshakeTimeout
	if timeout == 0 {
		timeout = shimHandshakeTimeout
	}

	if err := p.setDeadline(time.Now().Add(timeout)); err != nil {
		return "", false, err
	}

	version, framed, err := p.handshake()
	if err != nil {
		return "", false, err
	}

	return version, framed, p.setDeadline(time.Time{})
}

func (p *proxy) setDeadline(t time.Time) error {
	if err := p.client.SetDeadline(t); err != nil {
		return fmt.Errorf("client deadline: %w", err)
	}
	if err := p.server.SetDeadline(t); err != nil {
		return fmt.Errorf("server deadline: %w", err)
	}

	return nil
}

// handshake relays the RFB handshake up to and including ClientInit. It
// returns the RFB version and true when the shim can frame the messages that
// follow. It returns false, after one warning, when the rest of the stream
// must go through unchanged.
func (p *proxy) handshake() (string, bool, error) {
	c, s := p.client, p.server

	if _, err := relay(c, s, 12); err != nil {
		return "", false, fmt.Errorf("server version: %w", err)
	}

	b, err := relay(s, c, 12)
	if err != nil {
		return "", false, fmt.Errorf("client version: %w", err)
	}

	version := parseVersion(b)
	if version == "" {
		return p.warnPassthrough(fmt.Sprintf("%q", b), "client sent an unknown RFB version")
	}

	if version == "3.3" {
		b, err := relay(c, s, 4)
		if err != nil {
			return "", false, fmt.Errorf("security type: %w", err)
		}

		if t := binary.BigEndian.Uint32(b); t != securityNone {
			return p.warnPassthrough(version, fmt.Sprintf("server chose security type %d", t))
		}
	} else {
		b, err := relay(c, s, 1)
		if err != nil {
			return "", false, fmt.Errorf("security types: %w", err)
		}

		if b[0] == 0 {
			return p.warnPassthrough(version, "server sent no security types")
		}

		if _, err := relay(c, s, int(b[0])); err != nil {
			return "", false, fmt.Errorf("security types: %w", err)
		}

		b, err = relay(s, c, 1)
		if err != nil {
			return "", false, fmt.Errorf("security choice: %w", err)
		}

		if b[0] != securityNone {
			return p.warnPassthrough(version, fmt.Sprintf("client chose security type %d", b[0]))
		}

		if version == "3.8" {
			b, err := relay(c, s, 4)
			if err != nil {
				return "", false, fmt.Errorf("security result: %w", err)
			}

			if r := binary.BigEndian.Uint32(b); r != 0 {
				return p.warnPassthrough(version, fmt.Sprintf("security result %d", r))
			}
		}
	}

	if _, err := relay(s, c, 1); err != nil {
		return "", false, fmt.Errorf("client init: %w", err)
	}

	return version, true, nil
}

func (p *proxy) warnPassthrough(version, why string) (string, bool, error) {
	log.Warn("vnc shim %v: %v, passing the stream through unchanged", p.opts.Name, why)
	return version, false, nil
}

// passthrough forwards client bytes unchanged until an error.
func (p *proxy) passthrough() error {
	if _, err := io.Copy(p.server, p.client); err != nil {
		return fmt.Errorf("client to server: %w", err)
	}

	return ErrClientClosed
}

// forwardMessages frames and forwards client messages until an error.
func (p *proxy) forwardMessages() error {
	r := newMessageReader(p.client)

	for {
		raw, msg, err := r.next()

		var unknown *UnknownMessageError
		switch {
		case err == io.EOF:
			return ErrClientClosed
		case errors.As(err, &unknown):
			log.Warn("vnc shim %v: %v, passing the stream through unchanged", p.opts.Name, err)

			if _, err := p.server.Write(unknown.Raw); err != nil {
				return fmt.Errorf("client to server: %w", err)
			}

			return p.passthrough()
		case err != nil:
			if errors.Is(err, ErrMessageLimit) {
				log.Warn("vnc shim %v: %v", p.opts.Name, err)
			}

			return fmt.Errorf("client message: %w", err)
		}

		if m, ok := msg.(*SetEncodings); ok {
			if encs, removed := p.opts.filterEncodings(m.Encodings); len(removed) > 0 {
				msg = &SetEncodings{Encodings: encs}

				var buf bytes.Buffer
				if err := msg.Write(&buf); err != nil {
					return err
				}
				raw = buf.Bytes()

				for _, r := range removed {
					log.Debug("vnc shim %v: removed %v (%d) from SetEncodings", p.opts.Name, r.Name, r.Value)
				}
			}
		}

		if _, err := p.server.Write(raw); err != nil {
			return fmt.Errorf("client to server: %w", err)
		}

		if p.opts.OnMessage != nil {
			p.opts.OnMessage(msg)
		}

		if cut, ok := msg.(*ClientCutText); ok && len(cut.Text) > 0 && !p.opts.ExtendedClipboard && p.opts.OnPaste != nil {
			log.Debug("vnc shim %v: paste of %d characters", p.opts.Name, len(cut.Text))
			p.opts.OnPaste(latin1(cut.Text))
		}
	}
}

// latin1 decodes ISO 8859-1 text.
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}

	return string(r)
}

// Serve accepts connections on l and calls handle for each one in a new
// goroutine. It retries a failed Accept with backoff, and it returns only when
// l is closed.
func Serve(l net.Listener, handle func(net.Conn)) {
	var delay time.Duration

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			if delay == 0 {
				delay = 5 * time.Millisecond
			} else {
				delay *= 2
			}
			if delay > time.Second {
				delay = time.Second
			}

			log.Warn("vnc shim: accept failed: %v; retrying in %v", err, delay)
			time.Sleep(delay)
			continue
		}

		delay = 0
		go handle(conn)
	}
}
