package gonetflow

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// v5 packet with one record, tagged in the first record byte.
func testPacket(tag byte) []byte {
	b := make([]byte, NETFLOW_HEADER_LEN+NETFLOW_RECORD_LEN)
	b[1] = 5
	b[3] = 1
	b[NETFLOW_HEADER_LEN] = tag
	return b
}

func sendLoop(t *testing.T, port int, stop <-chan struct{}) {
	c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Error(err)
		return
	}
	defer c.Close()
	for {
		select {
		case <-stop:
			return
		default:
			c.Write(testPacket(1))
		}
	}
}

// Adding/removing writers and stopping while flows arrive must not race or panic.
func TestWriterChurnWhileReceiving(t *testing.T) {
	nf, port, err := NewNetflow()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := nf.NewFileWriter(filepath.Join(dir, "base"), RAW, false); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { sendLoop(t, port, stop); close(done) }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for i := 0; time.Now().Before(deadline); i++ {
		p := filepath.Join(dir, fmt.Sprintf("f%d", i))
		if err := nf.NewFileWriter(p, RAW, false); err != nil {
			t.Fatal(err)
		}
		_ = nf.HasWriter()
		if err := nf.RemoveWriter(p); err != nil {
			t.Fatal(err)
		}
	}
	nf.Stop()
	close(stop)
	<-done
}

// A writer must see each datagram as it arrived, not as later overwritten.
func TestRawPacketsNotAliased(t *testing.T) {
	nf, _, err := NewNetflow()
	if err != nil {
		t.Fatal(err)
	}
	defer nf.Stop()

	c := make(chan *Packet, BUFFER_DEPTH)
	nf.registerWriter("t", c)

	b := make([]byte, UDP_BUFFER_DEPTH)
	var got []*Packet
	for tag := byte(1); tag <= 3; tag++ {
		copy(b, testPacket(tag))
		p, err := nf.process(len(testPacket(tag)), b)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, p)
	}
	for i, p := range got {
		if want := byte(i + 1); p.Raw[NETFLOW_HEADER_LEN] != want {
			t.Errorf("packet %d raw tag = %d, want %d", i, p.Raw[NETFLOW_HEADER_LEN], want)
		}
	}
}

// A writer that stops draining its queue must not block RemoveWriter.
func TestStalledWriterDoesNotBlockRemove(t *testing.T) {
	nf, port, err := NewNetflow()
	if err != nil {
		t.Fatal(err)
	}
	defer nf.Stop()

	nf.registerWriter("stalled", make(chan *Packet)) // never read

	c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		c.Write(testPacket(1))
	}
	time.Sleep(100 * time.Millisecond) // let the reader pick them up

	done := make(chan error)
	go func() { done <- nf.RemoveWriter("stalled") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RemoveWriter blocked by a stalled writer")
	}
}
