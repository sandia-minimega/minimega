/*
goircd -- minimalistic simple Internet Relay Chat (IRC) server
Copyright (C) 2014-2016 Sergey Matveev <stargrave@stargrave.org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package goircd

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"path"
	"time"
)

const (
	EventNew   = iota
	EventDel   = iota
	EventMsg   = iota
	EventTopic = iota
	EventWho   = iota
	EventMode  = iota
	EventTerm  = iota
	EventTick  = iota
	FormatMsg  = "[%s] <%s> %s\n"
	FormatMeta = "[%s] * %s %s\n"
)

var (
	logSink   chan LogEvent   = make(chan LogEvent)
	stateSink chan StateEvent = make(chan StateEvent)
)

// Client events going from each of client
// They can be either NEW, DEL or unparsed MSG
type ClientEvent struct {
	client    *Client
	eventType int
	text      string
}

func (m ClientEvent) String() string {
	return string(m.eventType) + ": " + m.client.String() + ": " + m.text
}

// Logging in-room events
// Intended to tell when, where and who send a message or meta command
type LogEvent struct {
	where string
	who   string
	what  string
	meta  bool
}

// Logging events logger itself
// Each room's events are written to separate file in logdir
// Events include messages, topic and keys changes, joining and leaving
func Logger(logdir string, events <-chan LogEvent) {
	mode := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	perm := os.FileMode(0660)
	var format string
	var logfile string
	var fd *os.File
	var err error
	for event := range events {
		logfile = path.Join(logdir, event.where+".log")
		fd, err = os.OpenFile(logfile, mode, perm)
		if err != nil {
			log.Debugln("Can not open logfile", logfile, err)
			continue
		}
		if event.meta {
			format = FormatMeta
		} else {
			format = FormatMsg
		}
		_, err = fd.WriteString(fmt.Sprintf(format, time.Now(), event.who, event.what))
		fd.Close()
		if err != nil {
			log.Debugln("Error writing to logfile", logfile, err)
		}
	}
}

type StateEvent struct {
	where string
	topic string
	key   string
}

// Room state events saver
// Room states shows that either topic or key has been changed
// Each room's state is written to separate file in statedir
func StateKeeper(statedir string, events <-chan StateEvent) {
	var fn string
	var data string
	var err error
	for event := range events {
		fn = path.Join(statedir, event.where)
		data = event.topic + "\n" + event.key + "\n"
		err = ioutil.WriteFile(fn, []byte(data), os.FileMode(0660))
		if err != nil {
			log.Debug("Can not write statefile %s: %v", fn, err)
		}
	}
}
