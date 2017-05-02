// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	log "minilog"
	"regexp"
	"strings"
)

type Type int

// Ron message types to inform the mux on either end how to route the message
const (
	MESSAGE_COMMAND Type = iota
	MESSAGE_CLIENT
	MESSAGE_TUNNEL
	MESSAGE_FILE
	MESSAGE_PIPE
)

const (
	HEARTBEAT_RATE = 5
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
	RESPONSE_PATH  = "miniccc_responses"
)

type Process struct {
	PID     int
	Command []string
}

type VM interface {
	GetNamespace() string
	GetUUID() string
	SetCCActive(bool)
	GetTags() map[string]string
	SetTag(string, string)
}

type Message struct {
	Type     Type
	UUID     string
	Commands map[int]*Command
	Client   *Client
	File     []byte
	Filename string
	Error    string
	Tunnel   []byte
	Pipe     string
	PipeMode int
	PipeData string
}

func (t Type) String() string {
	switch t {
	case MESSAGE_COMMAND:
		return "COMMAND"
	case MESSAGE_CLIENT:
		return "CLIENT"
	case MESSAGE_TUNNEL:
		return "TUNNEL"
	case MESSAGE_FILE:
		return "FILE"
	case MESSAGE_PIPE:
		return "PIPE"
	}

	return "UNKNOWN"
}

func unmangle(uuid string) string {
	// string must be in the form:
	//	XXXXXXXX-XXXX-XXXX-YYYY-YYYYYYYYYYYY
	// the X characters are reversed at 2 byte intervals (big/little endian for a uuid?)
	var ret string
	re := regexp.MustCompile("[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}")

	u := re.FindString(strings.ToLower(uuid))
	if uuid == "" {
		log.Fatal("uuid failed to match uuid format: %v", uuid)
	}

	ret += u[6:8]
	ret += u[4:6]
	ret += u[2:4]
	ret += u[:2]
	ret += "-"
	ret += u[11:13]
	ret += u[9:11]
	ret += "-"
	ret += u[16:18]
	ret += u[14:16]
	ret += u[18:]

	log.Debug("mangled/unmangled uuid: %v %v", u, ret)
	return ret
}
