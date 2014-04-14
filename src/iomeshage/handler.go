// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package iomeshage

import (
	"fmt"
	"io"
	log "minilog"
	"os"
	"strings"
	"sync"
)

const (
	TYPE_INFO = iota
	TYPE_WHOHAS
	TYPE_XFER
	TYPE_RESPONSE
)

const (
	PART_SIZE = 10485760 // 10MB
)

var (
	TIDLock sync.Mutex
)

// IOMessage is the only structure sent between iomeshage nodes (including
// ACKS). It is used as the body of a meshage message.
type IOMMessage struct {
	From     string
	Type     int
	Filename string
	Part     int64
	TID      int64
	ACK      bool
	Data     []byte
}

// Message pump for incoming iomeshage messages.
func (iom *IOMeshage) handleMessages() {
	for {
		message := (<-iom.Messages).Body.(IOMMessage)
		m := &message
		if log.WillLog(log.DEBUG) {
			log.Debug("got iomessage from %v, type %v", m.From, m.Type)
		}
		switch m.Type {
		case TYPE_INFO:
			go iom.handleInfo(m)
		case TYPE_WHOHAS:
			go iom.handleWhohas(m)
		case TYPE_XFER:
			go iom.handleXfer(m)
		case TYPE_RESPONSE:
			go iom.handleResponse(m)
		default:
			log.Errorln("iomeshage: received invalid message type: ", m.Type)
		}
	}
}

// Handle incoming responses (ACK, file transfer, etc.). It's possible for an
// incoming response to be invalid, such as when a message times out and the
// receiver is no longer expecting the message to arrive. If so, drop the
// message. Responses are sent along registered channels, which are closed when
// the receiver gives up. If we try to send on a closed channel, recover and
// move on.
func (iom *IOMeshage) handleResponse(m *IOMMessage) {
	if c, ok := iom.TIDs[m.TID]; ok {
		defer func() {
			recover()
			if log.WillLog(log.DEBUG) {
				log.Debugln("send on closed channel recovered")
			}
		}()
		c <- m
	} else {
		log.Errorln("dropping message for invalid TID: ", m.TID)
	}
}

// Handle incoming "get file info" messages by looking up if we have the file
// and responding with the number of parts or a NACK.
func (iom *IOMeshage) handleInfo(m *IOMMessage) {
	// do we have this file, rooted at iom.base?
	resp := IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	parts, err := iom.fileInfo(iom.base + m.Filename)
	if err != nil {
		resp.ACK = false
	} else {
		resp.ACK = true
		resp.Part = parts
		if log.WillLog(log.DEBUG) {
			log.Debugln("handleInfo found file with parts: ", resp.Part)
		}
	}

	_, err = iom.node.Set([]string{m.From}, resp)
	if err != nil {
		log.Errorln("handleInfo: sending message: ", err)
	}
}

// Get file info and return the number of parts in the file.
func (iom *IOMeshage) fileInfo(filename string) (int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// we do have the file, calculate the number of parts
	fi, err := f.Stat()
	if err != nil {
		if log.WillLog(log.DEBUG) {
			log.Debugln("fileInfo error stat: ", err)
		}
		return 0, err
	}

	parts := (fi.Size() + PART_SIZE - 1) / PART_SIZE // integer divide with ceiling instead of floor
	return parts, nil
}

// Transactions need unique TIDs, and a corresponing channel to return
// responses along. Register a TID and channel for the mux to respond along.
func (iom *IOMeshage) registerTID(TID int64, c chan *IOMMessage) error {
	TIDLock.Lock()
	defer TIDLock.Unlock()
	if _, ok := iom.TIDs[TID]; ok {
		return fmt.Errorf("TID already exists, collision?")
	}
	iom.TIDs[TID] = c
	return nil
}

// Unregister TIDs from the mux.
func (iom *IOMeshage) unregisterTID(TID int64) {
	TIDLock.Lock()
	defer TIDLock.Unlock()
	if _, ok := iom.TIDs[TID]; !ok {
		log.Errorln("TID does not exist")
	} else {
		close(iom.TIDs[TID])
		delete(iom.TIDs, TID)
	}
}

// handle "who has this filepart" messages by returning an ACK if we have the file.
func (iom *IOMeshage) handleWhohas(m *IOMMessage) {
	iom.handlePart(m, false)
}

// Respond to message m with an ACK if a filepart exists, and optionally the
// contents of that filepart.
func (iom *IOMeshage) handlePart(m *IOMMessage, xfer bool) {
	// do we have this file, rooted at iom.base?
	resp := IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	iom.drainLock.RLock()
	defer iom.drainLock.RUnlock()

	_, err := iom.fileInfo(iom.base + m.Filename)
	if err != nil {
		resp.ACK = false
	} else {
		resp.ACK = true
		resp.Part = m.Part
		if xfer {
			resp.Data = iom.readPart(m.Filename, m.Part)
		}
		if log.WillLog(log.DEBUG) {
			log.Debugln("handlePart found file with parts: ", resp.Part)
		}
	}

	if resp.ACK {
		_, err = iom.node.Set([]string{m.From}, resp)
		if err != nil {
			log.Errorln("handlePart: sending message: ", err)
		}
		return
	}

	// we don't have the file in a complete state at least, do we have that specific part in flight somewhere?
	// we consider a part to be transferrable IFF it exists on disk and is marked as being fully received.
	iom.transferLock.RLock()
	if t, ok := iom.transfers[m.Filename]; ok {
		// we are currently transferring parts of the file
		if t.Parts[m.Part] {
			partname := fmt.Sprintf("%v/%v.part_%v", t.Dir, t.Filename, m.Part)
			_, err := iom.fileInfo(partname)
			if err == nil {
				// we have it
				resp.ACK = true
				resp.Part = m.Part
				if xfer {
					resp.Data = iom.readPart(partname, 0)
					log.Debug("sending partial %v", partname)
				}
			} else {
				resp.ACK = false
			}
		}
	}
	iom.transferLock.RUnlock()

	_, err = iom.node.Set([]string{m.From}, resp)
	if err != nil {
		log.Errorln("handlePart: sending message: ", err)
	}
}

// Transfer a filepart.
func (iom *IOMeshage) handleXfer(m *IOMMessage) {
	iom.handlePart(m, true)
}

// Read a filepart and return a byteslice.
func (iom *IOMeshage) readPart(filename string, part int64) []byte {
	if !strings.HasPrefix(filename, iom.base) {
		filename = iom.base + filename
	}
	f, err := os.Open(filename)
	if err != nil {
		log.Errorln(err)
		return nil
	}
	defer f.Close()

	// we do have the file, calculate the number of parts
	fi, err := f.Stat()
	if err != nil {
		log.Errorln(err)
		return nil
	}

	parts := (fi.Size() + PART_SIZE - 1) / PART_SIZE // integer divide with ceiling instead of floor
	if part > parts {
		log.Errorln("attempt to read beyond file")
		return nil
	}

	// read up to PART_SIZE
	data := make([]byte, PART_SIZE)
	n, err := f.ReadAt(data, part*PART_SIZE)

	if err != nil {
		if err != io.EOF {
			log.Errorln(err)
			return nil
		}
	}

	return data[:n]
}
