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

// Files handles all the logic for whether or not a file needs to be pulled
// from another node in the mesh. It takes into account if the mesh is running
// in -headnode mode and has -hashfiles enabled. Having -headnode mode enabled
// but -hashfiles disabled is equivalent to having -headnode mode disabled.
type Files struct {
	head      string                // node to prioritize getting files from (if set)
	msgMap    map[string][]*Message // tracks all the messages for a specific file
	hashMap   map[string]string     // tracks all the hashes for a specific file
	useTstamp map[string]bool       // tracks if the latest version of a specific file should be used

	msgs []*Message
}

func NewFiles(head string, hash bool) *Files {
	// disable -headnode mode if -hashfiles mode is disabled
	if !hash {
		head = ""
	}

	return &Files{
		head:      head,
		msgMap:    make(map[string][]*Message),
		hashMap:   make(map[string]string),
		useTstamp: make(map[string]bool),
	}
}

func (this *Files) messages() []*Message {
	if this.msgs != nil {
		return this.msgs
	}

	for _, v := range this.msgMap {
		this.msgs = append(this.msgs, v...)
	}

	return this.msgs
}

func (this *Files) add(message *Message) {
	msgs := this.msgMap[message.Filename]
	msgs = append(msgs, message)
	this.msgMap[message.Filename] = msgs

	// Don't track hashes for glob file messages. The hashMap and useTstamp maps
	// are only used in the `use` function below, which should not be called with
	// glob paths.
	if len(message.Glob) > 0 {
		return
	}

	if hash, ok := this.hashMap[message.Filename]; ok {
		if hash != message.Hash {
			this.useTstamp[message.Filename] = true
		}

		return
	}

	this.hashMap[message.Filename] = message.Hash
}

// use determines what Message should be used to get the correct version of a
// file from another node in the mesh. (nil, true) is returned when no file
// needs to be used because the local file is the correct one. (nil, false) is
// returned when we're unable to determine which Message to use (mainly because
// the path is a glob). (*Message, true) is returned when the correct Message to
// use was determined. The result of passing a glob path to this function is
// undefined.
func (this Files) use(path, hash string, local bool) (*Message, bool) {
	// If running in -headnode mode, and the file exists on the head node, and the
	// hash is different, use the file from the head node. This will also use the
	// file from the head node when the file doesn't exist locally (since the hash
	// will be different).
	if this.head != "" { // running in -headnode mode
		for _, msg := range this.msgMap[path] {
			if msg.From == this.head { // file exists on head node
				if msg.Hash == hash {
					// This will happen if the local file is the same as the file on the
					// head node.
					return nil, true
				} else {
					// This will happen if the local file is different from the file on
					// the head node or if the file does not exist locally.
					return msg, true
				}
			}
		}

		// If we get here, the file does not exist on the head node.

		// If the file exists locally (the hash is not empty), then stick with the
		// local file.
		if local {
			return nil, true
		}

		// If the file doesn't exist locally, and doesn't exist on the head node
		// either, then proceed as if we're not running in -headnode mode.
	}

	// Different file hashes were returned by nodes in the mesh for this file, so
	// use the node with the newest timestamp for the file. This will never be the
	// case when file hashing is disabled.
	if this.useTstamp[path] {
		var use *Message

		for _, msg := range this.msgMap[path] {
			if use == nil { // first message
				use = msg
			}

			if msg.ModTime.After(use.ModTime) {
				use = msg
			}
		}

		return use, true
	}

	// All the file hashes were the same, so just use the first message present.
	// This will always be the case when file hashing is disabled.
	for _, msg := range this.msgMap[path] {
		return msg, true
	}

	// Likely the path provided was a glob, so it was never present in the list of
	// messages for these files.
	return nil, false
}
