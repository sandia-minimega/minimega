// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package meshage

import (
	"bytes"
	"encoding/gob"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// pipeLink returns a link on one side of a pipe, and the encoder and decoder
// of the peer on the other side. A goroutine reads the ACKs of the link.
func pipeLink(t *testing.T, timeout time.Duration) (*link, *gob.Encoder, *gob.Decoder, net.Conn) {
	return pipeLinkKeepalive(t, timeout, time.Hour)
}

func pipeLinkKeepalive(t *testing.T, timeout, keepalive time.Duration) (*link, *gob.Encoder, *gob.Decoder, net.Conn) {
	t.Helper()

	a, b := net.Pipe()
	enc := gob.NewEncoder(&deadlineWriter{conn: a, timeout: timeout})
	in := &progressReader{conn: a, timeout: func() time.Duration { return time.Hour }}
	l := newLink(Control, a, enc, gob.NewDecoder(in), in, func() time.Duration { return timeout }, func() time.Duration { return keepalive })
	t.Cleanup(func() {
		l.close(errLinkClosed)
		b.Close()
	})

	go func() {
		for {
			var m Message
			if err := l.dec.Decode(&m); err != nil {
				l.close(err)
				return
			}
			if m.Command == ACK && m.Seq != 0 {
				l.acked(m.Seq)
			}
		}
	}()

	return l, gob.NewEncoder(b), gob.NewDecoder(b), b
}

// TestLinkWindow sends more than one message before the peer sends an ACK.
// With one message at a time, the peer would never get the second message.
func TestLinkWindow(t *testing.T) {
	const sends = 10

	l, enc, dec, conn := pipeLink(t, 5*time.Second)

	errs := make(chan error, sends)
	for i := 0; i < sends; i++ {
		go func(i int) {
			errs <- l.send(&Message{Command: MESSAGE, Body: strconv.Itoa(i)})
		}(i)
	}

	var seqs []uint64
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for len(seqs) < sends {
		var m Message
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("got %d messages before an ACK, want %d: %v", len(seqs), sends, err)
		}
		seqs = append(seqs, m.Seq)
	}

	for _, seq := range seqs {
		if err := enc.Encode(&Message{Command: ACK, Seq: seq}); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < sends; i++ {
		if err := <-errs; err != nil {
			t.Errorf("send: %v", err)
		}
	}
}

// TestLinkWindowLimit checks that at most WINDOW messages wait for an ACK.
func TestLinkWindowLimit(t *testing.T) {
	l, enc, dec, conn := pipeLink(t, 10*time.Second)

	errs := make(chan error, WINDOW+1)
	for i := 0; i < WINDOW+1; i++ {
		go func() {
			errs <- l.send(&Message{Command: MESSAGE})
		}()
	}

	var first uint64
	for i := 0; i < WINDOW; i++ {
		var m Message
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if i == 0 {
			first = m.Seq
		}
	}

	// The last message must wait for a free place in the window.
	var m Message
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if err := dec.Decode(&m); err == nil {
		t.Fatalf("got message %d with %d messages waiting for an ACK", m.Seq, WINDOW)
	}

	// After a read timeout, net.Pipe can still be read.
	if err := enc.Encode(&Message{Command: ACK, Seq: first}); err != nil {
		t.Fatal(err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("send: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("no message after an ACK freed a place: %v", err)
	}
}

// TestLinkTimeout checks that a message with no ACK fails with a timeout and
// closes the link.
func TestLinkTimeout(t *testing.T) {
	l, _, dec, _ := pipeLink(t, 200*time.Millisecond)

	go func() {
		for {
			var m Message
			if dec.Decode(&m) != nil {
				return
			}
		}
	}()

	start := time.Now()
	err := l.send(&Message{Command: MESSAGE})
	if !errors.Is(err, errTimeout) {
		t.Fatalf("send = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("timeout after %v, want about 200ms", d)
	}
	if !l.closed() {
		t.Error("link is open after a timeout")
	}
	if err := l.send(&Message{Command: MESSAGE}); err == nil {
		t.Error("send on a closed link succeeded")
	}
}

// slowWrite encodes the values with a new encoder and writes the result to
// conn in chunks, with a pause before each chunk.
func slowWrite(conn net.Conn, chunk int, pause time.Duration, values ...interface{}) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	for _, v := range values {
		if err := enc.Encode(v); err != nil {
			return err
		}
	}

	for data := buf.Bytes(); len(data) > 0; {
		time.Sleep(pause)

		end := min(chunk, len(data))
		if _, err := conn.Write(data[:end]); err != nil {
			return err
		}
		data = data[end:]
	}

	return nil
}

// TestLinkACKBehindLargeMessage checks that the link stays open while the
// peer writes a large message, which takes longer than the timeout, before
// the ACK.
func TestLinkACKBehindLargeMessage(t *testing.T) {
	const timeout = 200 * time.Millisecond

	l, _, dec, conn := pipeLink(t, timeout)

	errs := make(chan error, 1)
	go func() {
		errs <- l.send(&Message{Command: MESSAGE})
	}()

	var m Message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}

	// 64 KiB in 8 chunks, 100ms each: 800ms in total
	large := &Message{Command: MESSAGE, Body: strings.Repeat("x", 64<<10)}
	ack := &Message{Command: ACK, Seq: m.Seq}
	start := time.Now()
	if err := slowWrite(conn, 8<<10, timeout/2, large, ack); err != nil {
		t.Fatalf("write from the peer: %v", err)
	}
	if d := time.Since(start); d <= timeout {
		t.Fatalf("the write took %v; the test needs more than the timeout %v", d, timeout)
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("send = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no result from send after the ACK")
	}
	if l.closed() {
		t.Errorf("link closed: %v", l.closeErr())
	}
}

// TestProgressReader decodes a message that arrives in chunks over more time
// than the read deadline. A peer that stops writing makes the read fail.
func TestProgressReader(t *testing.T) {
	const timeout = 100 * time.Millisecond

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	in := &progressReader{conn: a, timeout: func() time.Duration { return timeout }}
	dec := gob.NewDecoder(in)

	want := strings.Repeat("y", 32<<10)
	go slowWrite(b, 4<<10, timeout/2, &Message{Command: MESSAGE, Body: want})

	start := time.Now()
	var m Message
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode = %v, want nil", err)
	}
	if d := time.Since(start); d <= timeout {
		t.Errorf("the message took %v; the test needs more than the timeout %v", d, timeout)
	}
	if m.Body != want {
		t.Errorf("got a body of %d bytes, want %d", len(m.Body.(string)), len(want))
	}
	if last := in.lastRead(); time.Since(last) > time.Second {
		t.Errorf("last read at %v, want after the start %v", last, start)
	}

	if err := dec.Decode(&m); err == nil {
		t.Error("Decode from a peer that does not write succeeded")
	}
}

// TestLinkACKFirst checks that the writer sends a queued ACK before the next
// message.
func TestLinkACKFirst(t *testing.T) {
	l, _, dec, conn := pipeLink(t, 5*time.Second)

	if err := l.ack(7); err != nil {
		t.Fatal(err)
	}
	go l.send(&Message{Command: MESSAGE})

	var m Message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Command != ACK || m.Seq != 7 {
		t.Errorf("first message = %v %d, want ACK 7", m.Command, m.Seq)
	}
}

// TestLinkKeepalive checks that an idle link sends a keepalive.
func TestLinkKeepalive(t *testing.T) {
	_, _, dec, conn := pipeLinkKeepalive(t, 5*time.Second, 50*time.Millisecond)

	for i := 0; i < 2; i++ {
		var m Message
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("keepalive %d: %v", i, err)
		}
		if m.Command != ACK || m.Seq != 0 {
			t.Errorf("keepalive %d = %v %d, want ACK 0", i, m.Command, m.Seq)
		}
	}
}

