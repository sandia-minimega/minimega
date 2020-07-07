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
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	// Max deadline time of client's unresponsiveness
	PingTimeout = time.Second * 180
	// Max idle client's time before PING are sent
	PingThreshold = time.Second * 90
)

var (
	RENickname = regexp.MustCompile("^[a-zA-Z0-9-]{1,24}$")

	clients    map[*Client]struct{} = make(map[*Client]struct{})
	clientsM   sync.RWMutex
	rooms      map[string]*Room = make(map[string]*Room)
	roomsM     sync.RWMutex
	roomsGroup sync.WaitGroup
	roomSinks  map[*Room]chan ClientEvent = make(map[*Room]chan ClientEvent)
)

var (
	HitFunc func()
)

func HitCallback(fn func()) {
	HitFunc = fn
}

func SendLusers(client *Client) {
	lusers := 0
	clientsM.RLock()
	for client := range clients {
		if client.registered {
			lusers++
		}
	}
	clientsM.RUnlock()
	client.ReplyNicknamed("251", fmt.Sprintf("There are %d users and 0 invisible on 1 servers", lusers))
}

func SendMotd(client *Client) {
	if motd == nil {
		client.ReplyNicknamed("422", "MOTD File is missing")
		return
	}
	motdText, err := ioutil.ReadFile(*motd)
	if err != nil {
		log.Debug("Can not read motd file %s: %v", *motd, err)
		client.ReplyNicknamed("422", "Error reading MOTD File")
		return
	}
	client.ReplyNicknamed("375", "- "+*hostname+" Message of the day -")
	for _, s := range strings.Split(strings.TrimSuffix(string(motdText), "\n"), "\n") {
		client.ReplyNicknamed("372", "- "+s)
	}
	client.ReplyNicknamed("376", "End of /MOTD command")
}

func SendWhois(client *Client, nicknames []string) {
	var c *Client
	var hostPort string
	var err error
	var subscriptions []string
	var room *Room
	var subscriber *Client
	for _, nickname := range nicknames {
		nickname = strings.ToLower(nickname)
		clientsM.RLock()
		for c = range clients {
			if strings.ToLower(*c.nickname) == nickname {
				clientsM.RUnlock()
				goto Found
			}
		}
		clientsM.RUnlock()
		client.ReplyNoNickChan(nickname)
		continue
	Found:
		hostPort, _, err = net.SplitHostPort(c.conn.RemoteAddr().String())
		if err != nil {
			log.Debug("Can't parse RemoteAddr %q: %v", hostPort, err)
			hostPort = "Unknown"
		}
		client.ReplyNicknamed("311", *c.nickname, *c.username, hostPort, "*", *c.realname)
		client.ReplyNicknamed("312", *c.nickname, *hostname, *hostname)
		if c.away != nil {
			client.ReplyNicknamed("301", *c.nickname, *c.away)
		}
		subscriptions = make([]string, 0)
		roomsM.RLock()
		for _, room = range rooms {
			for subscriber = range room.members {
				if *subscriber.nickname == nickname {
					subscriptions = append(subscriptions, *room.name)
				}
			}
		}
		roomsM.RUnlock()
		sort.Strings(subscriptions)
		client.ReplyNicknamed("319", *c.nickname, strings.Join(subscriptions, " "))
		client.ReplyNicknamed("318", *c.nickname, "End of /WHOIS list")
	}
}

func SendList(client *Client, cols []string) {
	var rs []string
	var r string
	if (len(cols) > 1) && (cols[1] != "") {
		rs = strings.Split(strings.Split(cols[1], " ")[0], ",")
	} else {
		rs = make([]string, 0)
		roomsM.RLock()
		for r = range rooms {
			rs = append(rs, r)
		}
		roomsM.RUnlock()
	}
	sort.Strings(rs)
	var room *Room
	var found bool
	for _, r = range rs {
		roomsM.RLock()
		if room, found = rooms[r]; found {
			client.ReplyNicknamed(
				"322",
				r,
				fmt.Sprintf("%d", len(room.members)),
				*room.topic,
			)
		}
		roomsM.RUnlock()
	}
	client.ReplyNicknamed("323", "End of /LIST")
}

