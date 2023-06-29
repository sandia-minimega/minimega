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
