package iomeshage

import (
	"fmt"
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
	PART_SIZE = 104857600 // 100MB
)

type IOMMessage struct {
	From     string
	Type     int
	Filename string
	Part     int64
	TID      int64
	ACK      bool
}

func (iom *IOMeshage) handleMessages() {
	for {
		message := (<-iom.Messages).Body.(IOMMessage)
		m := &message
		log.Debug("got iomessage from %v, type %v", m.From, m.Type)
		switch m.Type {
		case TYPE_INFO:
			go iom.handleInfo(m)
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
	f, err := os.Open(iom.base + m.Filename)
	var fi os.FileInfo
	if err != nil {
		log.Debugln("handleInfo error opening file: ", err)
		resp.ACK = false
		goto HANDLEINFO_DONE
	}

	// we do have the file, calculate the number of parts
	fi, err = f.Stat()
	if err != nil {
		log.Debugln("handleInfo error stat: ", err)
		resp.ACK = false
		goto HANDLEINFO_DONE
	}

	resp.Part = (fi.Size() + PART_SIZE - 1) / PART_SIZE // integer divide with ceiling instead of floor
	resp.ACK = true
	log.Debugln("handleInfo found file with parts: ", resp.Part)
HANDLEINFO_DONE:
	err = iom.node.Set([]string{m.From}, meshage.UNORDERED, resp)
	if err != nil {
		log.Errorln("handleInfo: sending message: ", err)
	}
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