// Unregistered client workflow processor. Unregistered client:
// * is not PINGed
// * only QUIT, NICK and USER commands are processed
// * other commands are quietly ignored
// When client finishes NICK/USER workflow, then MOTD and LUSERS are send to him.
func ClientRegister(client *Client, cmd string, cols []string) {
	switch cmd {
	case "PASS":
		if len(cols) == 1 || len(cols[1]) < 1 {
			client.ReplyNotEnoughParameters("PASS")
			return
		}
		password := strings.TrimPrefix(cols[1], ":")
		client.password = &password
	case "NICK":
		if len(cols) == 1 || len(cols[1]) < 1 {
			client.ReplyParts("431", "No nickname given")
			return
		}
		nickname := cols[1]
		// Compatibility with some clients prepending colons to nickname
		nickname = strings.TrimPrefix(nickname, ":")
		nickname = strings.ToLower(nickname)
		clientsM.RLock()
		for existingClient := range clients {
			if *existingClient.nickname == nickname {
				clientsM.RUnlock()
				client.ReplyParts("433", "*", nickname, "Nickname is already in use")
				return
			}
		}
		clientsM.RUnlock()
		if !RENickname.MatchString(nickname) {
			client.ReplyParts("432", "*", cols[1], "Erroneous nickname")
			return
		}
		client.nickname = &nickname
	case "USER":
		if len(cols) == 1 {
			client.ReplyNotEnoughParameters("USER")
			return
		}
		args := strings.SplitN(cols[1], " ", 4)
		if len(args) < 4 {
			client.ReplyNotEnoughParameters("USER")
			return
		}
		client.username = &args[0]
		realname := strings.TrimLeft(args[3], ":")
		client.realname = &realname
	}
	if *client.nickname != "*" && *client.username != "" {
		if passwords != nil && *passwords != "" {
			if client.password == nil {
				client.ReplyParts("462", "You may not register")
				client.Close()
				return
			}
			contents, err := ioutil.ReadFile(*passwords)
			if err != nil {
				log.Fatal("Can no read passwords file %s: %s", *passwords, err)
				return
			}
			for _, entry := range strings.Split(string(contents), "\n") {
				if entry == "" {
					continue
				}
				if lp := strings.Split(entry, ":"); lp[0] == *client.nickname && lp[1] != *client.password {
					client.ReplyParts("462", "You may not register")
					client.Close()
					return
				}
			}
		}
		client.registered = true
		client.ReplyNicknamed("001", "Hi, welcome to IRC")
		client.ReplyNicknamed("002", "Your host is "+*hostname+", running goircd "+version)
		client.ReplyNicknamed("003", "This server was created sometime")
		client.ReplyNicknamed("004", *hostname+" goircd o o")
		SendLusers(client)
		SendMotd(client)
		log.Debugln(client, "logged in")
	}
}

// Register new room in Daemon. Create an object, events sink, save pointers
// to corresponding daemon's places and start room's processor goroutine.
func RoomRegister(name string) (*Room, chan ClientEvent) {
	roomNew := NewRoom(name)
	roomSink := make(chan ClientEvent)
	roomsM.Lock()
	rooms[name] = roomNew
	roomSinks[roomNew] = roomSink
	roomsM.Unlock()
	go roomNew.Processor(roomSink)
	roomsGroup.Add(1)
	return roomNew, roomSink
}

