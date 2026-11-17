// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
)

const (
	uuidA = "3b440429-067f-5b75-a0c3-f436519f8ccb"
	uuidB = "a5ba6920-5bcf-4022-b8cf-015425f7b05c"
)

type fakeVM struct {
	uuid   string
	name   string
	policy string

	mu   sync.Mutex
	tags map[string]string
}

func (v *fakeVM) GetNamespace() string {
	return ""
}

func (v *fakeVM) GetUUID() string {
	return v.uuid
}

func (v *fakeVM) SetCCActive(bool) {
}

func (v *fakeVM) Info(k string) (string, error) {
	if k == "name" {
		return v.name, nil
	}
	return "", nil
}

func (v *fakeVM) CCPolicy() string {
	return v.policy
}

func (v *fakeVM) GetTags() map[string]string {
	v.mu.Lock()
	defer v.mu.Unlock()

	res := map[string]string{}
	for k, val := range v.tags {
		res[k] = val
	}
	return res
}

func (v *fakeVM) SetTag(k, val string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.tags[k] = val
}

func (v *fakeVM) tag(k string) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.tags[k]
}

// plainVM does not implement CCPolicy.
type plainVM struct{ *fakeVM }

func (v plainVM) CCPolicy() {}

type testServer struct {
	*Server
	sockDir string
}

func newTestServer(t *testing.T, onMessage func(*Message)) *testServer {
	t.Helper()

	s, err := NewServer(t.TempDir(), "", miniplumber.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	s.onMessage = onMessage

	// unix socket paths must be short
	sockDir, err := os.MkdirTemp("", "ron")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		s.Destroy()
		os.RemoveAll(sockDir)
	})

	return &testServer{Server: s, sockDir: sockDir}
}

// bind registers a VM and creates a bound listener for it.
func (s *testServer) bind(t *testing.T, uuid, policy string) (*fakeVM, string) {
	t.Helper()

	vm := &fakeVM{uuid: uuid, policy: policy, tags: map[string]string{}}
	s.RegisterVM(vm)

	path := filepath.Join(s.sockDir, uuid[:8]+".sock")
	if err := s.ListenUnixBound(path, uuid, -1); err != nil {
		t.Fatal(err)
	}

	return vm, path
}

type guardClient struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

// handshake runs the client side of the ron handshake on conn.
func handshake(t *testing.T, conn net.Conn, uuid string) (*guardClient, *Message, error) {
	t.Helper()

	tc := &guardClient{conn: conn, enc: gob.NewEncoder(conn), dec: gob.NewDecoder(conn)}

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})

	if _, err := io.WriteString(conn, "RON"); err != nil {
		return tc, nil, err
	}

	var buf [3]byte
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		return tc, nil, err
	}
	if string(buf[:]) != "RON" {
		t.Fatalf("magic bytes = %q", buf[:])
	}

	m := &Message{
		Type:    MESSAGE_CLIENT,
		UUID:    uuid,
		Version: "v1",
		Client:  &Client{UUID: uuid, Hostname: "guest"},
	}
	if err := tc.enc.Encode(m); err != nil {
		return tc, nil, err
	}

	var reply Message
	if err := tc.dec.Decode(&reply); err != nil {
		return tc, nil, err
	}

	return tc, &reply, nil
}

func dialClient(t *testing.T, path, uuid string) (*guardClient, *Message, error) {
	t.Helper()

	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { conn.Close() })

	return handshake(t, conn, uuid)
}

func mustDial(t *testing.T, s *testServer, path, uuid, want string) *guardClient {
	t.Helper()

	tc, reply, err := dialClient(t, path, uuid)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if reply.Client == nil || reply.Client.UUID != want {
		t.Fatalf("handshake reply %+v, want UUID %v", reply.Client, want)
	}
	waitFor(t, "client registered", func() bool { return s.HasClient(want) })

	return tc
}

