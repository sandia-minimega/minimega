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
	"testing"
)

// New client creation test. It must send an event about new client,
// two predefined messages from it and deletion one
func TestNewClient(t *testing.T) {
	conn := NewTestingConn()
	sink := make(chan ClientEvent)
	host := "foohost"
	hostname = &host
	client := NewClient(conn)
	go client.Processor(sink)

	event := <-sink
	if event.eventType != EventNew {
		t.Fatal("no NEW event", event)
	}
	conn.inbound <- "foo"
	event = <-sink
	if (event.eventType != EventMsg) || (event.text != "foo") {
		t.Fatal("no first MSG", event)
	}
	conn.inbound <- "bar"
	event = <-sink
	if (event.eventType != EventMsg) || (event.text != "bar") {
		t.Fatal("no second MSG", event)
	}
	conn.inbound <- ""
	event = <-sink
	if event.eventType != EventDel {
		t.Fatal("no client termination", event)
	}
}

// Test replies formatting
func TestClientReplies(t *testing.T) {
	conn := NewTestingConn()
	host := "foohost"
	hostname = &host
	client := NewClient(conn)
	nickname := "мойник"
	client.nickname = &nickname

	client.Reply("hello")
	if r := <-conn.outbound; r != ":foohost hello\r\n" {
		t.Fatal("did not recieve hello message", r)
	}

	client.ReplyParts("200", "foo", "bar")
	if r := <-conn.outbound; r != ":foohost 200 foo :bar\r\n" {
		t.Fatal("did not recieve 200 message", r)
	}

	client.ReplyNicknamed("200", "foo", "bar")
	if r := <-conn.outbound; r != ":foohost 200 мойник foo :bar\r\n" {
		t.Fatal("did not recieve nicknamed message", r)
	}

	client.ReplyNotEnoughParameters("CMD")
	if r := <-conn.outbound; r != ":foohost 461 мойник CMD :Not enough parameters\r\n" {
		t.Fatal("did not recieve 461 message", r)
	}
}