// TestDeadlineWriter writes more than one chunk to a peer that reads slowly.
// Each chunk gets a new deadline, so the write succeeds although it takes
// longer than the timeout.
func TestDeadlineWriter(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	const timeout = 100 * time.Millisecond
	data := make([]byte, 4*writeChunk)

	go func() {
		buf := make([]byte, writeChunk)
		for {
			time.Sleep(timeout / 2)
			if _, err := b.Read(buf); err != nil {
				return
			}
		}
	}()

	w := &deadlineWriter{conn: a, timeout: timeout}
	start := time.Now()
	if n, err := w.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v, want %d, nil", n, err, len(data))
	}
	if d := time.Since(start); d <= timeout {
		t.Errorf("the write took %v; the test needs more than the timeout %v", d, timeout)
	}

	// a peer that stops reading makes the write fail
	b.Close()
	a2, b2 := net.Pipe()
	defer a2.Close()
	defer b2.Close()

	w = &deadlineWriter{conn: a2, timeout: timeout}
	if _, err := w.Write(data); err == nil {
		t.Error("Write to a peer that does not read succeeded")
	}
}

// TestRegister checks that two nodes that dial each other at the same time
// keep the same control link: the one that the node with the lower name
// dialed.
func TestRegister(t *testing.T) {
	for _, tt := range []struct {
		node, peer string
		oldDialed  bool // this node dialed the existing link
		newDialer  string
		want       bool
	}{
		{"a", "b", true, "b", false},
		{"a", "b", false, "a", true},
		{"b", "a", true, "a", true},
		{"b", "a", false, "b", false},
		{"a", "b", false, "b", true},
		{"a", "b", true, "a", true},
	} {
		n := &Node{name: tt.node, clients: map[string]*client{}}

		old := &client{name: tt.peer, done: make(chan struct{})}
		if tt.oldDialed {
			old.addr = "x"
		}
		n.clients[tt.peer] = old

		c := &client{name: tt.peer, done: make(chan struct{})}
		if got := n.register(c, tt.newDialer); got != tt.want {
			t.Errorf("%+v: register = %v", tt, got)
		}

		want := old
		if tt.want {
			want = c
		}
		if n.clients[tt.peer] != want {
			t.Errorf("%+v: wrong client in the list", tt)
		}
	}
}