func (tc *guardClient) send(t *testing.T, m *Message) {
	t.Helper()

	if err := tc.enc.Encode(m); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// recv returns the next message that is not a heartbeat.
func (tc *guardClient) recv() (*Message, error) {
	tc.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var m Message
		if err := tc.dec.Decode(&m); err != nil {
			return nil, err
		}
		if m.Type != MESSAGE_HEARTBEAT {
			return &m, nil
		}
	}
}

// waitClosed waits until the server closes the connection.
func (tc *guardClient) waitClosed(t *testing.T) {
	t.Helper()

	for {
		_, err := tc.recv()
		if err == nil {
			continue
		}

		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatal("server did not close the connection")
		}
		return
	}
}

// setTag sends a heartbeat that sets a tag, and waits until vm has it.
func (tc *guardClient) setTag(t *testing.T, vm *fakeVM, k, v string) {
	t.Helper()

	tc.send(t, &Message{
		Type:   MESSAGE_CLIENT,
		Client: &Client{Hostname: "guest", Tags: map[string]string{k: v}},
	})
	waitFor(t, "tag "+k, func() bool { return vm.tag(k) == v })
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %v", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBoundListenerBindsEmptyUUID(t *testing.T) {
	s := newTestServer(t, nil)
	_, path := s.bind(t, uuidA, CCPolicyStrict)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("socket mode = %v, want 0600", fi.Mode().Perm())
	}

	mustDial(t, s, path, "", uuidA)
}

func TestBoundListenerOwner(t *testing.T) {
	uid := os.Getuid()
	if os.Geteuid() == 0 {
		uid = 4242
	} else if os.Getgid() != uid {
		t.Skipf("uid %v cannot give a file the group %v", uid, uid)
	}

	s := newTestServer(t, nil)
	s.RegisterVM(&fakeVM{uuid: uuidA, policy: CCPolicyStrict, tags: map[string]string{}})

	path := filepath.Join(s.sockDir, "owner.sock")
	if err := s.ListenUnixBound(path, uuidA, uid); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	if int(st.Uid) != uid || int(st.Gid) != uid || fi.Mode().Perm() != 0600 {
		t.Fatalf("socket is %v:%v with mode %v, want %v:%v with mode 0600", st.Uid, st.Gid, fi.Mode().Perm(), uid, uid)
	}

	mustDial(t, s, path, "", uuidA)
}

func TestBoundListenerAcceptsOwnUUID(t *testing.T) {
	s := newTestServer(t, nil)
	_, path := s.bind(t, uuidA, CCPolicyStrict)

	mustDial(t, s, path, uuidA, uuidA)
}

func TestBoundListenerRejectsOtherUUID(t *testing.T) {
	s := newTestServer(t, nil)
	_, path := s.bind(t, uuidA, CCPolicyStrict)
	s.RegisterVM(&fakeVM{uuid: uuidB, tags: map[string]string{}})

	if _, _, err := dialClient(t, path, uuidB); err == nil {
		t.Fatal("handshake with another VM's UUID succeeded")
	}
	if s.HasClient(uuidA) || s.HasClient(uuidB) {
		t.Fatal("refused client was registered")
	}

	// the listener keeps serving the VM's own client
	mustDial(t, s, path, "", uuidA)
}

func TestBoundListenerReconnectAndClose(t *testing.T) {
	s := newTestServer(t, nil)
	_, path := s.bind(t, uuidA, CCPolicyStrict)

	tc := mustDial(t, s, path, "", uuidA)
	tc.conn.Close()
	waitFor(t, "client removed", func() bool { return !s.HasClient(uuidA) })

	tc = mustDial(t, s, path, "", uuidA)

	if err := s.CloseUnix(path); err != nil {
		t.Fatal(err)
	}
	tc.waitClosed(t)
	waitFor(t, "client removed", func() bool { return !s.HasClient(uuidA) })

	if _, _, err := dialClient(t, path, ""); err == nil {
		t.Fatal("closed listener accepted a client")
	}

	// the same path can be bound again, as for a relaunched VM
	if err := s.ListenUnixBound(path, uuidA, -1); err != nil {
		t.Fatal(err)
	}
	mustDial(t, s, path, "", uuidA)
}

