// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// miniplumber is a package to facilitate communication pipelines between
// registered readers and writers across a distributed meshage environment.
// miniplumber supports plumbing to external programs over stdio, similar to
// unix pipelines, supports trees of communication pipelines, and configurable
// delivery options.
package miniplumber

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	TIMEOUT   = time.Duration(10 * time.Second)
	TOKEN_MAX = 1024 * 1024
)

const (
	SCHEDULE_ALL = -1
)

const (
	MODE_ALL = iota
	MODE_RR
	MODE_RND
)

const (
	MESSAGE_FORWARD = iota
	MESSAGE_QUERY
	MESSAGE_QUERY_RESPONSE
	MESSAGE_VIA_WRITE
	MESSAGE_VIA_HOST
)

// mesh is what the plumber needs from a meshage node to talk to the plumbers
// on the other hosts of a distributed environment.
type mesh interface {
	Name() string
	Set(recipients []string, body interface{}) ([]string, error)
	Broadcast(body interface{}) ([]string, error)
}

type Plumber struct {
	node      mesh                  // meshage node to use for distributed environments
	Messages  chan *meshage.Message // incoming messages from meshage
	pipes     map[string]*Pipe
	pipelines map[string]*pipeline
	lock      sync.Mutex
	tidLock   sync.Mutex
	tids      map[int64]*tid

	// messages from other hosts that wait to be applied to a pipe, by pipe.
	// A pipe has an entry while a goroutine (drain) applies its messages.
	inboxes   map[string][]Message
	inboxLock sync.Mutex
}

type tid struct {
	TID  int64
	C    chan *Message
	Done chan struct{}
	once sync.Once
}

type Pipe struct {
	name          string
	mode          int
	readers       map[int64]*Reader
	numWriters    int
	lock          sync.Mutex
	lastRecipient int64
	last          string
	log           bool
	viaCommand    []string
	viaHost       string
	numMessages   int64

	// set by PipeDelete, so that NewReader does not add to a deleted pipe
	deleted bool
}

type Reader struct {
	C    chan string
	Done chan struct{}
	once sync.Once
	ID   int64
}

type pipeline struct {
	name       string
	production []string
	done       chan bool
	canceler   sync.Once

	// wg tracks terminal I/O and processes that must finish before the
	// pipeline is removed
	wg sync.WaitGroup
}

type Message struct {
	TID     int64
	From    string
	Type    int
	Pipe    string
	Data    map[int64]string
	Readers []int64
	Value   string

	// Skip is the ID of a reader that must not get the value of a
	// MESSAGE_FORWARD or MESSAGE_VIA_WRITE: the reader of whoever wrote it.
	// 0 means that every reader may get it.
	Skip int64
}

type int64Sorter []int64

func (a int64Sorter) Len() int           { return len(a) }
func (a int64Sorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64Sorter) Less(i, j int) bool { return a[i] < a[j] }

func (t *tid) Close() {
	t.once.Do(func() {
		close(t.Done)
	})
}

func (r *Reader) Close() {
	r.once.Do(func() {
		close(r.Done)
	})
}

// New returns a new Plumber object over meshage node n
func New(n *meshage.Node) *Plumber {
	// a nil node must not become a mesh that is not nil
	if n == nil {
		return newPlumber(nil)
	}

	return newPlumber(n)
}

func newPlumber(n mesh) *Plumber {
	p := &Plumber{
		node:      n,
		Messages:  make(chan *meshage.Message, 1024),
		pipes:     make(map[string]*Pipe),
		pipelines: make(map[string]*pipeline),
		tids:      make(map[int64]*tid),
		inboxes:   make(map[string][]Message),
	}

	if p.node != nil {
		go p.handleMessages()
	}

	return p
}

