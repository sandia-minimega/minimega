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
	"strings"
	"testing"
)

func noNickchan(t *testing.T, c *TestingConn) {
	if r := <-c.outbound; !strings.HasPrefix(r, ":foohost 401") {
		t.Fatal("no nick/channel", r)
	}
}

func noChan(t *testing.T, c *TestingConn) {
	if r := <-c.outbound; !strings.HasPrefix(r, ":foohost 403") {
		t.Fatal("no channel", r)
	}
}

func notEnoughParams(t *testing.T, c *TestingConn) {
	if r := <-c.outbound; !strings.HasPrefix(r, ":foohost 461") {
		t.Fatal("not enough params", r)
	}
}

func TestTwoUsers(t *testing.T) {
	logSink = make(chan LogEvent, 8)
	stateSink = make(chan StateEvent, 8)
	host := "foohost"
	hostname = &host
	events := make(chan ClientEvent)
	roomsM.Lock()
	rooms = make(map[string]*Room)
	roomSinks = make(map[*Room]chan ClientEvent)
	roomsM.Unlock()
	clients = make(map[*Client]struct{})
	finished := make(chan struct{})
	go Processor(events, finished)
	defer func() {
		events <- ClientEvent{eventType: EventTerm}
		<-finished
	}()

	conn1 := NewTestingConn()
	conn2 := NewTestingConn()
	client1 := NewClient(conn1)
	client2 := NewClient(conn2)
	go client1.Processor(events)
	go client2.Processor(events)

	conn1.inbound <- "NICK nick1\r\nUSER foo1 bar1 baz1 :Long name1"
	conn2.inbound <- "NICK nick2\r\nUSER foo2 bar2 baz2 :Long name2"
	for i := 0; i < 6; i++ {
		<-conn1.outbound
		<-conn2.outbound
	}

	SendLusers(client1)
	if r := <-conn1.outbound; !strings.Contains(r, "There are 2 users") {
		t.Fatal("LUSERS", r)
	}

	conn1.inbound <- "WHOIS"
	notEnoughParams(t, conn1)
	conn1.inbound <- "WHOIS nick3"
	noNickchan(t, conn1)
	conn1.inbound <- "WHOIS nick2"
	if r := <-conn1.outbound; r != ":foohost 311 nick1 nick2 foo2 Unknown * :Long name2\r\n" {
		t.Fatal("first WHOIS 311", r)
	}
	if r := <-conn1.outbound; r != ":foohost 312 nick1 nick2 foohost :foohost\r\n" {
		t.Fatal("first WHOIS 312", r)
	}
	if r := <-conn1.outbound; r != ":foohost 319 nick1 nick2 :\r\n" {
		t.Fatal("first WHOIS 319", r)
	}
	if r := <-conn1.outbound; r != ":foohost 318 nick1 nick2 :End of /WHOIS list\r\n" {
		t.Fatal("first WHOIS 318", r)
	}

	conn1.inbound <- "LIST"
	if r := <-conn1.outbound; r != ":foohost 323 nick1 :End of /LIST\r\n" {
		t.Fatal("first LIST", r)
	}

	conn1.inbound <- "WHO"
	notEnoughParams(t, conn1)
	conn1.inbound <- "WHO #fooroom"
	noChan(t, conn1)

	conn1.inbound <- "JOIN #foo"
	conn2.inbound <- "JOIN #foo"
	for i := 0; i < 4; i++ {
		<-conn1.outbound
		<-conn2.outbound
	}
	conn1.inbound <- "PRIVMSG nick2 :Hello"
	conn1.inbound <- "PRIVMSG #foo :world"
	conn1.inbound <- "NOTICE #foo :world"
	m1 := <-conn2.outbound
	m2 := <-conn2.outbound
	mNeeded := ":nick1!foo1@someclient PRIVMSG nick2 :Hello\r\n"
	if !(m1 == mNeeded || m2 == mNeeded) {
		t.Fatal("first message", m1, m2)
	}
	if m2 == mNeeded {
		m2 = <-conn2.outbound
	}
	if m2 != ":nick1!foo1@someclient PRIVMSG #foo :world\r\n" {
		t.Fatal("second message", m2)
	}
	if m2 = <-conn2.outbound; m2 != ":nick1!foo1@someclient NOTICE #foo :world\r\n" {
		t.Fatal("third message", m2)
	}
}