func TestDialSerialRejectsOtherUUID(t *testing.T) {
	s := newTestServer(t, nil)
	// let the reconnect loop see the unregistered VM before the test ends
	s.reconnectRate = 10 * time.Millisecond

	vm := &fakeVM{uuid: uuidA, tags: map[string]string{}}
	s.RegisterVM(vm)
	s.RegisterVM(&fakeVM{uuid: uuidB, tags: map[string]string{}})

	path := filepath.Join(s.sockDir, "serial")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if err := s.DialSerial(path, uuidA); err != nil {
		t.Fatal(err)
	}

	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, _, err := handshake(t, conn, uuidB); err == nil {
		t.Fatal("serial handshake with another VM's UUID succeeded")
	}
	if s.HasClient(uuidA) || s.HasClient(uuidB) {
		t.Fatal("refused client was registered")
	}

	// stop the reconnect loop
	s.UnregisterVM(vm)
}

func TestClientMessageBoundToConnection(t *testing.T) {
	s := newTestServer(t, nil)
	vmA, pathA := s.bind(t, uuidA, CCPolicyStrict)
	vmB, pathB := s.bind(t, uuidB, CCPolicyLegacy)

	a := mustDial(t, s, pathA, "", uuidA)
	mustDial(t, s, pathB, "", uuidB)

	a.send(t, &Message{
		Type: MESSAGE_CLIENT,
		UUID: uuidB,
		Client: &Client{
			UUID:     uuidB,
			Hostname: "evil",
			Tags:     map[string]string{"owner": "a"},
		},
	})

	waitFor(t, "tag on A", func() bool { return vmA.tag("owner") == "a" })

	if v := vmB.tag("owner"); v != "" {
		t.Fatalf("client A set a tag on VM B: %q", v)
	}

	clients := s.GetClients()
	if clients[uuidB].Hostname != "guest" {
		t.Fatalf("client A changed client B: %+v", clients[uuidB])
	}
	if clients[uuidA].Hostname != "evil" {
		t.Fatalf("client A update was not applied to A: %+v", clients[uuidA])
	}
}

func TestMalformedMessagesCloseConnection(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)
	vmB, pathB := s.bind(t, uuidB, CCPolicyLegacy)

	b := mustDial(t, s, pathB, "", uuidB)

	outside := t.TempDir()
	abs := filepath.Join(outside, "abs")

	tests := map[string]*Message{
		"file without file":   {Type: MESSAGE_FILE},
		"client without data": {Type: MESSAGE_CLIENT},
		"tag with NUL":        {Type: MESSAGE_CLIENT, Client: &Client{Tags: map[string]string{"k": "v\x00"}}},
		"long hostname":       {Type: MESSAGE_CLIENT, Client: &Client{Hostname: strings.Repeat("h", 1000)}},
		"control character":   {Type: MESSAGE_CLIENT, Client: &Client{IPs: []string{"10.0.0.1\n"}}},
		"pipe with slash":     {Type: MESSAGE_PIPE, Pipe: "other//pipe", PipeMode: PIPE_NEW_WRITER},
		"pipe mode":           {Type: MESSAGE_PIPE, Pipe: "pipe", PipeMode: 42},
		"unknown type":        {Type: 42},
		"negative offset":     {Type: MESSAGE_FILE, File: &File{ID: 1, Name: "x", Data: []byte("a"), Offset: -1}},
		"big chunk":           {Type: MESSAGE_FILE, File: &File{ID: 1, Name: "x", Data: make([]byte, MaxFileChunk+1)}},
		"big file":            {Type: MESSAGE_FILE, File: &File{ID: 1, Name: "x", Data: []byte("a"), Offset: MaxUploadFileSize}},
		"upload with dotdot":  {Type: MESSAGE_FILE, File: &File{ID: 1, Name: "../x", Data: []byte("a"), EOF: true}},
		"absolute upload":     {Type: MESSAGE_FILE, File: &File{ID: 1, Name: abs, Data: []byte("a"), EOF: true}},
	}

	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			a := mustDial(t, s, pathA, "", uuidA)
			a.send(t, m)
			a.waitClosed(t)
			waitFor(t, "client removed", func() bool { return !s.HasClient(uuidA) })

			// the other client is still served
			b.setTag(t, vmB, "seq", name)
		})
	}

	id := 1
	if _, err := os.Stat(filepath.Join(s.responsePath(&id), "x")); err == nil {
		t.Fatal("upload left its response directory")
	}
	if _, err := os.Stat(abs); err == nil {
		t.Fatal("absolute upload was written")
	}
}

