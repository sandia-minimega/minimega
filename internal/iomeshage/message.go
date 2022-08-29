// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package iomeshage

import (
	"os"
	"strconv"
	"time"
)

type MessageType int

const (
	TYPE_INFO MessageType = iota
	TYPE_WHOHAS
	TYPE_XFER
	TYPE_RESPONSE
)

// Message is the only structure sent between iomeshage nodes (including ACKS).
// It is used as the body of a meshage message.
type Message struct {
	// Type of message
	Type MessageType

	From     string
	Filename string
	Perm     os.FileMode
	ModTime  time.Time
	Hash     string
	Glob     []string
	Part     int64
	TID      int64
	ACK      bool
	Data     []byte
}

func (m MessageType) String() string {
	switch m {
	case TYPE_INFO:
		return "INFO"
	case TYPE_WHOHAS:
		return "WHOHAS"
	case TYPE_XFER:
		return "XFER"
	case TYPE_RESPONSE:
		return "RESPONSE"
	}

	return "MessageType(" + strconv.Itoa(int(m)) + ")"
}

type Messages struct {
	msgMap    map[string][]*Message
	hashMap   map[string]string
	useTstamp map[string]bool

	msgs []*Message
}

func NewMessages() *Messages {
	return &Messages{
		msgMap:    make(map[string][]*Message),
		hashMap:   make(map[string]string),
		useTstamp: make(map[string]bool),
	}
}

func (m *Messages) messages() []*Message {
	if m.msgs != nil {
		return m.msgs
	}

	for _, v := range m.msgMap {
		m.msgs = append(m.msgs, v...)
	}

	return m.msgs
}

func (m *Messages) add(message *Message) {
	msgs := m.msgMap[message.Filename]
	msgs = append(msgs, message)
	m.msgMap[message.Filename] = msgs

	if len(message.Glob) > 0 {
		return
	}

	if hash, ok := m.hashMap[message.Filename]; ok {
		if hash != message.Hash {
			m.useTstamp[message.Filename] = true
		}

		return
	}

	m.hashMap[message.Filename] = message.Hash
}

func (m Messages) use(path string) *Message {
	// Different file hashes were returned by nodes in the mesh for this file, so
	// use the node with the newest timestamp for the file.
	if m.useTstamp[path] {
		var use *Message

		for _, msg := range m.msgMap[path] {
			if use == nil { // first message
				use = msg
			}

			if msg.ModTime.After(use.ModTime) {
				use = msg
			}
		}

		return use
	}

	// All the file hashes were the same, so just use the first message present.
	for _, msg := range m.msgMap[path] {
		return msg
	}

	return nil
}