func TestJoin(t *testing.T) {
	logSink = make(chan LogEvent, 8)
	stateSink = make(chan StateEvent, 8)
	host := "foohost"
	hostname = &host
	events := make(chan ClientEvent)
	rooms = make(map[string]*Room)
	clients = make(map[*Client]struct{})
	roomSinks = make(map[*Room]chan ClientEvent)
	finished := make(chan struct{})
	go Processor(events, finished)
	defer func() {
		events <- ClientEvent{eventType: EventTerm}
		<-finished
	}()
	conn := NewTestingConn()
	client := NewClient(conn)
	go client.Processor(events)

	conn.inbound <- "NICK nick2\r\nUSER foo2 bar2 baz2 :Long name2"
	for i := 0; i < 6; i++ {
		<-conn.outbound
	}

	conn.inbound <- "JOIN"
	notEnoughParams(t, conn)
	conn.inbound <- "JOIN bla/bla/bla"
	noChan(t, conn)
	conn.inbound <- "JOIN bla:bla:bla"
	noChan(t, conn)

	conn.inbound <- "JOIN #foo"
	if r := <-conn.outbound; r != ":foohost 331 nick2 #foo :No topic is set\r\n" {
		t.Fatal("no topic is set", r)
	}
	if r := <-conn.outbound; r != ":nick2!foo2@someclient JOIN #foo\r\n" {
		t.Fatal("no JOIN message", r)
	}
	if r := <-conn.outbound; r != ":foohost 353 nick2 = #foo :nick2\r\n" {
		t.Fatal("no NAMES list", r)
	}
	if r := <-conn.outbound; r != ":foohost 366 nick2 #foo :End of NAMES list\r\n" {
		t.Fatal("no end of NAMES list", r)
	}
	if r := <-logSink; (r.what != "joined") || (r.where != "#foo") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("invalid join log event", r)
	}

	conn.inbound <- "JOIN #bar,#baz"
	for i := 0; i < 4*2; i++ {
		<-conn.outbound
	}
	roomsM.RLock()
	if _, ok := rooms["#bar"]; !ok {
		t.Fatal("#bar does not exist")
	}
	if _, ok := rooms["#baz"]; !ok {
		t.Fatal("#baz does not exist")
	}
	roomsM.RUnlock()
	if r := <-logSink; (r.what != "joined") || (r.where != "#bar") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("invalid join log event #bar", r)
	}
	if r := <-logSink; (r.what != "joined") || (r.where != "#baz") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("invalid join log event #baz", r)
	}

	conn.inbound <- "JOIN #barenc,#bazenc key1,key2"
	for i := 0; i < 4*2; i++ {
		<-conn.outbound
	}
	roomsM.RLock()
	if *rooms["#barenc"].key != "key1" {
		t.Fatal("no room with key1")
	}
	if *rooms["#bazenc"].key != "key2" {
		t.Fatal("no room with key2")
	}
	roomsM.RUnlock()
	if r := <-logSink; (r.what != "joined") || (r.where != "#barenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("invalid join log event #barenc", r)
	}
	if r := <-logSink; (r.what != "joined") || (r.where != "#bazenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("invalid join log event #bazenc", r)
	}
	if r := <-stateSink; (r.topic != "") || (r.where != "#barenc") || (r.key != "key1") {
		t.Fatal("set channel key1 state", r)
	}
	if r := <-stateSink; (r.topic != "") || (r.where != "#bazenc") || (r.key != "key2") {
		t.Fatal("set channel key2 state", r)
	}

	conn.inbound <- "MODE #barenc -k"
	if r := <-conn.outbound; r != ":nick2!foo2@someclient MODE #barenc -k\r\n" {
		t.Fatal("remove #barenc key", r)
	}
	roomsM.RLock()
	if *rooms["#barenc"].key != "" {
		t.Fatal("removing key from #barenc")
	}
	roomsM.RUnlock()
	if r := <-logSink; (r.what != "removed channel key") || (r.where != "#barenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("removed channel key log", r)
	}
	if r := <-stateSink; (r.topic != "") || (r.where != "#barenc") || (r.key != "") {
		t.Fatal("removed channel key state", r)
	}

	conn.inbound <- "PART #bazenc\r\nMODE #bazenc -k"
	if r := <-conn.outbound; r != ":foohost 442 #bazenc :You are not on that channel\r\n" {
		t.Fatal("not on that channel", r)
	}
	if r := <-logSink; (r.what != "left") || (r.where != "#bazenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("left #bazenc log", r)
	}

	conn.inbound <- "MODE #barenc +i"
	if r := <-conn.outbound; r != ":foohost 472 nick2 +i :Unknown MODE flag\r\n" {
		t.Fatal("unknown MODE flag", r)
	}

	conn.inbound <- "MODE #barenc +k newkey"
	if r := <-conn.outbound; r != ":nick2!foo2@someclient MODE #barenc +k newkey\r\n" {
		t.Fatal("+k MODE setting", r)
	}
	if r := <-logSink; (r.what != "set channel key to newkey") || (r.where != "#barenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("set channel key", r)
	}
	if r := <-stateSink; (r.topic != "") || (r.where != "#barenc") || (r.key != "newkey") {
		t.Fatal("set channel newkey state", r)
	}

	conn.inbound <- "TOPIC #barenc :New topic"
	if r := <-conn.outbound; r != ":nick2!foo2@someclient TOPIC #barenc :New topic\r\n" {
		t.Fatal("set TOPIC", r)
	}
	if r := <-logSink; (r.what != "set topic to New topic") || (r.where != "#barenc") || (r.who != "nick2") || (r.meta != true) {
		t.Fatal("set TOPIC log", r)
	}
	if r := <-stateSink; (r.topic != "New topic") || (r.where != "#barenc") || (r.key != "newkey") {
		t.Fatal("set channel TOPIC state", r)
	}

	conn.inbound <- "WHO #barenc"
	if r := <-conn.outbound; r != ":foohost 352 nick2 #barenc foo2 someclient foohost nick2 H :0 Long name2\r\n" {
		t.Fatal("WHO", r)
	}
	if r := <-conn.outbound; r != ":foohost 315 nick2 #barenc :End of /WHO list\r\n" {
		t.Fatal("end of WHO", r)
	}
}