func TestOversizeMessageClosesConnection(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)
	vmB, pathB := s.bind(t, uuidB, CCPolicyLegacy)

	a := mustDial(t, s, pathA, "", uuidA)
	b := mustDial(t, s, pathB, "", uuidB)

	// the server closes the connection part way, so the write can fail
	go a.enc.Encode(&Message{Type: MESSAGE_TUNNEL, Tunnel: make([]byte, MaxMessageSize+1)})

	a.waitClosed(t)
	waitFor(t, "client removed", func() bool { return !s.HasClient(uuidA) })

	b.setTag(t, vmB, "alive", "yes")
}

func TestHandlerPanicRecovered(t *testing.T) {
	s := newTestServer(t, func(m *Message) {
		if m.Type == MESSAGE_PIPE && m.Pipe == "boom" {
			panic("boom")
		}
	})
	vmA, pathA := s.bind(t, uuidA, CCPolicyStrict)
	vmB, pathB := s.bind(t, uuidB, CCPolicyLegacy)

	a := mustDial(t, s, pathA, "", uuidA)
	b := mustDial(t, s, pathB, "", uuidB)

	a.send(t, &Message{Type: MESSAGE_PIPE, Pipe: "boom", PipeMode: PIPE_NEW_WRITER})
	a.waitClosed(t)
	waitFor(t, "client removed", func() bool { return !s.HasClient(uuidA) })

	b.setTag(t, vmB, "alive", "yes")

	a = mustDial(t, s, pathA, "", uuidA)
	a.setTag(t, vmA, "alive", "again")
}

// requestFile asks for name and returns the data, or the error that the
// server sent.
func requestFile(t *testing.T, tc *guardClient, name string) (string, string) {
	t.Helper()

	tc.send(t, &Message{Type: MESSAGE_FILE, File: &File{Name: name}})

	var data []byte
	for {
		m, err := tc.recv()
		if err != nil {
			t.Fatalf("request %q: %v", name, err)
		}
		if m.Type != MESSAGE_FILE {
			continue
		}
		if m.Error != "" {
			return "", m.Error
		}
		data = append(data, m.File.Data...)
		if m.File.EOF {
			return string(data), ""
		}
	}
}

func TestFileRequestConfined(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.path, "ok.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(s.path, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.path, "sub")); err != nil {
		t.Fatal(err)
	}

	a := mustDial(t, s, pathA, "", uuidA)

	if data, errMsg := requestFile(t, a, "ok.txt"); errMsg != "" || data != "hello" {
		t.Fatalf("ok.txt = %q, %q", data, errMsg)
	}

	rel, err := filepath.Rel(s.path, filepath.Join(outside, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{rel, filepath.Join(outside, "secret.txt"), "link", "sub/secret.txt"} {
		if data, errMsg := requestFile(t, a, name); errMsg == "" || data != "" {
			t.Fatalf("request %q was not refused: %q", name, data)
		}
	}

	if !s.HasClient(uuidA) {
		t.Fatal("refused file requests closed the connection")
	}
}

func TestFileUploadConfinedAndMasked(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)

	id := 1
	dir := filepath.Join(s.responsePath(&id), uuidA)

	readFile := func(name string) func() bool {
		return func() bool {
			_, err := os.Stat(filepath.Join(dir, name))
			return err == nil
		}
	}

	s.NewCommand(&Command{})
	a := mustDial(t, s, pathA, "", uuidA)

	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{
		ID:   1,
		Name: "bin/tool",
		Perm: os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0777,
		Data: []byte("x"),
		EOF:  true,
	}})
	waitFor(t, "bin/tool", readFile("bin/tool"))

	fi, err := os.Stat(filepath.Join(dir, "bin/tool"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || fi.Mode().Perm()&0022 != 0 || fi.Mode().Perm()&0100 == 0 {
		t.Fatalf("bin/tool mode = %v", fi.Mode())
	}

	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{ID: 1, Name: "data", Perm: 0666, Data: []byte("ab")}})
	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{ID: 1, Name: "data", Perm: 0666, Data: []byte("cd"), Offset: 2, EOF: true}})
	waitFor(t, "data", readFile("data"))

	b, err := os.ReadFile(filepath.Join(dir, "data"))
	if err != nil || string(b) != "abcd" {
		t.Fatalf("data = %q, %v", b, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "data")); err != nil || fi.Mode().Perm()&0133 != 0 {
		t.Fatalf("data mode = %v, %v", fi.Mode(), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data.partial")); err == nil {
		t.Fatal("data.partial was not renamed")
	}

	// a symlink inside the response directory cannot lead outside
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{ID: 1, Name: "link/evil", Data: []byte("x"), EOF: true}})
	a.waitClosed(t)

	if _, err := os.Stat(filepath.Join(outside, "evil")); err == nil {
		t.Fatal("upload followed a symlink out of the response directory")
	}
}