func HandlerJoin(client *Client, cmd string) {
	args := strings.Split(cmd, " ")
	rs := strings.Split(args[0], ",")
	var keys []string
	if len(args) > 1 {
		keys = strings.Split(args[1], ",")
	} else {
		keys = make([]string, 0)
	}
	var roomExisting *Room
	var roomSink chan ClientEvent
	var roomNew *Room
	for n, room := range rs {
		if !RoomNameValid(room) {
			client.ReplyNoChannel(room)
			continue
		}
		var key string
		if (n < len(keys)) && (keys[n] != "") {
			key = keys[n]
		} else {
			key = ""
		}
		roomsM.RLock()
		for roomExisting, roomSink = range roomSinks {
			if room == *roomExisting.name {
				roomsM.RUnlock()
				if (*roomExisting.key != "") && (*roomExisting.key != key) {
					goto Denied
				}
				roomSink <- ClientEvent{client, EventNew, ""}
				goto Joined
			}
		}
		roomsM.RUnlock()
		roomNew, roomSink = RoomRegister(room)
		log.Debugln("Room", roomNew, "created")
		if key != "" {
			roomNew.key = &key
			roomNew.StateSave()
		}
		roomSink <- ClientEvent{client, EventNew, ""}
		continue
	Denied:
		client.ReplyNicknamed("475", room, "Cannot join channel (+k) - bad key")
	Joined:
	}
}