// forward a value written to a pipe on this host to the plumbers on the other
// hosts, for recipient r as chosen by schedule. The reader with ID skip, if
// any, doesn't get it. No lock may be held.
func (p *Plumber) forward(pp *Pipe, data string, r, skip int64) error {
	if p.node == nil {
		return nil
	}

	m := &Message{
		From: p.node.Name(),
		Type: MESSAGE_FORWARD,
		Pipe: pp.name,
		Data: make(map[int64]string),
		Skip: skip,
	}

	pp.lock.Lock()
	via := len(pp.viaCommand) != 0
	_, local := pp.readers[r]
	pp.lock.Unlock()

	if !via {
		m.Data[r] = data
	} else {
		// forwarding with an enabled via is much more involved than just
		// forwarding the value unmodified. We instead create a value for
		// /every/ recipient on the other hosts, and ship the list of
		// recipient values up.
		var readers []int64

		if r == SCHEDULE_ALL {
			var err error

			readers, err = p.remoteReaders(pp.name)
			if err != nil {
				return err
			}
		} else if !local {
			readers = []int64{r}
		}

		pp.lock.Lock()
		for _, k := range readers {
			if k == skip {
				continue
			}

			d, err := pp.via(data)
			if err != nil {
				pp.lock.Unlock()
				return err
			}
			m.Data[k] = d
		}
		pp.lock.Unlock()

		// every recipient is on this host
		if len(m.Data) == 0 {
			return nil
		}
	}

	_, err := p.node.Broadcast(m)
	return err
}

// handleMessages receives the messages of the plumbers on the other hosts. It
// must not wait for anything that waits for a message in turn: a write to a
// pipe waits for the pipe's readers, and a reader can be a pipeline, whose
// write to its next pipe waits for the other hosts' answers (QUERY_RESPONSE)
// in round-robin and random modes. So the messages that touch a pipe go to
// the pipe's inbox, and queries are answered by goroutines of their own.
func (p *Plumber) handleMessages() {
	for {
		m := (<-p.Messages).Body.(Message)

		log.Debug("got message type %v from %v", m.Type, m.From)

		switch m.Type {
		case MESSAGE_FORWARD, MESSAGE_VIA_WRITE, MESSAGE_VIA_HOST:
			p.enqueue(m)
		case MESSAGE_QUERY:
			go p.sendReaders(&m)
		case MESSAGE_QUERY_RESPONSE:
			p.tidLock.Lock()
			t, ok := p.tids[m.TID]
			p.tidLock.Unlock()

			if !ok {
				// the query timed out; keep handling messages
				log.Errorln("dropping message for invalid TID: ", m.TID)
				continue
			}

			select {
			case t.C <- &m:
			case <-t.Done:
			}
		default:
			log.Error("unknown plumber message type: %v", m.Type)
		}
	}
}

// enqueue adds a message from another host to the inbox of its pipe, and
// starts a goroutine that applies the pipe's messages if none is running. It
// never waits for a pipe or for the plumber lock.
func (p *Plumber) enqueue(m Message) {
	p.inboxLock.Lock()
	defer p.inboxLock.Unlock()

	inbox, busy := p.inboxes[m.Pipe]
	p.inboxes[m.Pipe] = append(inbox, m)

	if !busy {
		go p.drain(m.Pipe)
	}
}

// drain applies the messages in a pipe's inbox, in the order of their
// arrival, and returns when the inbox is empty.
func (p *Plumber) drain(pipe string) {
	for {
		p.inboxLock.Lock()
		inbox := p.inboxes[pipe]
		if len(inbox) == 0 {
			delete(p.inboxes, pipe)
			p.inboxLock.Unlock()
			return
		}
		m := inbox[0]
		inbox[0] = Message{}
		p.inboxes[pipe] = inbox[1:]
		p.inboxLock.Unlock()

		switch m.Type {
		case MESSAGE_FORWARD:
			p.writeNoForward(m.Pipe, m.Data, m.Skip)
		case MESSAGE_VIA_WRITE:
			// wait for the write, to keep the order of the values
			p.lock.Lock()
			w, done := p.newWriter(m.Pipe, m.Skip)
			p.lock.Unlock()

			w <- m.Value
			close(w)
			<-done
		case MESSAGE_VIA_HOST:
			p.lock.Lock()

			if _, ok := p.pipes[m.Pipe]; !ok {
				p.pipes[m.Pipe] = &Pipe{
					name:    m.Pipe,
					readers: make(map[int64]*Reader),
				}
			}
			pp := p.pipes[m.Pipe]

			p.lock.Unlock()

			// a write to the pipe holds the pipe lock, so don't wait for
			// it with the plumber lock
			pp.lock.Lock()

			if m.Value != "" {
				pp.viaHost = m.Value
			} else {
				pp.viaHost = p.node.Name()
			}

			pp.lock.Unlock()
		}
	}
}

