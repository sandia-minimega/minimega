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
	"net"
	"time"
)

// Testing network connection that satisfies net.Conn interface
// Can send predefined messages and store all written ones
type TestingConn struct {
	inbound  chan string
	outbound chan string
	closed   bool
}

func NewTestingConn() *TestingConn {
	inbound := make(chan string, 8)
	outbound := make(chan string, 8)
	return &TestingConn{inbound: inbound, outbound: outbound}
}

func (conn TestingConn) Error() string {
	return "i am finished"
}

func (conn *TestingConn) Read(b []byte) (n int, err error) {
	msg := <-conn.inbound
	if msg == "" {
		return 0, conn
	}
	for n, bt := range append([]byte(msg), CRLF...) {
		b[n] = bt
	}
	return len(msg) + 2, nil
}

type MyAddr struct{}

func (a MyAddr) String() string {
	return "someclient"
}
func (a MyAddr) Network() string {
	return "somenet"
}

func (conn *TestingConn) Write(b []byte) (n int, err error) {
	conn.outbound <- string(b)
	return len(b), nil
}

func (conn *TestingConn) Close() error {
	conn.closed = true
	close(conn.outbound)
	return nil
}

func (conn TestingConn) LocalAddr() net.Addr {
	return nil
}

func (conn TestingConn) RemoteAddr() net.Addr {
	return MyAddr{}
}

func (conn TestingConn) SetDeadline(t time.Time) error {
	return nil
}

func (conn TestingConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (conn TestingConn) SetWriteDeadline(t time.Time) error {
	return nil
}
