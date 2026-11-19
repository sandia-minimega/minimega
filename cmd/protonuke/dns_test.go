package main

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// dnsClient must reach an IPv6 target: before the fix it dialed "::1:53".
func TestDNSClientIPv6Target(t *testing.T) {
	queried := make(chan struct{}, 1)
	// dnsClient always dials port 53, so the server needs a privileged port
	// on the IPv6 loopback
	pc, err := net.ListenPacket("udp6", "[::1]:53")
	if err != nil {
		t.Skipf("cannot listen on [::1]:53: %v", err)
	}

	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		w.WriteMsg(m)
		select {
		case queried <- struct{}{}:
		default:
		}
	})}
	started := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(started) }
	go srv.ActivateAndServe()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("DNS server on [::1]:53 never started")
	}
	defer srv.Shutdown()

	go dnsClient()

	select {
	case <-queried:
	case <-time.After(3 * time.Second):
		t.Fatal("dnsClient never got an answer from IPv6 target ::1")
	}
}
