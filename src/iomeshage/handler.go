package iomeshage

import (
	"fmt"
	"io"
	"meshage"
	log "minilog"
	"os"
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

type IOMMessage struct {
	From     string
	Type     int
	Filename string
	Part     int64
	TID      int64
	ACK      bool
	Data     []byte
}

func (iom *IOMeshage) handleMessages() {
	for {
		message := (<-iom.Messages).Body.(IOMMessage)
		m := &message
		log.Debug("got iomessage from %v, type %v", m.From, m.Type)
		switch m.Type {
		case TYPE_INFO:
			go iom.handleInfo(m)
		case TYPE_WHOHAS:
			go iom.handleWhohas(m)
		case TYPE_XFER:
			go iom.handleXfer(m)
		case TYPE_RESPONSE:
			if c, ok := iom.TIDs[m.TID]; ok {
				defer func() {
					recover()
					log.Debugln("send on closed channel recovered")
				}()
				c <- m
			} else {
				log.Errorln("dropping message for invalid TID: ", m.TID)
			}
		default:
			log.Errorln("iomeshage: received invalid message type: ", m.Type)
		}
	}
}

func (iom *IOMeshage) handleInfo(m *IOMMessage) {
	// do we have this file, rooted at iom.base?
	resp := IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	parts, err := iom.fileInfo(m.Filename)
	if err != nil {
		resp.ACK = false
	} else {
		resp.ACK = true
		resp.Part = parts
		log.Debugln("handleInfo found file with parts: ", resp.Part)
	}

	err = iom.node.Set([]string{m.From}, meshage.UNORDERED, resp)
	if err != nil {
		log.Errorln("handleInfo: sending message: ", err)
	}
}

func (iom *IOMeshage) fileInfo(filename string) (int64, error) {
	f, err := os.Open(iom.base + filename)
	if err != nil {
		log.Debugln("fileInfo error opening file: ", err)
		return 0, err
	}

	// we do have the file, calculate the number of parts
	fi, err := f.Stat()
	if err != nil {
		log.Debugln("fileInfo error stat: ", err)
		return 0, err
	}

	parts := (fi.Size() + PART_SIZE - 1) / PART_SIZE // integer divide with ceiling instead of floor
	return parts, nil
}

func (iom *IOMeshage) registerTID(TID int64, c chan *IOMMessage) error {
	if _, ok := iom.TIDs[TID]; ok {
		return fmt.Errorf("TID already exists, collision?")
	}
	iom.TIDs[TID] = c
	return nil
}

func (iom *IOMeshage) unregisterTID(TID int64) {
	if _, ok := iom.TIDs[TID]; !ok {
		log.Errorln("TID does not exist")
	} else {
		close(iom.TIDs[TID])
		delete(iom.TIDs, TID)
	}
}

func (iom *IOMeshage) handleWhohas(m *IOMMessage) {
	iom.handlePart(m, false)
}

func (iom *IOMeshage) handlePart(m *IOMMessage, xfer bool) {
	// do we have this file, rooted at iom.base?
	resp := IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_RESPONSE,
		Filename: m.Filename,
		TID:      m.TID,
	}

	_, err := iom.fileInfo(m.Filename)
	if err != nil {
		resp.ACK = false
	} else {
		resp.ACK = true
		resp.Part = m.Part
		if xfer {
			resp.Data = iom.readPart(m.Filename, m.Part)
		}
		log.Debugln("handlePart found file with parts: ", resp.Part)
	}

	if resp.ACK {
		err = iom.node.Set([]string{m.From}, meshage.UNORDERED, resp)
		if err != nil {
			log.Errorln("handleWhohas: sending message: ", err)
		}
		return
	}

	// we don't have the file in a complete state at least, do we have that specific part in flight somewhere?
	// we consider a part to be transferrable IFF it exists on disk and is not in the in-flight list.
	if t, ok := iom.transfers[m.Filename]; ok {
		// we are currently transferring or caching parts of this file
		if t.Parts[m.Part] {
			partname := fmt.Sprintf("%v/%v.part_%v", t.Dir, t.Filename, m.Part)
			_, err := iom.fileInfo(partname)
			if err != nil {
				// we have it
				resp.ACK = true
				if xfer {
					resp.Data = iom.readPart(partname, 0)
				}
			} else {
				resp.ACK = false
			}
		}
	}

	err = iom.node.Set([]string{m.From}, meshage.UNORDERED, resp)
	if err != nil {
		log.Errorln("handleWhohas: sending message: ", err)
	}
}

func (iom *IOMeshage) handleXfer(m *IOMMessage) {
	iom.handlePart(m, true)
}

func (iom *IOMeshage) readPart(filename string, part int64) []byte {
	f, err := os.Open(iom.base + filename)
	if err != nil {
		log.Errorln(err)
		return nil
	}

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