func Processor(events chan ClientEvent, finished chan struct{}) {
	var now time.Time
	go func() {
		for {
			time.Sleep(10 * time.Second)
			events <- ClientEvent{eventType: EventTick}
		}
	}()
	for event := range events {
		now = time.Now()
		client := event.client
		HitFunc()
		switch event.eventType {
		case EventTick:
			clientsM.RLock()
			for c := range clients {
				if c.recvTimestamp.Add(PingTimeout).Before(now) {
					log.Debugln(c, "ping timeout")
					c.Close()
					continue
				}
				if c.sendTimestamp.Add(PingThreshold).Before(now) {
					if c.registered {
						c.Msg("PING :" + *hostname)
						c.sendTimestamp = time.Now()
					} else {
						log.Debugln(c, "ping timeout")
						c.Close()
					}
				}
			}
			clientsM.RUnlock()
			roomsM.Lock()
			for rn, r := range rooms {
				if *statedir == "" && len(r.members) == 0 {
					log.Debugln(rn, "emptied room")
					delete(rooms, rn)
					close(roomSinks[r])
					delete(roomSinks, r)
				}
			}
			roomsM.Unlock()
		case EventTerm:
			roomsM.RLock()
			for _, sink := range roomSinks {
				sink <- ClientEvent{eventType: EventTerm}
			}
			roomsM.RUnlock()
			roomsGroup.Wait()
			close(finished)
			return
		case EventNew:
			clientsM.Lock()
			clients[client] = struct{}{}
			clientsM.Unlock()
		case EventDel:
			clientsM.Lock()
			delete(clients, client)
			clientsM.Unlock()
			roomsM.RLock()
			for _, roomSink := range roomSinks {
				roomSink <- event
			}
			roomsM.RUnlock()
		case EventMsg:
			cols := strings.SplitN(event.text, " ", 2)
			cmd := strings.ToUpper(cols[0])
			if *verbose {
				log.Debugln(client, "command", cmd)
			}
			if cmd == "QUIT" {
				log.Debugln(client, "quit")
				client.Close()
				continue
			}
			if !client.registered {
				ClientRegister(client, cmd, cols)
				continue
			}
			if client != nil {
				client.recvTimestamp = now
			}
			switch cmd {
			case "AWAY":
				if len(cols) == 1 {
					client.away = nil
					client.ReplyNicknamed("305", "You are no longer marked as being away")
					continue
				}
				msg := strings.TrimLeft(cols[1], ":")
				client.away = &msg
				client.ReplyNicknamed("306", "You have been marked as being away")
			case "JOIN":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("JOIN")
					continue
				}
				HandlerJoin(client, cols[1])
			case "LIST":
				SendList(client, cols)
			case "LUSERS":
				SendLusers(client)
			case "MODE":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("MODE")
					continue
				}
				cols = strings.SplitN(cols[1], " ", 2)
				if cols[0] == *client.username {
					if len(cols) == 1 {
						client.Msg("221 " + *client.nickname + " +")
					} else {
						client.ReplyNicknamed("501", "Unknown MODE flag")
					}
					continue
				}
				room := cols[0]
				roomsM.RLock()
				r, found := rooms[room]
				if !found {
					client.ReplyNoChannel(room)
					roomsM.RUnlock()
					continue
				}
				if len(cols) == 1 {
					roomSinks[r] <- ClientEvent{client, EventMode, ""}
				} else {
					roomSinks[r] <- ClientEvent{client, EventMode, cols[1]}
				}
				roomsM.RUnlock()
			case "MOTD":
				SendMotd(client)
			case "PART":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("PART")
					continue
				}
				rs := strings.Split(cols[1], " ")[0]
				roomsM.RLock()
				for _, room := range strings.Split(rs, ",") {
					if r, found := rooms[room]; found {
						roomSinks[r] <- ClientEvent{client, EventDel, ""}
					} else {
						client.ReplyNoChannel(room)
					}
				}
				roomsM.RUnlock()
			case "PING":
				if len(cols) == 1 {
					client.ReplyNicknamed("409", "No origin specified")
					continue
				}
				client.Reply(fmt.Sprintf("PONG %s :%s", *hostname, cols[1]))
			case "PONG":
				continue
			case "NOTICE", "PRIVMSG":
				if len(cols) == 1 {
					client.ReplyNicknamed("411", "No recipient given ("+cmd+")")
					continue
				}
				cols = strings.SplitN(cols[1], " ", 2)
				if len(cols) == 1 {
					client.ReplyNicknamed("412", "No text to send")
					continue
				}
				msg := ""
				target := strings.ToLower(cols[0])
				clientsM.RLock()
				for c := range clients {
					if *c.nickname == target {
						msg = fmt.Sprintf(":%s %s %s %s", client, cmd, *c.nickname, cols[1])
						c.Msg(msg)
						if c.away != nil {
							client.ReplyNicknamed("301", *c.nickname, *c.away)
						}
						break
					}
				}
				clientsM.RUnlock()
				if msg != "" {
					continue
				}
				roomsM.RLock()
				if r, found := rooms[target]; found {
					roomSinks[r] <- ClientEvent{
						client,
						EventMsg,
						cmd + " " + strings.TrimLeft(cols[1], ":"),
					}
				} else {
					client.ReplyNoNickChan(target)
				}
				roomsM.RUnlock()
			case "TOPIC":
				if len(cols) == 1 {
					client.ReplyNotEnoughParameters("TOPIC")
					continue
				}
				cols = strings.SplitN(cols[1], " ", 2)
				roomsM.RLock()
				r, found := rooms[cols[0]]
				roomsM.RUnlock()
				if !found {
					client.ReplyNoChannel(cols[0])
					continue
				}
				var change string
				if len(cols) > 1 {
					change = cols[1]
				} else {
					change = ""
				}
				roomsM.RLock()
				roomSinks[r] <- ClientEvent{client, EventTopic, change}
				roomsM.RUnlock()
			case "WHO":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("WHO")
					continue
				}
				room := strings.Split(cols[1], " ")[0]
				roomsM.RLock()
				if r, found := rooms[room]; found {
					roomSinks[r] <- ClientEvent{client, EventWho, ""}
				} else {
					client.ReplyNoChannel(room)
				}
				roomsM.RUnlock()
			case "WHOIS":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("WHOIS")
					continue
				}
				cols := strings.Split(cols[1], " ")
				nicknames := strings.Split(cols[len(cols)-1], ",")
				SendWhois(client, nicknames)
			case "ISON":
				if len(cols) == 1 || len(cols[1]) < 1 {
					client.ReplyNotEnoughParameters("ISON")
					continue
				}
				nicksKnown := make(map[string]struct{})
				clientsM.RLock()
				for c := range clients {
					nicksKnown[*c.nickname] = struct{}{}
				}
				clientsM.RUnlock()
				var nicksExists []string
				for _, nickname := range strings.Split(cols[1], " ") {
					if _, exists := nicksKnown[nickname]; exists {
						nicksExists = append(nicksExists, nickname)
					}
				}
				client.ReplyNicknamed("303", strings.Join(nicksExists, " "))
			case "VERSION":
				var debug string
				if *verbose {
					debug = "debug"
				} else {
					debug = ""
				}
				client.ReplyNicknamed("351", fmt.Sprintf("%s.%s %s :", version, debug, *hostname))
			default:
				client.ReplyNicknamed("421", cmd, "Unknown command")
			}
		}
	}
}
