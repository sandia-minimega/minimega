package main

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

// dnsClient must reach an IPv6 target: before the fix it dialed "::1:53".
func TestDNSClientIPv6Target(t *testing.T) {
	queried := make(chan struct{}, 1)
	srv := &dns.Server{Addr: "[::1]:53", Net: "udp", Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
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
	go srv.ListenAndServe()
	<-started
	defer srv.Shutdown()

	go dnsClient()

	select {
	case <-queried:
	case <-time.After(3 * time.Second):
		t.Fatal("dnsClient never got an answer from IPv6 target ::1")
	}
}