// freePort returns a port that is free for TCP and UDP, because a node
// listens on both.
func freePort(t *testing.T) int {
	t.Helper()

	for i := 0; i < 20; i++ {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port

		u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
		l.Close()
		if err == nil {
			u.Close()
			return port
		}
	}

	t.Fatal("no port that is free for TCP and UDP")
	return 0
}

// testNodes starts one node for each name. The nodes dial each other on the
// loopback address.
func testNodes(t *testing.T, names ...string) map[string]*Node {
	t.Helper()

	addrs := map[string]string{}
	for _, name := range names {
		addrs[name] = net.JoinHostPort("127.0.0.1", strconv.Itoa(freePort(t)))
	}

	var mu sync.Mutex
	nodes := map[string]*Node{}
	for _, name := range names {
		_, port, _ := net.SplitHostPort(addrs[name])
		p, _ := strconv.Atoi(port)

		n := NewNode(name, "test-"+t.Name(), 0, net.IPv4(127, 255, 255, 255), p, "test")
		n.resolve = func(host string) string {
			mu.Lock()
			defer mu.Unlock()

			if addr, ok := addrs[host]; ok {
				return addr
			}
			return host
		}
		nodes[name] = n
	}

	return nodes
}

// waitLinks waits until n has all the links to peer.
func waitLinks(t *testing.T, n *Node, peer string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(n.Links()[peer]) == int(numClasses) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("%v links to %v = %v, want all %d", n.name, peer, n.Links()[peer], numClasses)
}

func dialTest(t *testing.T, a, b *Node) {
	t.Helper()

	var err error
	for i := 0; i < 50; i++ {
		// the listener can start after the node
		if err = a.Dial(b.name); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}

	waitLinks(t, a, b.name)
	waitLinks(t, b, a.name)
}

// classifyPrefix uses the text before ":" in a string body as the class.
func classifyPrefix(body interface{}) Class {
	s, _ := body.(string)
	switch {
	case strings.HasPrefix(s, "bulk:"):
		return Bulk
	case strings.HasPrefix(s, "log:"):
		return Log
	}

	return Control
}