func (p *Plumber) newTID() *tid {
	p.tidLock.Lock()
	defer p.tidLock.Unlock()

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	t := &tid{
		TID:  r.Int63(),
		C:    make(chan *Message),
		Done: make(chan struct{}),
	}

	p.tids[t.TID] = t

	return t
}

func (p *Plumber) unregisterTID(t *tid) {
	t.Close()

	p.tidLock.Lock()
	defer p.tidLock.Unlock()

	delete(p.tids, t.TID)
}

// sendReaders answers another host's query for the readers of a pipe on this
// host. It holds no lock while it sends the answer.
func (p *Plumber) sendReaders(m *Message) {
	resp := &Message{
		TID:  m.TID,
		From: p.node.Name(),
		Type: MESSAGE_QUERY_RESPONSE,
		Pipe: m.Pipe,
	}

	p.lock.Lock()
	pp, ok := p.pipes[m.Pipe]
	p.lock.Unlock()

	if ok {
		pp.lock.Lock()
		for k, _ := range pp.readers {
			resp.Readers = append(resp.Readers, k)
		}
		pp.lock.Unlock()
	}

	_, err := p.node.Set([]string{m.From}, resp)
	if err != nil {
		log.Errorln(err)
	}
}

func (p *Plumber) Plumb(production ...string) error {
	for i, element := range production {
		if strings.HasPrefix(element, "file://") {
			if i == 0 {
				return fmt.Errorf("file terminal cannot be the first pipeline element: %v", element)
			}
			if i != len(production)-1 {
				return fmt.Errorf("file terminal must be the final pipeline element: %v", element)
			}
		}
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	// pipelines are unique by their string name, which may be an issue
	// down the road
	name := strings.Join(production, " ")

	log.Debug("got production: %v", name)

	if _, ok := p.pipelines[name]; ok {
		return fmt.Errorf("pipeline already exists: %v", production)
	}

	// done must exist before the pipeline can be found and deleted
	p.pipelines[name] = &pipeline{
		name:       name,
		production: production,
		done:       make(chan bool),
	}

	go p.startPipeline(p.pipelines[name])

	return nil
}

func (p *Plumber) Via(pipe string, command []string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			name:    pipe,
			readers: make(map[int64]*Reader),
		}
	}
	pp := p.pipes[pipe]

	pp.lock.Lock()
	defer pp.lock.Unlock()

	pp.viaCommand = command

	// a plumber without a mesh has no other hosts to tell
	if p.node == nil {
		return
	}

	m := &Message{
		From:  p.node.Name(),
		Type:  MESSAGE_VIA_HOST,
		Pipe:  pipe,
		Value: p.node.Name(),
	}

	pp.viaHost = p.node.Name()

	if len(command) == 0 {
		m.Value = ""
	}

	_, err := p.node.Broadcast(m)
	if err != nil {
		log.Errorln(err)
	}
}

func (p *Plumber) Mode(pipe string, mode int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			name:    pipe,
			readers: make(map[int64]*Reader),
		}
	}
	pp := p.pipes[pipe]

	pp.lock.Lock()
	defer pp.lock.Unlock()

	pp.mode = mode
}

func (p *Plumber) Log(pipe string, mode bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			name:    pipe,
			readers: make(map[int64]*Reader),
		}
	}
	pp := p.pipes[pipe]

	pp.lock.Lock()
	defer pp.lock.Unlock()

	pp.Log(mode)
}

func (p *Plumber) PipelineDelete(production ...string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	name := strings.Join(production, " ")
	if pp, ok := p.pipelines[name]; !ok {
		return fmt.Errorf("no such pipeline: %v", production)
	} else {
		pp.cancel()
		return nil
	}
}

