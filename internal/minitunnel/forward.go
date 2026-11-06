package minitunnel

import (
	"fmt"
	"net"
)

type forward struct {
	fid  int
	src  int
	host string
	dst  int

	listener    net.Listener
	connections []net.Conn
}

func (f *forward) addConnection(c net.Conn) {
	f.connections = append(f.connections, c)
}

func (f *forward) close() {
	f.listener.Close()

	for _, conn := range f.connections {
		conn.Close()
	}
}

func (f *forward) String() string {
	return fmt.Sprintf("%d:%s:%d", f.src, f.host, f.dst)
}

func (t *Tunnel) newForward(l net.Listener, src int, host string, dst int) *forward {
	return &forward{
		fid:  <-t.forwardIDs,
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
	f, ok := t.forwards[id]
	if !ok {
		return fmt.Errorf("forwarder with ID %d not found", id)
	}

	f.close()

	t.sendLock.Lock()
	defer t.sendLock.Unlock()

	delete(t.forwards, f.fid)
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
