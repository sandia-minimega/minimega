package minitunnel

import (
	"fmt"
	"net"
	"sync"
)

type forward struct {
	fid  int
	src  int
	host string
	dst  int

	listener net.Listener

	mu          sync.Mutex // guards closed and connections
	closed      bool
	connections []net.Conn
}

// addConnection adds c to the connections that close closes. It returns false
// if the forward is closed, and then the caller must close c.
func (f *forward) addConnection(c net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return false
	}

	f.connections = append(f.connections, c)

	return true
}

func (f *forward) close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true
	f.listener.Close()

	for _, conn := range f.connections {
		conn.Close()
	}
	f.connections = nil
}

func (f *forward) String() string {
	return fmt.Sprintf("%d:%s:%d", f.src, f.host, f.dst)
}

func (t *Tunnel) newForward(l net.Listener, src int, host string, dst int) *forward {
	return &forward{
		fid:  int(t.lastFID.Add(1)),
		src:  src,
		host: host,
		dst:  dst,

		listener: l,
	}
}

func (t *Tunnel) ListForwards() map[int]string {
	list := make(map[int]string)

	t.sendLock.Lock()
	defer t.sendLock.Unlock()

	for i, f := range t.forwards {
		list[i] = f.String()
	}

	return list
}

func (t *Tunnel) CloseForward(id int) error {
	t.sendLock.Lock()
	f, ok := t.forwards[id]
	delete(t.forwards, id)
	t.sendLock.Unlock()

	if !ok {
		return fmt.Errorf("forwarder with ID %d not found", id)
	}

	f.close()

	return nil
}

// ReverseTunnel is a reverse tunnel that the remote end opened: it listens on
// Source and tunnels each connection back to the local end, which connects to
// Host and Dest.
type ReverseTunnel struct {
	Source int
	Host   string
	Dest   int
}

// addReverse records r unless the tunnel has already quit.
func (t *Tunnel) addReverse(r ReverseTunnel) {
	t.reverseLock.Lock()
	defer t.reverseLock.Unlock()

	select {
	case <-t.quit:
	default:
		t.reverses = append(t.reverses, r)
	}
}

// clearReverses removes the records of all reverse tunnels. The mux calls it
// when the tunnel quits, because no reverse tunnel can work after that.
func (t *Tunnel) clearReverses() {
	t.reverseLock.Lock()
	defer t.reverseLock.Unlock()

	t.reverses = nil
}

// ListReverses returns the reverse tunnels that the remote end acknowledged, in
// the order of the acknowledgments. It returns none after the tunnel quits.
func (t *Tunnel) ListReverses() []ReverseTunnel {
	t.reverseLock.Lock()
	defer t.reverseLock.Unlock()

	return append([]ReverseTunnel(nil), t.reverses...)
}