func receive(t *testing.T, n *Node, class Class) *Message {
	t.Helper()

	select {
	case m := <-n.Messages(class):
		return m
	case <-time.After(5 * time.Second):
		t.Fatalf("%v: no %v message", n.name, class)
	}

	return nil
}

func TestNodeLinks(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]
	a.Classify = classifyPrefix

	dialTest(t, a, b)

	for _, body := range []string{"control:1", "bulk:2", "log:3"} {
		class := classifyPrefix(body)

		if _, err := a.Set([]string{"b"}, body); err != nil {
			t.Fatalf("Set %v: %v", body, err)
		}

		m := receive(t, b, class)
		if m.Body != body || m.Class != class {
			t.Errorf("got %v on %v, want %v on %v", m.Body, m.Class, body, class)
		}
	}

	if err := a.Dial("b"); err == nil || !strings.Contains(err.Error(), "already connected") {
		t.Errorf("second dial = %v, want already connected", err)
	}
}

// TestNodeACKTimeout checks that the ACK timeout of a link is two MSA
// periods, and that it follows a change of the MSA period.
func TestNodeACKTimeout(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]

	dialTest(t, a, b)

	c, err := a.getClient("b")
	if err != nil {
		t.Fatal(err)
	}
	l := c.link(Control)

	if got, want := l.timeout(), 2*DEFAULT_MSA_TIMEOUT*time.Second; got != want {
		t.Errorf("ACK timeout = %v, want %v", got, want)
	}

	a.SetMSATimeout(30)
	if got, want := l.timeout(), time.Minute; got != want {
		t.Errorf("ACK timeout after the MSA change = %v, want %v", got, want)
	}
}

// TestDialSolicitedConnected checks that a solicited dial to a peer that is
// already connected does not log an error. A node with fewer peers than its
// degree solicits all the time, and each peer then dials it again.
func TestDialSolicitedConnected(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]

	dialTest(t, a, b)

	var buf safeBuffer
	log.AddLogger("solicited", &buf, log.ERROR, false)
	defer log.DelLogger("solicited")

	if err := a.dial("b", true); !errors.Is(err, errAlreadyConnected) {
		t.Errorf("solicited dial = %v, want already connected", err)
	}
	if out := buf.String(); out != "" {
		t.Errorf("solicited dial logged: %q", out)
	}
}

// safeBuffer is a bytes.Buffer that goroutines can write at the same time.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// TestNodeLinkFallback closes the bulk link. Bulk messages then use the
// control link until the node dials the bulk link again.
func TestNodeLinkFallback(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]
	a.Classify = classifyPrefix

	dialTest(t, a, b)

	c, err := a.getClient("b")
	if err != nil {
		t.Fatal(err)
	}
	c.lock.Lock()
	bulk, control := c.links[Bulk], c.links[Control]
	c.lock.Unlock()

	bulk.close(errors.New("test"))
	if l := c.link(Bulk); l != control {
		t.Fatal("bulk message does not use the control link after the bulk link closed")
	}

	if _, err := a.Set([]string{"b"}, "bulk:x"); err != nil {
		t.Fatal(err)
	}
	if m := receive(t, b, Bulk); m.Body != "bulk:x" {
		t.Errorf("got %v, want bulk:x", m.Body)
	}

	waitLinks(t, a, "b")
	waitLinks(t, b, "a")
}

// stallProxy forwards TCP connections to target. It can stop the data from
// the dialer on one connection.
type stallProxy struct {
	ln     net.Listener
	target string

	mu      sync.Mutex
	stalled map[string]bool // by the address of the dialer
	conns   []net.Conn
}

func newStallProxy(t *testing.T, target string) *stallProxy {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	p := &stallProxy{ln: ln, target: target, stalled: map[string]bool{}}
	t.Cleanup(func() {
		ln.Close()

		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conns {
			c.Close()
		}
	})

	go func() {
		for {
			down, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", target)
			if err != nil {
				down.Close()
				continue
			}

			p.mu.Lock()
			p.conns = append(p.conns, down, up)
			p.mu.Unlock()

			go p.forward(down.RemoteAddr().String(), up, down)
			go p.forward("", down, up)
		}
	}()

	return p
}

