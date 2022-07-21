package minitunnel

import (
	"fmt"
	"net"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type chans struct {
	sync.Mutex // embed

	// maps of transaction id/incoming channel pairs for routing multiple tunnels
	chans map[int]chan *tunnelMessage
}

func makeChans() chans {
	return chans{
		chans: make(map[int]chan *tunnelMessage),
	}
}

func (c *chans) add(ID int) chan *tunnelMessage {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.chans[ID]; ok {
		log.Fatal("ID %v already exists!", ID)
	}

	ch := make(chan *tunnelMessage, 1024)
	c.chans[ID] = ch

	return ch
}

func (c *chans) remove(ID int) {
	c.Lock()
	defer c.Unlock()

	if ch, ok := c.chans[ID]; ok {
		close(ch)
	}

	delete(c.chans, ID)
}

func (c *chans) get(ID int) chan *tunnelMessage {
	c.Lock()
	defer c.Unlock()

	return c.chans[ID]
}

func (c *chans) dropAll() []chan *tunnelMessage {
	c.Lock()
	defer c.Unlock()

	res := []chan *tunnelMessage{}

	for k, ch := range c.chans {
		res = append(res, ch)
		delete(c.chans, k)
	}

	return res
}

// mux to handle i/o over the transport. Data on channel out will be sent over
// the transport. Data coming in over the transport will be routed to the
// incoming channel as tagged be the message's TID. This allows us to trunk
// multiple tunnels over a single transport.
func (t *Tunnel) mux() {
	var err error

	log.Info("starting minitunnel mux")

	for {
		var m tunnelMessage
		if err = t.dec.Decode(&m); err != nil {
			break
		}

		log.Debug("new message: %v", m.Type)

		// create new session if necessary
		if m.Type == CONNECT {
			t.handleRemote(&m)
		} else if m.Type == FORWARD {
			t.handleReverse(&m)
		} else if c := t.chans.get(m.TID); c != nil {
			// route the message to the handler by TID
			c <- &m
		} else {
			log.Info("invalid TID: %v", m.TID)
		}
	}

	close(t.quit) // signal to all listeners that this tunnel is outa here
	t.transport.Close()

	for _, ch := range t.chans.dropAll() {
		close(ch)
	}

	log.Info("mux exit: %v", err)
}

func (t *Tunnel) handleRemote(m *tunnelMessage) {
	host := m.Host
	port := m.Port
	TID := m.TID

	// attempt to connect to the host/port
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", host, port))
	if err == nil {
		in := t.chans.add(TID)
		go t.transfer(in, conn, TID)
		return
	}

	log.Errorln(err)

	resp := &tunnelMessage{
		Type:  CLOSED,
		TID:   TID,
		Error: err.Error(),
	}

	if err := t.sendMessage(resp); err != nil {
		log.Errorln(err)
	}
}

// reverse tunnels are made by simply asking the remote end to invoke 'Forward'
func (t *Tunnel) handleReverse(m *tunnelMessage) {
	resp := &tunnelMessage{
		Type: DATA,
		TID:  m.TID,
		Ack:  true,
	}
	if err := t.Forward(m.Source, m.Host, m.Port); err != nil {
		resp.Error = err.Error()
	}

	if err := t.sendMessage(resp); err != nil {
		log.Errorln(err)
	}
}

func (t *Tunnel) sendMessage(m *tunnelMessage) error {
	t.sendLock.Lock()
	defer t.sendLock.Unlock()

	return t.enc.Encode(m)
}
