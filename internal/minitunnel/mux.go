package minitunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var errTooManyStreams = errors.New("too many open tunnel streams")

type chans struct {
	sync.Mutex // embed

	// maps of transaction id/incoming channel pairs for routing multiple tunnels
	chans map[int]chan *tunnelMessage

	// max is the maximum number of entries in chans, zero for no limit
	max int
}

func makeChans() chans {
	return chans{
		chans: make(map[int]chan *tunnelMessage),
	}
}

func (c *chans) add(ID int) (chan *tunnelMessage, error) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.chans[ID]; ok {
		return nil, fmt.Errorf("tunnel ID %v already exists", ID)
	}

	if c.max > 0 && len(c.chans) >= c.max {
		return nil, errTooManyStreams
	}

	ch := make(chan *tunnelMessage, 1024)
	c.chans[ID] = ch

	return ch, nil
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

	log.Debug("starting minitunnel mux")

	for {
		m := new(tunnelMessage)
		if err = t.decode(m); err != nil {
			break
		}

		if err = t.dispatch(m); err != nil {
			break
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

// dispatch handles one message from the remote end. A panic ends the tunnel
// instead of the process.
func (t *Tunnel) dispatch(m *tunnelMessage) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("minitunnel message handler panic: %v", r)
		}
	}()

	log.Debug("new message: %v", m.Type)

	// create new session if necessary
	if m.Type == CONNECT {
		t.handleRemote(m)
	} else if m.Type == FORWARD {
		t.handleReverse(m)
	} else if c := t.chans.get(m.TID); c != nil {
		// route the message to the handler by TID
		c <- m
	} else {
		log.Info("invalid TID: %v", m.TID)
	}

	return nil
}

func (t *Tunnel) handleRemote(m *tunnelMessage) {
	host := m.Host
	port := m.Port
	TID := m.TID

	var err error

	if t.policy.AllowConnect != nil && !t.policy.AllowConnect(host, port) {
		err = fmt.Errorf("connect to %v:%v refused by policy", host, port)
		t.refused(err.Error())
	} else {
		var in chan *tunnelMessage
		if in, err = t.chans.add(TID); err == nil {
			// attempt to connect to the host/port
			var conn net.Conn
			conn, err = net.Dial("tcp", fmt.Sprintf("%v:%v", host, port))
			if err == nil {
				go t.transfer(in, conn, TID)
				return
			}

			t.chans.remove(TID)
			log.Errorln(err)
		} else {
			t.refused(fmt.Sprintf("connect to %v:%v: %v", host, port, err))
		}
	}

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

	if t.policy.DenyForward {
		resp.Error = fmt.Sprintf("forward of port %v to %v:%v refused by policy", m.Source, m.Host, m.Port)
		t.refused(resp.Error)
	} else if err := t.Forward(m.Source, m.Host, m.Port); err != nil {
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