func (p *Plumber) PipelineDeleteAll() error {
	p.lock.Lock()

	var keys []string
	for k, _ := range p.pipelines {
		keys = append(keys, k)
	}

	p.lock.Unlock()

	for _, k := range keys {
		err := p.PipelineDelete(k)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plumber) PipeDelete(pipe string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if pp, ok := p.pipes[pipe]; ok {
		pp.lock.Lock()
		defer pp.lock.Unlock()

		if pp.numWriters != 0 {
			return fmt.Errorf("cannot delete named pipe with open writers")
		}

		// kill all of the readers
		for _, c := range pp.readers {
			c.Close()
		}
		pp.deleted = true
		delete(p.pipes, pipe)

		return nil
	} else {
		return fmt.Errorf("no such pipe: %v", pipe)
	}
}

func (p *Plumber) PipeDeleteAll() error {
	p.lock.Lock()

	var keys []string
	for k, _ := range p.pipes {
		keys = append(keys, k)
	}

	p.lock.Unlock()

	for _, k := range keys {
		err := p.PipeDelete(k)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plumber) Pipes() []*Pipe {
	p.lock.Lock()
	defer p.lock.Unlock()

	var keys []string
	var ret []*Pipe

	for k, _ := range p.pipes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, v := range keys {
		ret = append(ret, p.pipes[v])
	}

	return ret
}

// Pipelines returns a sorted list of pipeline production strings
func (p *Plumber) Pipelines() []string {
	p.lock.Lock()
	defer p.lock.Unlock()

	var ret []string

	for k, _ := range p.pipelines {
		ret = append(ret, k)
	}

	sort.Strings(ret)

	return ret
}

func (p *Plumber) id() int64 {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	return r.Int63()
}

func (p *Plumber) NewReader(pipe string) *Reader {
	log.Debug("newReader: %v", pipe)

	r := &Reader{
		C:    make(chan string),
		Done: make(chan struct{}),
		ID:   p.id(),
	}

	// A write holds the pipe lock while it waits for a reader, so do not wait
	// for the pipe lock with the plumber lock held.
	var pp *Pipe
	for {
		p.lock.Lock()
		if _, ok := p.pipes[pipe]; !ok {
			p.pipes[pipe] = &Pipe{
				name:    pipe,
				readers: make(map[int64]*Reader),
			}
		}
		pp = p.pipes[pipe]
		p.lock.Unlock()

		pp.lock.Lock()
		if !pp.deleted {
			break
		}
		pp.lock.Unlock()
	}
	pp.readers[r.ID] = r
	pp.lock.Unlock()

	go func() {
		<-r.Done
		pp.lock.Lock()
		defer pp.lock.Unlock()
		close(r.C)
		delete(pp.readers, r.ID)
	}()

	return r
}

func (p *Plumber) NewWriter(pipe string) chan<- string {
	w, _ := p.NewWriterDone(pipe)
	return w
}

// NewWriterDone is like NewWriter, and also returns a channel that is closed
// once the writer has been closed and every message sent on it has been
// written to the pipe.
func (p *Plumber) NewWriterDone(pipe string) (chan<- string, <-chan struct{}) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.newWriter(pipe, 0)
}

// NewWriterExcept is like NewWriterDone, for a writer that also reads the pipe
// with r: nothing sent on the writer is delivered to r. In round-robin and
// random modes, one of the other readers gets it.
func (p *Plumber) NewWriterExcept(pipe string, r *Reader) (chan<- string, <-chan struct{}) {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.newWriter(pipe, r.ID)
}

// assume the lock is held. The reader with ID skip, if any, gets nothing of
// what is sent on the writer.
func (p *Plumber) newWriter(pipe string, skip int64) (chan<- string, <-chan struct{}) {
	log.Debug("newWriter: %v", pipe)

	c := make(chan string)
	done := make(chan struct{})

	if _, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			name:    pipe,
			readers: make(map[int64]*Reader),
		}
	}
	pp := p.pipes[pipe]
	pp.lock.Lock()
	pp.numWriters++
	pp.lock.Unlock()

	go func() {
		defer close(done)

		for v := range c {
			// don't do local writes if the via doesn't belong to
			// us - post it to the owner instead.
			if p.node != nil {
				pp.lock.Lock()
				viaHost := pp.viaHost
				pp.lock.Unlock()

				if viaHost != p.node.Name() && viaHost != "" {
					m := &Message{
						From:  p.node.Name(),
						Type:  MESSAGE_VIA_WRITE,
						Pipe:  pipe,
						Skip:  skip,
						Value: v,
					}

					_, err := p.node.Set([]string{viaHost}, m)
					if err != nil {
						log.Errorln(err)
					}

					continue
				}
			}

			r, err := p.schedule(pp, skip)
			if err != nil {
				log.Errorln(err)
				continue
			}

			err = p.forward(pp, v, r, skip)
			if err != nil {
				log.Errorln(err)
				continue
			}

			pp.write(v, r, skip)
		}
		pp.lock.Lock()
		pp.numWriters--
		pp.lock.Unlock()
	}()

	return c, done
}

