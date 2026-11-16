package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	irc "github.com/thoj/go-ircevent"
)

// ircServer and ircClient run forever and keep state in package globals
// (goircd's settings, go-ircevent's HitFunc), so start each once per test
// process. The peers below speak raw IRC over TCP instead of go-ircevent, so
// the client under test is the only go-ircevent connection that writes, and
// the only one that calls go-ircevent's global HitFunc.
var startIRC sync.Once

func waitFor(t *testing.T, what string, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ircPeer joins and parts #a until done is closed, and counts JOIN/PRIVMSG
// lines from nicks that are not peers, which can only be the client under test.
func ircPeer(t *testing.T, addr, nick string, seen *atomic.Int32, done <-chan struct{}) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Error(err)
		return
	}
	defer conn.Close()

	var wmu sync.Mutex
	send := func(format string, a ...any) {
		wmu.Lock()
		defer wmu.Unlock()
		fmt.Fprintf(conn, format+"\r\n", a...)
	}

	go func() {
		s := bufio.NewScanner(conn)
		for s.Scan() {
			line := s.Text()
			if strings.HasPrefix(line, "PING") {
				send("PONG%s", strings.TrimPrefix(line, "PING"))
				continue
			}
			f := strings.Fields(line)
			if len(f) < 2 || (f[1] != "JOIN" && f[1] != "PRIVMSG") {
				continue
			}
			from, _, _ := strings.Cut(strings.TrimPrefix(f[0], ":"), "!")
			if !strings.HasPrefix(from, "peer") {
				seen.Add(1)
			}
		}
	}()

	send("NICK %s", nick)
	send("USER %s 0 * :%s", nick, nick)
	for {
		select {
		case <-done:
			send("QUIT")
			return
		default:
		}
		send("JOIN #a")
		time.Sleep(5 * time.Millisecond)
		send("PART #a")
		time.Sleep(5 * time.Millisecond)
	}
}

// While the client under test runs its conversation loop, other users join and
// part its channel, so NAMES/JOIN/PART callbacks mutate the client's state
// concurrently. Run with -race. The client targets ::1, so it only shows up in
// the channel if it could connect to an IPv6 address.
func TestIRCClientNoRace(t *testing.T) {
	addr := "127.0.0.1:" + *f_ircport

	startIRC.Do(func() {
		go ircServer()
		waitFor(t, "the IRC server to listen", 10*time.Second, func() bool {
			c, err := net.Dial("tcp", addr)
			if err == nil {
				c.Close()
			}
			return err == nil
		})
		go ircClient()
	})

	var seen atomic.Int32
	done := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ircPeer(t, addr, fmt.Sprintf("peer%d", i), &seen, done)
		}()
	}

	// keep the churn going long enough for -race to see the callbacks overlap
	// with the conversation loop, and until the client has shown up in #a
	start := time.Now()
	waitFor(t, "the client (connected to ::1) to appear in #a", 30*time.Second, func() bool {
		return seen.Load() > 0 && time.Since(start) > 2*time.Second
	})

	close(done)
	wg.Wait()
}

// The conversation goroutine reads the client's nick while the NICK callback,
// on the connection's read goroutine, changes it. Run with -race.
func TestIRCGetNickConcurrentWithNickCallback(t *testing.T) {
	c := irc.IRC("nick", "user")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			c.RunCallbacks(&irc.Event{Code: "NICK", Nick: "nick", Arguments: []string{"nick2"}})
		}
	}()
	for i := 0; i < 100; i++ {
		_ = c.GetNick()
	}
	<-done

	if got := c.GetNick(); got != "nick2" {
		t.Fatalf("GetNick() = %q after NICK nick2, want nick2", got)
	}
}
