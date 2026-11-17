package minitunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type chans struct {
	sync.Mutex // embed

	// maps of transaction id/incoming channel pairs for routing multiple tunnels
	chans map[int]*registration
}

// registration is one TID's incoming channel. done is closed by remove so a
// mux send blocked on a full, abandoned channel gives up.
type registration struct {
	c    chan *tunnelMessage
	done chan struct{}
}

func makeChans() chans {
	return chans{
		chans: make(map[int]*registration),
	}
}

func (c *chans) add(ID int) chan *tunnelMessage {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.chans[ID]; ok {
		log.Fatal("ID %v already exists!", ID)
	}

	r := &registration{
		c:    make(chan *tunnelMessage, 1024),
		done: make(chan struct{}),
	}
	c.chans[ID] = r

	return r.c
}

func (c *chans) remove(ID int) {
	c.Lock()
	defer c.Unlock()

	// don't close the channel: the mux may still hold it and be about to send.
	// Only the mux closes channels, in dropAll. Closing done releases a send
	// that is blocked on it.
	if r, ok := c.chans[ID]; ok {
		close(r.done)
		delete(c.chans, ID)
	}
}

// send routes m to ID's channel, giving up if ID is removed while the
// channel is full. Returns false if ID is not registered.
func (c *chans) send(ID int, m *tunnelMessage) bool {
	c.Lock()
	r := c.chans[ID]
	c.Unlock()

	if r == nil {
		return false
	}

	select {
	case r.c <- m:
	case <-r.done:
	}

	return true
}

func (c *chans) dropAll() []chan *tunnelMessage {
	c.Lock()
	defer c.Unlock()

	res := []chan *tunnelMessage{}

	for k, r := range c.chans {
		res = append(res, r.c)
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

	log.Debug("starting minitunnel mux")

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
		} else if !t.chans.send(m.TID, &m) {
			// route the message to the handler by TID
			log.Info("invalid TID: %v", m.TID)
		}
	}

	close(t.quit) // signal to all listeners that this tunnel is outa here
	t.transport.Close()
	t.clearReverses()

	for _, ch := range t.chans.dropAll() {
		close(ch)
	}

	// a clean EOF is the normal end of every cc client connection, so only
	// unexpected decode errors are logged above debug level
	if errors.Is(err, io.EOF) {
		log.Debug("mux exit: %v", err)
	} else {
		log.Info("mux exit: %v", err)
	}
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
