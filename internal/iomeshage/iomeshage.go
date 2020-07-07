// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package iomeshage

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	MAX_ATTEMPTS = 3
	QUEUE_LEN    = 3
)

// IOMeshage object, which must have a base path to serve files on and a
// meshage node.
type IOMeshage struct {
	base      string                // base path for serving files
	node      *meshage.Node         // meshage node to use
	Messages  chan *meshage.Message // Incoming messages from meshage
	drainLock sync.RWMutex
	queue     chan bool
	rand      *rand.Rand

	// transferLock guards transfers
	transferLock sync.RWMutex
	transfers    map[string]*Transfer // current transfers

	// tidLock guards TIDs
	tidLock sync.Mutex
	TIDs    map[int64]chan *Message // transfer ID -> channel
}

// Transfer describes an in-flight transfer.
type Transfer struct {
	Dir      string         // temporary directory hold the file parts
	Filename string         // file name
	Parts    map[int64]bool // completed parts
	NumParts int            // total number of parts for this file
	Inflight int64          // currently in-flight part, -1 if none
	Queued   bool
}

var (
	timeout = time.Duration(30 * time.Second)
)

// New returns a new iomeshage object service base directory via meshage
func New(base string, node *meshage.Node) (*IOMeshage, error) {
	base = filepath.Clean(base)
	log.Debug("new iomeshage node on base %v", base)
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}

	r := &IOMeshage{
		base:      base,
		node:      node,
		Messages:  make(chan *meshage.Message, 1024),
		TIDs:      make(map[int64]chan *Message),
		transfers: make(map[string]*Transfer),
		queue:     make(chan bool, QUEUE_LEN),
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	go r.handleMessages()

	return r, nil
}

func (iom *IOMeshage) info(file string) ([]*Message, error) {
	TID, c := iom.newTID()
	defer iom.unregisterTID(TID)

	m := &Message{
		From:     iom.node.Name(),
		Type:     TYPE_INFO,
		Filename: file,
		TID:      TID,
	}
	recipients, err := iom.node.Broadcast(m)
	if err != nil {
		return nil, err
	}
	if log.WillLog(log.DEBUG) {
		log.Debug("sent info request to %v nodes", len(recipients))
	}

	var info []*Message

	// wait for n responses, or a timeout
	for i := 0; i < len(recipients); i++ {
		select {
		case resp := <-c:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			if resp.ACK {
				if log.WillLog(log.DEBUG) {
					log.Debugln("got info from: ", resp.From)
				}
				info = append(info, resp)
			}
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout")
		}
	}

	return info, nil
}

// search the mesh for the file/glob/directory, returning a slice of string
// matches. The search includes local matches.
func (iom *IOMeshage) Info(file string) []string {
	var ret []string

	// search locally
	files, _ := iom.List(file, true)
	for _, file := range files {
		ret = append(ret, iom.Rel(file))
	}

	// search the mesh
	TID, c := iom.newTID()
	defer iom.unregisterTID(TID)

	m := &Message{
		From:     iom.node.Name(),
		Type:     TYPE_INFO,
		Filename: file,
		TID:      TID,
	}
	recipients, err := iom.node.Broadcast(m)
	if err != nil {
		log.Errorln(err)
		return nil
	}
	if log.WillLog(log.DEBUG) {
		log.Debug("sent info request to %v nodes", len(recipients))
	}

	// wait for n responses, or a timeout
	for i := 0; i < len(recipients); i++ {
		select {
		case resp := <-c:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			if resp.ACK {
				if log.WillLog(log.DEBUG) {
					log.Debugln("got info from: ", resp.From)
				}
				if len(resp.Glob) == 0 {
					// exact match unless the exact match is the original glob
					if !strings.Contains(resp.Filename, "*") {
						ret = append(ret, resp.Filename)
					}
				} else {
					ret = append(ret, resp.Glob...)
				}
			}
		case <-time.After(timeout):
			log.Errorln(fmt.Errorf("timeout"))
			return nil
		}
	}

	return ret
}