func TestPipeLimit(t *testing.T) {
	s := newTestServer(t, nil)
	vmA, pathA := s.bind(t, uuidA, CCPolicyStrict)

	a := mustDial(t, s, pathA, "", uuidA)

	for i := 0; i <= MaxPipes; i++ {
		a.send(t, &Message{Type: MESSAGE_PIPE, Pipe: fmt.Sprintf("p%d", i), PipeMode: PIPE_NEW_WRITER})
	}
	a.setTag(t, vmA, "sync", "1")

	s.clientLock.Lock()
	c := s.clients[uuidA]
	s.clientLock.Unlock()
	if c == nil {
		t.Fatal("pipe limit closed the connection")
	}

	c.pipeLock.Lock()
	n := len(c.pipeWriters)
	_, last := c.pipeWriters[fmt.Sprintf("p%d", MaxPipes)]
	c.pipeLock.Unlock()

	if n != MaxPipes || last {
		t.Fatalf("%d pipes open (last %v), want %d", n, last, MaxPipes)
	}
}

// TestPipeDataShutdown checks that shutdown does not wait for a PIPE_DATA
// write that a pipe reader blocks.
func TestPipeDataShutdown(t *testing.T) {
	plumber := miniplumber.New(nil)

	// a reader that never reads
	r := plumber.NewReader("p")
	defer r.Close()

	conn, peer := net.Pipe()
	defer peer.Close()

	c := &client{
		Client:      &Client{UUID: uuidA},
		uuid:        uuidA,
		conn:        conn,
		pipeReaders: map[string]*miniplumber.Reader{},
		pipeWriters: map[string]chan<- string{},
		pipesDone:   make(chan struct{}),
	}

	if err := c.pipeHandler(plumber, &Message{Type: MESSAGE_PIPE, Pipe: "p", PipeMode: PIPE_NEW_WRITER}); err != nil {
		t.Fatal(err)
	}

	// The writer of the plumber takes the first value and blocks on the
	// reader, so the second write blocks in pipeHandler.
	written := make(chan struct{})
	go func() {
		defer close(written)
		for i := 0; i < 2; i++ {
			c.pipeHandler(plumber, &Message{Type: MESSAGE_PIPE, Pipe: "p", PipeMode: PIPE_DATA, PipeData: "x"})
		}
	}()
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		c.shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown waits for a blocked PIPE_DATA write")
	}

	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("pipeHandler did not return after shutdown")
	}
}

