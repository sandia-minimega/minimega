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
	"regexp"
	"sort"
	"strings"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	RERoom = regexp.MustCompile("^#[^\x00\x07\x0a\x0d ,:/]{1,200}$")
)

// Sanitize room's name. It can consist of 1 to 50 ASCII symbols
// with some exclusions. All room names will have "#" prefix.
func RoomNameValid(name string) bool {
	return RERoom.MatchString(name)
}

type Room struct {
	name    *string
	topic   *string
	key     *string
	members map[*Client]struct{}
	sync.RWMutex
}

func (room *Room) String() (name string) {
	room.RLock()
	name = *room.name
	room.RUnlock()
	return
}

func NewRoom(name string) *Room {
	topic := ""
	key := ""
	return &Room{
		name:    &name,
		topic:   &topic,
		key:     &key,
		members: make(map[*Client]struct{}),
	}
}

func (room *Room) SendTopic(client *Client) {
	room.RLock()
	if *room.topic == "" {
		client.ReplyNicknamed("331", room.String(), "No topic is set")
	} else {
		client.ReplyNicknamed("332", room.String(), *room.topic)
	}
	room.RUnlock()
}

// Send message to all room's subscribers, possibly excluding someone.
func (room *Room) Broadcast(msg string, clientToIgnore ...*Client) {
	room.RLock()
	for member := range room.members {
		if (len(clientToIgnore) > 0) && member == clientToIgnore[0] {
			continue
		}
		member.Msg(msg)
	}
	room.RUnlock()
}

func (room *Room) StateSave() {
	room.RLock()
	stateSink <- StateEvent{room.String(), *room.topic, *room.key}
	room.RUnlock()
}

func (room *Room) Processor(events <-chan ClientEvent) {
	var client *Client
	for event := range events {
		client = event.client
		switch event.eventType {
		case EventTerm:
			roomsGroup.Done()
			return
		case EventNew:
			room.Lock()
			room.members[client] = struct{}{}
			if *verbose {
				log.Debugln(client, "joined", room.name)
			}
			room.Unlock()
			room.SendTopic(client)
			room.Broadcast(fmt.Sprintf(":%s JOIN %s", client, room.String()))
			logSink <- LogEvent{room.String(), *client.nickname, "joined", true}
			nicknames := make([]string, 0)
			room.RLock()
			for member := range room.members {
				nicknames = append(nicknames, *member.nickname)
			}
			room.RUnlock()
			sort.Strings(nicknames)
			client.ReplyNicknamed("353", "=", room.String(), strings.Join(nicknames, " "))
			client.ReplyNicknamed("366", room.String(), "End of NAMES list")
		case EventDel:
			room.RLock()
			if _, subscribed := room.members[client]; !subscribed {
				client.ReplyNicknamed("442", room.String(), "You are not on that channel")
				room.RUnlock()
				continue
			}
			room.RUnlock()
			room.Lock()
			delete(room.members, client)
			room.Unlock()
			room.RLock()
			msg := fmt.Sprintf(":%s PART %s :%s", client, room.String(), *client.nickname)
			room.Broadcast(msg)
			logSink <- LogEvent{room.String(), *client.nickname, "left", true}
			room.RUnlock()
		case EventTopic:
			room.RLock()
			if _, subscribed := room.members[client]; !subscribed {
				client.ReplyParts("442", room.String(), "You are not on that channel")
				room.RUnlock()
				continue
			}
			if event.text == "" {
				room.SendTopic(client)
				room.RUnlock()
				continue
			}
			room.RUnlock()
			topic := strings.TrimLeft(event.text, ":")
			room.Lock()
			room.topic = &topic
			room.Unlock()
			room.RLock()
			msg := fmt.Sprintf(":%s TOPIC %s :%s", client, room.String(), *room.topic)
			room.Broadcast(msg)
			logSink <- LogEvent{
				room.String(),
				*client.nickname,
				"set topic to " + *room.topic,
				true,
			}
			room.RUnlock()
			room.StateSave()
		case EventWho:
			room.RLock()
			for m := range room.members {
				client.ReplyNicknamed(
					"352",
					room.String(),
					*m.username,
					m.Host(),
					*hostname,
					*m.nickname,
					"H",
					"0 "+*m.realname,
				)
			}
			client.ReplyNicknamed("315", room.String(), "End of /WHO list")
			room.RUnlock()
		case EventMode:
			room.RLock()
			if event.text == "" {
				mode := "+"
				if *room.key != "" {
					mode = mode + "k"
				}
				client.Msg(fmt.Sprintf("324 %s %s %s", *client.nickname, room.String(), mode))
				room.RUnlock()
				continue
			}
			if strings.HasPrefix(event.text, "b") {
				client.ReplyNicknamed("368", room.String(), "End of channel ban list")
				room.RUnlock()
				continue
			}
			if strings.HasPrefix(event.text, "-k") || strings.HasPrefix(event.text, "+k") {
				if _, subscribed := room.members[client]; !subscribed {
					client.ReplyParts("442", room.String(), "You are not on that channel")
					room.RUnlock()
					continue
				}
			} else {
				client.ReplyNicknamed("472", event.text, "Unknown MODE flag")
				room.RUnlock()
				continue
			}
			room.RUnlock()
			var msg string
			var msgLog string
			if strings.HasPrefix(event.text, "+k") {
				cols := strings.Split(event.text, " ")
				if len(cols) == 1 {
					client.ReplyNotEnoughParameters("MODE")
					continue
				}
				room.Lock()
				room.key = &cols[1]
				msg = fmt.Sprintf(":%s MODE %s +k %s", client, *room.name, *room.key)
				msgLog = "set channel key to " + *room.key
				room.Unlock()
			} else {
				key := ""
				room.Lock()
				room.key = &key
				msg = fmt.Sprintf(":%s MODE %s -k", client, *room.name)
				room.Unlock()
				msgLog = "removed channel key"
			}
			room.Broadcast(msg)
			logSink <- LogEvent{room.String(), *client.nickname, msgLog, true}
			room.StateSave()
		case EventMsg:
			sep := strings.Index(event.text, " ")
			room.Broadcast(fmt.Sprintf(
				":%s %s %s :%s",
				client,
				event.text[:sep],
				room.String(),
				event.text[sep+1:]),
				client,
			)
			logSink <- LogEvent{
				room.String(),
				*client.nickname,
				event.text[sep+1:],
				false,
			}
		}
	}
}
