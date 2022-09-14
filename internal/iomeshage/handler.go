// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package iomeshage

import (
	"fmt"
	"path/filepath"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	PART_SIZE = 10485760 // 10MB
)

// Message pump for incoming iomeshage messages.
func (iom *IOMeshage) handleMessages() {
	for {
		message := (<-iom.Messages).Body.(Message)
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
			log.Error("iomeshage: received invalid message type: %v", m.Type)
		}
	}
}

// Handle incoming responses (ACK, file transfer, etc.). It's possible for an
// incoming response to be invalid, such as when a message times out, or
// multiple nodes respond to a request, and the receiver is no longer expecting
// the message to arrive. If so, drop the message. Responses are sent along
// registered channels, which are closed when the receiver gives up. If we try
// to send on a closed channel, recover and move on.
func (iom *IOMeshage) handleResponse(m *Message) {
	iom.tidLock.Lock()
	c, ok := iom.TIDs[m.TID]
	iom.tidLock.Unlock()

	if !ok {
		// This will happen when, for example, the `whoHas` function sends a
		// TYPE_WHOAS message to multiple nodes and multiple nodes respond but the
		// `whoHas` function only cares about the first response.
		if log.WillLog(log.DEBUG) {
			log.Debugln("dropping message for invalid TID: ", m.TID)
		}

		return
	}

	defer func() {
		recover()
		if log.WillLog(log.DEBUG) {
			log.Debugln("send on closed channel recovered")
		}
	}()

	c <- m
}

// Handle incoming "get file info" messages by looking up if we have the file
// and responding with the number of parts or a NACK.  Also process directories
// and globs, populating the Glob field of the Message if needed.
func (iom *IOMeshage) handleInfo(m *Message) {
	// do we have this file, rooted at iom.base?
	resp := Message{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	log.Info("handleInfo: %v", m.Filename)

	files, err := iom.List(m.Filename, true)
	if err != nil || len(files) == 0 {
		resp.ACK = false

		log.Debug("handleInfo: file does not exist: %v", m.Filename)
	} else if len(files) == 1 && iom.Rel(files[0]) == m.Filename {
		resp.ACK = !files[0].IsDir()
		resp.Part = files[0].numParts()
		resp.Perm = files[0].Perm()
		resp.ModTime = files[0].ModTime
		resp.Hash = iom.getHash(files[0].Path)

		log.Debug("handleInfo: found %v with %v parts", m.Filename, resp.Part)
	} else {
		// populate Glob
		resp.ACK = true
		for _, file := range files {
			resp.Glob = append(resp.Glob, iom.Rel(file))
		}

		log.Debug("handleInfo: found glob for %v: %v", m.Filename, resp.Glob)
	}

	_, err = iom.node.Set([]string{m.From}, resp)
	if err != nil {
		log.Errorln("handleInfo: sending message: ", err)
	}
}

// Transactions need unique TIDs, and a corresponing channel to return
// responses along. Returns a new TID and channel for the mux to respond along.
func (iom *IOMeshage) newTID() (int64, <-chan *Message) {
	iom.tidLock.Lock()
	defer iom.tidLock.Unlock()

	var tid int64
	for {
		// can't run for more than a few iterations... surely
		tid = iom.rand.Int63()

		if _, ok := iom.TIDs[tid]; !ok {
			break
		}

		log.Warn("found duplicated TID, number of TIDs: %v", len(iom.TIDs))
	}

	c := make(chan *Message)
	iom.TIDs[tid] = c
	return tid, c
}

// Unregister TIDs from the mux.
func (iom *IOMeshage) unregisterTID(TID int64) {
	iom.tidLock.Lock()
	defer iom.tidLock.Unlock()

	if _, ok := iom.TIDs[TID]; !ok {
		log.Errorln("TID does not exist")
	} else {
		close(iom.TIDs[TID])
		delete(iom.TIDs, TID)
	}
}

// handle "who has this filepart" messages by returning an ACK if we have the file.
func (iom *IOMeshage) handleWhohas(m *Message) {
	iom.handlePart(m, false)
}

// Transfer a filepart.
func (iom *IOMeshage) handleXfer(m *Message) {
	iom.handlePart(m, true)
}

// Respond to message m with an ACK if a filepart exists, and optionally the
// contents of that filepart.
func (iom *IOMeshage) handlePart(m *Message, xfer bool) {
	// do we have this file, rooted at iom.base?
	resp := Message{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	iom.drainLock.RLock()
	defer iom.drainLock.RUnlock()

	log.Info("handlePart for %v (part %v), xfer = %v", m.Filename, m.Part, xfer)

	files, err := iom.List(m.Filename, false)
	if err != nil {
		resp.ACK = false
		log.Error("invalid file %v: %v", m.Filename, err)
	} else if len(files) == 0 {
		// it's okay to not have the entire file on this node
		resp.ACK = false
	} else if len(files) == 1 {
		resp.ACK = true
		resp.Part = m.Part

		if xfer {
			resp.Data = iom.readPart(files[0].Path, m.Part)
		} else {
			resp.ModTime = files[0].ModTime
			resp.Hash = iom.getHash(files[0].Path)
		}
	} else {
		// found more than one file
		resp.ACK = false
		log.Error("invalid file %v, found %v files", m.Filename, len(files))
	}

	if resp.ACK {
		_, err = iom.node.Set([]string{m.From}, resp)
		if err != nil {
			log.Errorln("handlePart: sending message: ", err)
		}
		return
	}

	// we don't have the file in a complete state at least, do we have that
	// specific part in flight somewhere? we consider a part to be
	// transferrable IFF it exists on disk and is marked as being fully
	// received.
	iom.transferLock.RLock()
	if t, ok := iom.transfers[m.Filename]; ok && t.Parts[m.Part] {
		// we are currently transferring parts of the file
		partname := fmt.Sprintf("%v/%v.part_%v", t.Dir, filepath.Base(t.Filename), m.Part)
		_, err := iom.List(partname, false)
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
	iom.transferLock.RUnlock()

	_, err = iom.node.Set([]string{m.From}, resp)
	if err != nil {
		log.Errorln("handlePart: sending message: ", err)
	}
}