func TestTunnelPolicy(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)
	_, pathB := s.bind(t, uuidB, CCPolicyLegacy)

	mustDial(t, s, pathA, "", uuidA)
	mustDial(t, s, pathB, "", uuidB)

	s.clientLock.Lock()
	a, b := s.clients[uuidA], s.clients[uuidB]
	s.clientLock.Unlock()

	if a.policy != CCPolicyStrict || b.policy != CCPolicyLegacy {
		t.Fatalf("policies = %v, %v", a.policy, b.policy)
	}

	strict := s.tunnelPolicy(a)
	if !strict.DenyForward || strict.AllowConnect == nil || strict.MaxStreams != MaxTunnelStreams {
		t.Fatalf("strict policy = %+v", strict)
	}
	if strict.AllowConnect("127.0.0.1", 8080) {
		t.Fatal("strict policy allowed a connect without a reverse tunnel")
	}

	a.allowReverse("127.0.0.1", 8080)
	if !strict.AllowConnect("127.0.0.1", 8080) {
		t.Fatal("strict policy refused the destination of a reverse tunnel")
	}
	if strict.AllowConnect("127.0.0.1", 8081) || strict.AllowConnect("10.0.0.1", 8080) {
		t.Fatal("strict policy allowed another destination")
	}
	a.disallowReverse("127.0.0.1", 8080)
	if strict.AllowConnect("127.0.0.1", 8080) {
		t.Fatal("strict policy kept a removed reverse tunnel")
	}

	legacy := s.tunnelPolicy(b)
	if legacy.DenyForward || legacy.AllowConnect != nil || legacy.MaxStreams != MaxTunnelStreams {
		t.Fatalf("legacy policy = %+v", legacy)
	}
}

func TestVMPolicy(t *testing.T) {
	for i, tt := range []struct {
		vm   VM
		want string
	}{
		{&fakeVM{policy: CCPolicyStrict}, CCPolicyStrict},
		{&fakeVM{policy: CCPolicyLegacy}, CCPolicyLegacy},
		{&fakeVM{policy: ""}, CCPolicyLegacy},
		{plainVM{&fakeVM{policy: CCPolicyStrict}}, CCPolicyLegacy},
	} {
		if got := vmPolicy(tt.vm); got != tt.want {
			t.Errorf("case %d: vmPolicy(%T) = %v, want %v", i, tt.vm, got, tt.want)
		}
	}
}

func TestOversizeHandshakeRefused(t *testing.T) {
	s := newTestServer(t, nil)
	_, pathA := s.bind(t, uuidA, CCPolicyStrict)

	conn, err := net.Dial("unix", pathA)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := io.WriteString(conn, "RON"); err != nil {
		t.Fatal(err)
	}
	var buf [3]byte
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		t.Fatal(err)
	}

	// valid tags, but more than MaxHandshakeSize
	tags := map[string]string{}
	for i := 0; i*maxTagValueLen <= MaxHandshakeSize; i++ {
		tags[fmt.Sprint(i)] = strings.Repeat("v", maxTagValueLen)
	}

	go gob.NewEncoder(conn).Encode(&Message{
		Type:    MESSAGE_CLIENT,
		Version: "v1",
		Client:  &Client{Hostname: "guest", Tags: tags},
	})

	var reply Message
	if err := gob.NewDecoder(conn).Decode(&reply); err == nil {
		t.Fatalf("oversize handshake was accepted: %+v", reply.Client)
	}
	if s.HasClient(uuidA) {
		t.Fatal("oversize handshake added a client")
	}

	mustDial(t, s, pathA, "", uuidA)
}