// Retrieve a file from the shortest path node that has it. Get blocks until
// the file transfer begins or errors out. If the file specified is a
// directory, the entire directory will be recursively transferred. If the file
// already exists on this node, Get will return immediately with no error.
func (iom *IOMeshage) Get(file string) error {
	// is this a directory or a glob
	fi, err := os.Stat(filepath.Join(iom.base, file))
	if err == nil && !fi.IsDir() {
		return nil
	}

	// is this file already in flight?
	iom.transferLock.RLock()
	if _, ok := iom.transfers[file]; ok {
		iom.transferLock.RUnlock()
		return fmt.Errorf("file already in flight")
	}
	iom.transferLock.RUnlock()

	info, err := iom.info(file)
	if err != nil {
		return err
	}
	if len(info) == 0 {
		return fmt.Errorf("get %v: file not found", file)
	}

	inflight := make(map[string]bool)

	for _, v := range info {
		// is this a single file or a directory/blob?
		if len(v.Glob) == 0 {
			if _, ok := inflight[v.Filename]; ok {
				continue
			}

			if log.WillLog(log.INFO) {
				log.Info("found file on node %v with %v parts", v.From, v.Part)
			}

			// create a transfer object
			tdir, err := ioutil.TempDir(iom.base, "transfer_")
			if err != nil {
				log.Errorln(err)
				return err
			}
			iom.transferLock.Lock()
			iom.transfers[v.Filename] = &Transfer{
				Dir:      tdir,
				Filename: v.Filename,
				Parts:    make(map[int64]bool),
				NumParts: int(v.Part),
				Inflight: -1,
				Queued:   true,
			}
			iom.transferLock.Unlock()

			go iom.getParts(v.Filename, v.Part, v.Perm)

			inflight[v.Filename] = true
		} else {
			// call Get on each of the constituent files, queued in a random order

			// fisher-yates shuffle
			for i := int64(len(v.Glob)) - 1; i > 0; i-- {
				j := iom.rand.Int63n(i + 1)
				t := v.Glob[j]
				v.Glob[j] = v.Glob[i]
				v.Glob[i] = t
			}

			for _, x := range v.Glob {
				if _, ok := inflight[x]; ok {
					continue
				}
				if err := iom.Get(x); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Stream requests each part of the file in order, returning a channel to read
// the parts from. This does not store the file locally to avoid filling up the
// local disk.
func (iom *IOMeshage) Stream(file string) (chan []byte, error) {
	// is this a directory or a glob
	fi, err := os.Stat(filepath.Join(iom.base, file))
	if err == nil && !fi.IsDir() {
		return stream(filepath.Join(iom.base, file))
	}

	info, err := iom.info(file)
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, fmt.Errorf("stream %v: file not found", file)
	}

	// request file from the first responder
	first := info[0]
	if len(first.Glob) > 0 {
		return nil, errors.New("cannot stream a glob")
	}

	out := make(chan []byte)

	go func() {
		defer close(out)

		if log.WillLog(log.DEBUG) {
			log.Debug("found file on node %v with %v parts", first.From, first.Part)
		}

		// get in line
		iom.queue <- true
		defer func() {
			<-iom.queue
		}()

		for i := int64(0); i < first.Part; i++ {
			data, err := iom.xfer(first.Filename, i, first.From)
			if err != nil {
				log.Error("stream failed: %v", err)
				return
			}

			out <- data
		}
	}()

	return out, nil
}

// Get a file with numParts parts. getParts will randomize the order of the
// parts to maximize the distributed transfer behavior of iomeshage when used
// at scale.
func (iom *IOMeshage) getParts(filename string, numParts int64, perm os.FileMode) {
	defer iom.destroyTempTransfer(filename)

	// corner case - empty file
	if numParts == 0 {
		fname := filepath.Join(iom.base, filename)
		log.Debug("file %v has 0 parts, creating empty file", fname)

		if err := touch(fname, perm); err != nil {
			log.Error("touch failed: %v", err)
		}

		return
	}

	// create a random list of parts to grab
	var parts []int64
	var i int64
	for i = 0; i < numParts; i++ {
		parts = append(parts, i)
	}

	// fisher-yates shuffle
	for i = numParts - 1; i > 0; i-- {
		j := iom.rand.Int63n(i + 1)
		t := parts[j]
		parts[j] = parts[i]
		parts[i] = t
	}

	// get in line
	iom.queue <- true
	defer func() {
		<-iom.queue
	}()

	iom.transferLock.Lock()
	iom.transfers[filename].Queued = false
	iom.transferLock.Unlock()

Outer:
	for _, p := range parts {
		// attempt to get this part up to MAX_ATTEMPTS attempts
		for attempt := 0; attempt < MAX_ATTEMPTS; attempt++ {
			if log.WillLog(log.DEBUG) {
				log.Debug("transferring filepart %v:%vattempt %v", filename, p, attempt)
			}

			if err := iom.getPart(filename, p); err != nil {
				log.Error("get filepart %v:%v failed: %v", filename, p, err)

				if attempt > 0 {
					// we're most likely issuing multiple attempts because of
					// heavy traffic, wait a bit for things to calm down
					time.Sleep(timeout)
				}
				continue
			}

			// success
			continue Outer
		}

		iom.transferLock.RLock()
		if !iom.transfers[filename].Parts[p] {
			log.Error("could not transfer filepart %v:%v after %v attempts", filename, p, MAX_ATTEMPTS)
			iom.transferLock.RUnlock()
			return
		}
		iom.transferLock.RUnlock()
	}

	log.Info("got all parts for %v", filename)

	// copy the parts into the whole file
	iom.transferLock.RLock()
	t := iom.transfers[filename]
	iom.transferLock.RUnlock()
	tfile, err := ioutil.TempFile(t.Dir, "cat_")
	if err != nil {
		log.Errorln(err)
	}

	for i = 0; i < numParts; i++ {
		fname := fmt.Sprintf("%v/%v.part_%v", t.Dir, filepath.Base(filename), i)
		fpart, err := os.Open(fname)
		if err != nil {
			log.Errorln(err)
			tfile.Close()
			return
		}
		io.Copy(tfile, fpart)
		fpart.Close()
	}
	name := tfile.Name()
	tfile.Close()

	// create subdirectories
	fullPath := filepath.Join(iom.base, filename)
	err = os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		log.Errorln(err)
		return
	}
	os.Rename(name, fullPath)

	log.Debug("changing permissions: %v %v", fullPath, perm)
	err = os.Chmod(fullPath, perm)
	if err != nil {
		log.Errorln(err)
	}
}

// Remove a temporary transfer directory and any transferred parts.
func (iom *IOMeshage) destroyTempTransfer(filename string) {
	iom.transferLock.RLock()
	t, ok := iom.transfers[filename]
	iom.transferLock.RUnlock()
	if !ok {
		log.Errorln("could not access transfer object!")
		return
	}

	iom.drainLock.Lock()
	defer iom.drainLock.Unlock()
	err := os.RemoveAll(t.Dir)
	if err != nil {
		log.Errorln(err)
	}
	iom.transferLock.Lock()
	delete(iom.transfers, filename)
	iom.transferLock.Unlock()
}

func (iom *IOMeshage) whoHas(filename string, p int64) (string, error) {
	TID, c := iom.newTID()
	defer iom.unregisterTID(TID)

	m := &Message{
		From:     iom.node.Name(),
		Type:     TYPE_WHOHAS,
		Filename: filename,
		TID:      TID,
		Part:     p,
	}

	recipients, err := iom.node.Broadcast(m)
	if err != nil {
		return "", err
	}
	log.Debug("sent info request to %v nodes", len(recipients))

	var timeoutCount int

	// wait for n responses, or too many timeouts
	for i := 0; i < len(recipients); i++ {
		select {
		case resp := <-c:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			if resp.ACK {
				log.Debug("%v has %v", resp.From, filename)

				return resp.From, nil
			}
		case <-time.After(timeout):
			timeoutCount++

			if timeoutCount == MAX_ATTEMPTS {
				return "", errors.New("too many timeouts")
			}
		}
	}

	return "", fmt.Errorf("who has %v: file not found", filename)
}

func (iom *IOMeshage) getPart(filename string, p int64) error {
	// did I already get this part via another node's request?
	iom.transferLock.Lock()
	if iom.transfers[filename].Parts[p] {
		iom.transferLock.Unlock()
		return nil
	}
	iom.transfers[filename].Inflight = p
	iom.transferLock.Unlock()

	who, err := iom.whoHas(filename, p)
	if err != nil {
		return err
	}

	if log.WillLog(log.DEBUG) {
		log.Debug("found part %v on node %v", p, who)
	}

	// transfer the part
	data, err := iom.xfer(filename, p, who)
	if err != nil {
		return err
	}

	iom.transferLock.Lock()
	defer iom.transferLock.Unlock()

	t, ok := iom.transfers[filename]
	if !ok {
		return fmt.Errorf("ghost transfer of %v:%v finished", filename, p)
	}

	outfile := fmt.Sprintf("%v/%v.part_%v", t.Dir, filepath.Base(filename), p)
	if err := ioutil.WriteFile(outfile, data, 0664); err != nil {
		return err
	}

	t.Parts[p] = true

	return nil
}

// xfer returns a part of the file read requested from a remote node.
func (iom *IOMeshage) xfer(filename string, part int64, from string) ([]byte, error) {
	TID, c := iom.newTID()
	defer iom.unregisterTID(TID)

	m := &Message{
		From:     iom.node.Name(),
		Type:     TYPE_XFER,
		Filename: filename,
		TID:      TID,
		Part:     part,
	}
	if _, err := iom.node.Set([]string{from}, m); err != nil {
		return nil, err
	}

	// wait for a response, or a timeout
	select {
	case resp := <-c:
		if log.WillLog(log.DEBUG) {
			log.Debugln("got part: ", resp.Part)
		}
		if resp.ACK {
			if log.WillLog(log.DEBUG) {
				log.Debugln("got part from: ", resp.From)
			}

			return resp.Data, nil
		}

		return nil, fmt.Errorf("received NACK from xfer node")
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

// Check iom messages that are routing through us in case it's a filepart that
// we're also looking for. If so, write it out. The message mux for meshage
// should call this.
func (iom *IOMeshage) MITM(m *Message) {
	if m.Type != TYPE_RESPONSE || !m.ACK || len(m.Data) == 0 {
		return
	}

	iom.transferLock.Lock()
	defer iom.transferLock.Unlock()
	if f, ok := iom.transfers[m.Filename]; ok {
		if f.Inflight == m.Part {
			return
		}
		if !f.Parts[m.Part] {
			log.Debug("snooped filepart %v;%v", f.Filename, m.Part)
			outfile := fmt.Sprintf("%v/%v.part_%v", f.Dir, filepath.Base(f.Filename), m.Part)
			err := ioutil.WriteFile(outfile, m.Data, 0664)
			if err != nil {
				log.Errorln(err)
				return
			}
			f.Parts[m.Part] = true
		}
	}
}

// Status returns a deep copy of the in-flight file transfers
func (iom *IOMeshage) Status() []*Transfer {
	iom.transferLock.RLock()
	defer iom.transferLock.RUnlock()

	res := []*Transfer{}

	for _, t := range iom.transfers {
		t2 := new(Transfer)

		// Make shallow copies of all fields
		*t2 = *t

		// Make deep copies
		t2.Parts = make(map[int64]bool)
		for k, v := range t.Parts {
			t2.Parts[k] = v
		}

		res = append(res, t2)
	}

	return res
}