func (p *Plumber) Write(pipe string, value string) {
	p.write(pipe, value, 0)
}

// write a value to a named pipe. The reader with ID skip, if any, doesn't
// get it.
func (p *Plumber) write(pipe string, value string, skip int64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	w, _ := p.newWriter(pipe, skip)
	w <- value
	close(w)
}

// Based on the pipe mode, choose a recipient - system wide. This means
// querying other plumber's state over meshage. The reader with ID skip, if
// any, is never chosen. No lock may be held.
func (p *Plumber) schedule(pp *Pipe, skip int64) (int64, error) {
	pp.lock.Lock()
	mode := pp.mode
	pp.lock.Unlock()

	if mode == MODE_ALL {
		return SCHEDULE_ALL, nil
	}

	remote, err := p.remoteReaders(pp.name)
	if err != nil {
		return 0, err
	}

	pp.lock.Lock()
	defer pp.lock.Unlock()

	var readers []int64

	for _, k := range remote {
		if k != skip {
			readers = append(readers, k)
		}
	}
	for k, _ := range pp.readers {
		if k != skip {
			readers = append(readers, k)
		}
	}

	sort.Sort(int64Sorter(readers))

	if len(readers) == 0 {
		return SCHEDULE_ALL, nil
	}

	// pick a winner!
	switch mode {
	case MODE_RR:
		i := sort.Search(len(readers), func(i int) bool { return readers[i] > pp.lastRecipient })
		if i == len(readers) {
			i = 0
		}
		pp.lastRecipient = readers[i]
		return readers[i], nil
	case MODE_RND:
		return readers[rand.Intn(len(readers))], nil
	}

	// we should never get here
	return 0, nil
}

// remoteReaders asks the plumbers on the other hosts for their readers of a
// pipe.
//
// No lock may be held: the other plumbers need their locks to answer, also
// when they are asking us at the same time, and we need ours to answer them.
//
// TODO: maybe don't ask on /every/ call, but instead set some kind of max
// rate
func (p *Plumber) remoteReaders(pipe string) ([]int64, error) {
	// a plumber without a mesh has no remote readers
	if p.node == nil {
		return nil, nil
	}

	t := p.newTID()
	defer p.unregisterTID(t)

	m := &Message{
		TID:  t.TID,
		From: p.node.Name(),
		Type: MESSAGE_QUERY,
		Pipe: pipe,
	}

	nodes, err := p.node.Broadcast(m)
	if err != nil {
		return nil, err
	}

	var readers []int64

	// wait for n responses, or a timeout
	for i := 0; i < len(nodes); i++ {
		select {
		case resp := <-t.C:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			readers = append(readers, resp.Readers...)
		case <-time.After(TIMEOUT):
			return nil, fmt.Errorf("timeout")
		}
	}

	return readers, nil
}

// write to a named pipe without forwarding the message over meshage or
// scheduling its delivery. The plumber lock is not held during the write,
// which waits for the pipe's readers.
func (p *Plumber) writeNoForward(pipe string, data map[int64]string, skip int64) {
	p.lock.Lock()
	pp, ok := p.pipes[pipe]
	p.lock.Unlock()

	if ok {
		for r, v := range data {
			pp.write(v, r, skip)
		}
	}
}