// forward copies src to dst. While the connection of dialer is stalled, it
// keeps the data that it read.
func (p *stallProxy) forward(dialer string, dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if err != nil {
			dst.Close()
			return
		}

		for p.isStalled(dialer) {
			time.Sleep(10 * time.Millisecond)
		}

		if _, err := dst.Write(buf[:n]); err != nil {
			src.Close()
			return
		}
	}
}

func (p *stallProxy) isStalled(dialer string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.stalled[dialer]
}

func (p *stallProxy) stall(dialer string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stalled[dialer] = true
}

// TestNodeBulkStall stops the data on the bulk link from a to b. Control and
// log messages must still arrive. With one link for all classes, they would
// wait behind the bulk message.
func TestNodeBulkStall(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]
	a.Classify = classifyPrefix

	proxy := newStallProxy(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(b.port)))
	a.resolve = func(string) string { return proxy.ln.Addr().String() }

	dialTest(t, a, b)

	c, err := a.getClient("b")
	if err != nil {
		t.Fatal(err)
	}
	proxy.stall(c.link(Bulk).conn.LocalAddr().String())

	stalled := make(chan error, 1)
	go func() {
		_, err := a.Set([]string{"b"}, "bulk:stalled")
		stalled <- err
	}()

	select {
	case err := <-stalled:
		t.Fatalf("Set on a stalled bulk link returned %v", err)
	case m := <-b.Messages(Bulk):
		t.Fatalf("got %v on a stalled bulk link", m.Body)
	case <-time.After(200 * time.Millisecond):
	}

	for _, body := range []string{"control:1", "log:2"} {
		class := classifyPrefix(body)

		start := time.Now()
		if _, err := a.Set([]string{"b"}, body); err != nil {
			t.Fatalf("Set %v with a stalled bulk link: %v", body, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("Set %v took %v with a stalled bulk link", body, d)
		}

		select {
		case m := <-b.Messages(class):
			if m.Body != body {
				t.Errorf("got %v, want %v", m.Body, body)
			}
		case <-time.After(time.Second):
			t.Fatalf("no %v message with a stalled bulk link", class)
		}
	}
}

// TestNodeForward sends messages of each class through a node in the middle.
func TestNodeForward(t *testing.T) {
	nodes := testNodes(t, "a", "b", "c")
	a, b, c := nodes["a"], nodes["b"], nodes["c"]
	a.Classify = classifyPrefix

	dialTest(t, a, b)
	dialTest(t, b, c)

	deadline := time.Now().Add(15 * time.Second)
	for {
		a.checkUpdateNetwork()
		a.meshLock.Lock()
		route := a.routes["c"]
		a.meshLock.Unlock()
		if route == "b" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a has no route to c through b: %q", route)
		}
		time.Sleep(50 * time.Millisecond)
	}

	for _, body := range []string{"control:1", "bulk:2", "log:3"} {
		if _, err := a.Set([]string{"c"}, body); err != nil {
			t.Fatalf("Set %v: %v", body, err)
		}
		if m := receive(t, c, classifyPrefix(body)); m.Body != body {
			t.Errorf("got %v, want %v", m.Body, body)
		}
	}
}

// TestNodeHangup checks that all the links close with the control link.
func TestNodeHangup(t *testing.T) {
	nodes := testNodes(t, "a", "b")
	a, b := nodes["a"], nodes["b"]

	dialTest(t, a, b)

	if err := b.Hangup("a"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for len(a.Links()) > 0 || len(b.Links()) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("links after hangup: a %v, b %v", a.Links(), b.Links())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestDialOldPeer dials a peer that sends the handshake of an older version.
func TestDialOldPeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		enc := gob.NewEncoder(conn)
		enc.Encode("old")
		enc.Encode(true)
		enc.Encode("old-version")
		time.Sleep(time.Second)
	}()

	nodes := testNodes(t, "a")
	a := nodes["a"]
	a.resolve = func(string) string { return ln.Addr().String() }

	err = a.Dial("old")
	if err == nil || !strings.Contains(err.Error(), "older mesh protocol") {
		t.Fatalf("Dial = %v, want an older mesh protocol error", err)
	}
}