func TestLargeMessageSlots(t *testing.T) {
	slots := make(chan struct{}, 1)
	done := make(chan struct{})
	data := bytes.Repeat([]byte("x"), 64)

	newReader := func() *limitReader {
		l := &limitReader{r: bytes.NewReader(data), limit: 64, small: 16, slots: slots, done: done}
		l.reset()
		return l
	}

	a := newReader()
	if n, err := a.Read(make([]byte, 64)); n != 16 || err != nil {
		t.Fatalf("read below the slot size = %v, %v; want 16, nil", n, err)
	}
	if _, err := a.Read(make([]byte, 64)); err != nil || !a.held {
		t.Fatalf("read above the slot size: %v, held %v", err, a.held)
	}

	b := newReader()
	b.Read(make([]byte, 16))

	got := make(chan error, 1)
	go func() {
		_, err := b.Read(make([]byte, 16))
		got <- err
	}()

	select {
	case err := <-got:
		t.Fatalf("second large read did not wait for a slot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	a.release()
	if err := <-got; err != nil || !b.held {
		t.Fatalf("second large read after release: %v, held %v", err, b.held)
	}

	c := newReader()
	c.Read(make([]byte, 16))
	close(done)
	if _, err := c.Read(make([]byte, 16)); err == nil {
		t.Fatal("read waiting for a slot did not stop when the client closed")
	}
	b.reset()
}

// TestWriteTimeoutChange checks that a test can change writeTimeout while
// another goroutine writes to a client. Run it with -race.
func TestWriteTimeoutChange(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	go io.Copy(io.Discard, clientConn)

	c := &client{
		Client: &Client{UUID: "client"},
		conn:   serverConn,
		enc:    gob.NewEncoder(serverConn),
		uuid:   "client",
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			c.writeMu.Lock()
			c.encode(&Message{Type: MESSAGE_COMMAND})
			c.writeMu.Unlock()
		}
	}()

	old := writeTimeout.Load()
	for i := 0; i < 100; i++ {
		writeTimeout.Store(int64(time.Second + time.Duration(i)))
	}
	writeTimeout.Store(old)

	<-done
}

func TestWriteTimeoutClosesClient(t *testing.T) {
	old := writeTimeout.Swap(int64(200 * time.Millisecond))
	defer writeTimeout.Store(old)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	s := &Server{
		commands: map[int]*Command{1: {ID: 1, Command: []string{"true"}}},
		clients: map[string]*client{
			"client": {
				Client: &Client{UUID: "client"},
				conn:   serverConn,
				enc:    gob.NewEncoder(serverConn),
				uuid:   "client",
			},
		},
	}

	routed := make(chan struct{})
	go func() {
		defer close(routed)
		s.sendCommands("")
	}()

	// route does not hold clientLock while it writes
	time.Sleep(50 * time.Millisecond)
	locked := make(chan struct{})
	go func() {
		s.Clients()
		close(locked)
	}()
	select {
	case <-locked:
	case <-time.After(time.Duration(writeTimeout.Load()) / 2):
		t.Fatal("clientLock is held during a write")
	}

	select {
	case <-routed:
	case <-time.After(5 * time.Second):
		t.Fatal("write to a client that does not read did not time out")
	}

	if _, err := serverConn.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("connection after the write timeout: %v", err)
	}
	if got := s.GetCommand(1).Issued; got != 0 {
		t.Fatalf("issued = %d, want 0", got)
	}
}

func TestResponsesForDeadCommandsDropped(t *testing.T) {
	s := newTestServer(t, nil)

	id := s.NewCommand(&Command{})
	dead := id + 1

	r := &Client{UUID: uuidA, Responses: []*Response{
		{ID: id, Stdout: "out", RecordExitCode: true},
		{ID: dead, Stdout: "out", RecordExitCode: true},
	}}
	s.handleResponses(r)
	s.handleResponses(r)

	if _, err := os.Stat(filepath.Join(s.responsePath(&id), uuidA, "stdout")); err != nil {
		t.Fatalf("response to a live command: %v", err)
	}
	if _, err := os.Stat(s.responsePath(&dead)); err == nil {
		t.Fatal("response to a command that does not exist was written")
	}
	if got := s.GetCommand(id).CheckedIn; len(got) != 1 {
		t.Fatalf("CheckedIn = %v, want one entry", got)
	}
}