// started in a goroutine, don't assume the lock is held
func (p *Plumber) startPipeline(pl *pipeline) {
	var b <-chan string
	for i, e := range pl.production {
		log.Debug("starting pipeline production element: %v", e)

		// looks like a file terminal
		if strings.HasPrefix(e, "file://") {
			path := strings.TrimPrefix(e, "file://")
			b = pl.fileTerminal(path, b)
			continue
		}

		// start a process if it looks like a process, otherwise create
		// a pipe
		f := fieldsQuoteEscape("\"", e)
		process, err := exec.LookPath(f[0])
		if err == nil {
			f[0] = process

			// don't write data on stdout/err if this is the last stage
			var write bool
			if i != len(pl.production)-1 {
				write = true
			}

			b, err = pl.exec(f, b, write)
			if err != nil {
				pl.cancel()
				log.Errorln(err)
				break
			}
			continue
		}

		// looks like a named pipe
		var in *Reader

		// don't produce output if this is the final stage
		if i != len(pl.production)-1 {
			in = p.NewReader(e)
		}

		var out chan<- string

		// don't produce input if this is the first stage
		if i != 0 {
			out = p.NewWriter(e)
		}
		b = pl.pipe(in, out, b)
	}

	// start after the loop so that every pl.wg.Add happens before pl.wg.Wait
	go func() {
		<-pl.done
		pl.wg.Wait()

		p.lock.Lock()
		delete(p.pipelines, pl.name)
		p.lock.Unlock()
	}()
}

func (pl *pipeline) exec(production []string, in <-chan string, write bool) (<-chan string, error) {
	log.Debug("exec: %v, %v", production, write)

	var out chan string

	cmd := &exec.Cmd{
		Path: production[0],
		Args: production,
	}

	if in != nil {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}

		go func() {
			defer pl.cancel()

			for {
				select {
				case v, ok := <-in:
					if !ok {
						// the upstream reader was closed
						return
					}
					_, err := stdin.Write([]byte(v))
					if err != nil {
						log.Errorln(err)
						return
					}
				case <-pl.done:
					return
				}
			}
		}()
	}

	if write {
		out = make(chan string)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}

		go func() {
			defer pl.cancel()

			scanner := bufio.NewScanner(stdout)
			buf := make([]byte, 0, TOKEN_MAX)
			scanner.Buffer(buf, TOKEN_MAX)
			for scanner.Scan() {
				select {
				case out <- scanner.Text() + "\n":
				case <-pl.done:
					return
				}
				log.Debug("exec got: %v", scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				select {
				case <-pl.done:
					// stdout was closed under us when the
					// pipeline was torn down
				default:
					log.Errorln(err)
				}
				return
			}
		}()
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		r := bufio.NewReader(stderr)
		for {
			d, err := r.ReadString('\n')
			if d := strings.TrimSpace(d); d != "" {
				log.Debug("plumber: %v: %v", cmd.Path, d)
			}
			if err != nil {
				break
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	// command is running

	pl.wg.Add(1)
	go func() {
		defer pl.wg.Done()

		<-pl.done
		cmd.Process.Kill()
		// reap the process so it doesn't stay around as a zombie
		cmd.Wait()
	}()

	return out, nil
}

func (pl *pipeline) pipe(pin *Reader, pout chan<- string, in <-chan string) <-chan string {
	log.Debug("pipe")

	if in != nil {
		go func() {
			defer close(pout)
			defer pl.cancel()

			for {
				select {
				case v, ok := <-in:
					if !ok {
						// the upstream reader was closed; don't
						// write its zero value downstream
						return
					}
					select {
					case pout <- v:
					case <-pl.done:
						return
					}
					log.Debug("pipe got: %v", v)
				case <-pl.done:
					return
				}
			}
		}()
	}

	if pin != nil {
		go func() {
			defer pin.Close()
			defer pl.cancel()
			select {
			case <-pl.done:
			case <-pin.Done:
			}
		}()
		return pin.C
	}

	return nil
}

func (pl *pipeline) cancel() {
	pl.canceler.Do(func() {
		log.Debug("closing pipeline: %v", pl.name)
		close(pl.done)
	})
}

// fileTerminal writes all incoming data to the file at path. If the file
// exists it is appended to; otherwise it is created. The returned channel is
// always nil because a file terminal consumes data and produces no output.
func (pl *pipeline) fileTerminal(path string, in <-chan string) <-chan string {
	log.Debug("fileTerminal: %v", path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		pl.cancel()
		log.Errorln(err)
		return nil
	}

	pl.wg.Add(1)
	go func() {
		defer pl.wg.Done()
		defer pl.cancel()
		defer f.Close()

		for {
			select {
			case v, ok := <-in:
				if !ok {
					return
				}
				if _, err := f.WriteString(v); err != nil {
					log.Errorln(err)
					return
				}
			case <-pl.done:
				return
			}
		}
	}()

	return nil
}

func (p *Pipe) Name() string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.name
}

func (p *Pipe) Mode() string {
	p.lock.Lock()
	defer p.lock.Unlock()

	switch p.mode {
	case MODE_ALL:
		return "all"
	case MODE_RR:
		return "round-robin"
	case MODE_RND:
		return "random"
	default:
		log.Fatal("unknown mode: %v", p.mode)
	}
	return ""
}

func (p *Pipe) NumReaders() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return len(p.readers)
}