func TestFileUploadNeedsLiveCommand(t *testing.T) {
	s := newTestServer(t, nil)
	vmA, pathA := s.bind(t, uuidA, CCPolicyStrict)

	id := s.NewCommand(&Command{})
	dead := id + 1

	a := mustDial(t, s, pathA, "", uuidA)
	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{ID: dead, Name: "x", Data: []byte("a"), EOF: true}})
	a.send(t, &Message{Type: MESSAGE_FILE, File: &File{ID: id, Name: "deep/y", Data: []byte("a"), Offset: 1, EOF: true}})
	a.setTag(t, vmA, "sync", "1")

	if !s.HasClient(uuidA) {
		t.Fatal("dropped uploads closed the connection")
	}
	if _, err := os.Stat(s.responsePath(&dead)); err == nil {
		t.Fatal("upload for a command that does not exist was written")
	}
	if _, err := os.Stat(filepath.Join(s.responsePath(&id), uuidA, "deep")); err == nil {
		t.Fatal("upload that did not start at offset 0 made a directory")
	}

	if !s.chargeEntries(uuidB, MaxUploadEntriesPerHour) || s.chargeEntries(uuidB, 1) {
		t.Fatal("entry limit not applied")
	}
}

func TestVMTagLimits(t *testing.T) {
	s := newTestServer(t, nil)
	vmA, _ := s.bind(t, uuidA, CCPolicyStrict)
	vmB, _ := s.bind(t, uuidB, CCPolicyStrict)

	s.clientLock.Lock()
	s.clients[uuidA] = &client{Client: &Client{UUID: uuidA}, uuid: uuidA}
	s.clients[uuidB] = &client{Client: &Client{UUID: uuidB}, uuid: uuidB}
	s.clientLock.Unlock()

	// Destroy waits for the clients
	t.Cleanup(func() {
		s.clientLock.Lock()
		delete(s.clients, uuidA)
		delete(s.clients, uuidB)
		s.clientLock.Unlock()
	})

	for i := 0; i <= MaxVMTags/maxListLen; i++ {
		tags := map[string]string{}
		for j := 0; j < maxListLen; j++ {
			tags[fmt.Sprintf("%d-%d", i, j)] = "v"
		}
		s.updateClient(&Client{UUID: uuidA, Tags: tags})
	}
	if n := len(vmA.GetTags()); n != MaxVMTags {
		t.Fatalf("%d tags, want %d", n, MaxVMTags)
	}

	// a client can replace a tag at the count limit
	s.updateClient(&Client{UUID: uuidA, Tags: map[string]string{"0-0": "w"}})
	if vmA.tag("0-0") != "w" {
		t.Fatal("tag replacement refused")
	}

	big := strings.Repeat("v", maxTagValueLen)
	for i := 0; i*maxTagValueLen <= 2*MaxVMTagBytes; i++ {
		s.updateClient(&Client{UUID: uuidB, Tags: map[string]string{fmt.Sprint(i): big}})
	}
	size := 0
	for k, v := range vmB.GetTags() {
		size += len(k) + len(v)
	}
	if size > MaxVMTagBytes || size < MaxVMTagBytes-maxTagValueLen-16 {
		t.Fatalf("%d bytes of tags, want at most %d", size, MaxVMTagBytes)
	}
}

func TestResponseQueueBoundedByBytes(t *testing.T) {
	s := newTestServer(t, nil)

	s.queueWait(MaxQueuedResponses)

	done := make(chan struct{})
	go func() {
		s.queueWait(1)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("queue accepted more than MaxQueuedResponses bytes")
	case <-time.After(100 * time.Millisecond):
	}

	s.queueDone(MaxQueuedResponses)
	<-done
	s.queueDone(1)
}

func TestValidateClientUpdateFields(t *testing.T) {
	valid := &Client{Capabilities: []string{CapAutoUpdate}, SHA256: strings.Repeat("a", 64)}
	if err := validateClient(valid); err != nil {
		t.Fatalf("validateClient(%+v) = %v, want nil", valid, err)
	}

	for name, cl := range map[string]*Client{
		"long sha256":       {SHA256: strings.Repeat("a", maxFieldLen+1)},
		"long capability":   {Capabilities: []string{strings.Repeat("a", maxFieldLen+1)}},
		"many capabilities": {Capabilities: make([]string, maxListLen+1)},
	} {
		if err := validateClient(cl); err == nil {
			t.Errorf("%v: validateClient = nil, want an error", name)
		}
	}
}