func (p *Pipe) NumWriters() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.numWriters
}

func (p *Pipe) Last() string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.last
}

func (p *Pipe) Log(mode bool) {
	p.log = mode
}

func (p *Pipe) GetVia() string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return strings.Join(p.viaCommand, " ")
}

func (p *Pipe) NumMessages() int64 {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.numMessages
}

// pipe lock is already held
func (p *Pipe) via(value string) (string, error) {
	if len(p.viaCommand) == 0 {
		return value, nil
	}

	process, err := exec.LookPath(p.viaCommand[0])
	if err != nil {
		return "", err
	}

	stdin := bytes.NewBufferString(value)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := &exec.Cmd{
		Path:   process,
		Args:   p.viaCommand,
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %v", err, stderr.String())
	}

	return stdout.String(), nil
}

// don't assume the plumber lock is held
// write a value to the pipe's readers on this host: to all of them if r is
// SCHEDULE_ALL, else to the reader with ID r. The reader with ID skip, if
// any, doesn't get it.
func (p *Pipe) write(value string, r, skip int64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	// messages must end in a newline, because things like scanners depend
	// on them. Add a newline if it doesn't already exist.
	if !strings.HasSuffix(value, "\n") {
		value += "\n"
	}

	if r == SCHEDULE_ALL {
		p.last = value
		if p.log {
			log.Debug("%s", fmt.Sprintf("pipe %v: %v", p.name, strings.TrimSpace(value)))
		}
		for _, c := range p.readers {
			if c.ID == skip {
				continue
			}

			d, err := p.via(value)
			if err != nil {
				log.Errorln(err)
				return
			}
			select {
			case <-c.Done:
				continue
			case c.C <- d:
			}
		}
	} else {
		if c, ok := p.readers[r]; ok && r != skip {
			if p.log {
				log.Debug("%s", fmt.Sprintf("pipe %v: %v", p.name, strings.TrimSpace(value)))
			}
			d, err := p.via(value)
			if err != nil {
				log.Errorln(err)
				return
			}
			p.last = value
			select {
			case <-c.Done:
			case c.C <- d:
			}
		}
	}

	// tally the messages
	p.numMessages += 1
}

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
//
//	Example: a b "c d"
//	will return: ["a", "b", "c d"]
func fieldsQuoteEscape(c string, input string) []string {
	log.Debug("fieldsQuoteEscape splitting on %v: %v", c, input)
	f := strings.Fields(input)
	var ret []string
	trace := false
	temp := ""

	for _, v := range f {
		if trace {
			if strings.Contains(v, c) {
				trace = false
				temp += " " + trimQuote(c, v)
				ret = append(ret, temp)
			} else {
				temp += " " + v
			}
		} else if strings.Contains(v, c) {
			temp = trimQuote(c, v)
			if strings.HasSuffix(v, c) {
				// special case, single word like 'foo'
				ret = append(ret, temp)
			} else {
				trace = true
			}
		} else {
			ret = append(ret, v)
		}
	}
	log.Debug("generated: %#v", ret)
	return ret
}

func trimQuote(c string, input string) string {
	if c == "" {
		log.Errorln("cannot trim empty space")
		return ""
	}
	var ret string
	for _, v := range input {
		if v != rune(c[0]) {
			ret += string(v)
		}
	}
	return ret
}
