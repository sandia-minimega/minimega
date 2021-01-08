// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	"math/rand"
	log "minilog"
	"net"
	"time"

	"github.com/miekg/dns"
)

const (
	addr  = ":53"
	ttl   = 59
	proto = "udp"
)

var questions = []uint16{dns.TypeA,
	dns.TypeAAAA,
	dns.TypeCNAME,
	dns.TypeMX,
	dns.TypeNS,
	dns.TypeSOA,
}

func dnsClient() {
	rand.Seed(time.Now().UnixNano())
	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	m := new(dns.Msg)

	for {
		t.Tick()
		h, _ := randomHost()
		d := randomDomain()
		var t uint16

		if *f_dnsv4 {
			t = dns.TypeA
		} else if *f_dnsv6 {
			t = dns.TypeAAAA
		} else {
			t = randomQuestionType()
		}

		m.SetQuestion(dns.Fqdn(d), t)
		log.Debug("dns client: question=%v", m.Question)
		in, err := dns.Exchange(m, h+addr)
		if err != nil {
			log.Debug(err.Error())
		} else {
			log.Debug("dns client: answer=%v", in)
			dnsReportChan <- 1
		}
	}
}

// Select a random domain from domains.go
func randomDomain() string {
	return domains[rand.Intn(len(domains))]
}

// Generate a random DNS request type
func randomQuestionType() uint16 {
	return questions[rand.Intn(len(questions))]
}

func dnsServer() {
	log.Debugln("dnsServer")
	rand.Seed(time.Now().UnixNano())

	dns.HandleFunc(".", handleDnsRequest)
	server := &dns.Server{Addr: addr, Net: proto}

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func handleDnsRequest(w dns.ResponseWriter, req *dns.Msg) {
	r := new(dns.Msg)
	r.SetReply(req)
	r.Authoritative = true

	if len(req.Question) > 1 || req.Rcode != dns.OpcodeQuery {
		r.SetRcode(req, dns.RcodeNotImplemented)
	}

	if len(req.Question) == 0 {
		r.SetRcode(req, dns.RcodeFormatError)
	}

	if r.Rcode != dns.RcodeSuccess {
		w.WriteMsg(r)
		dnsReportChan <- 1
		return
	}

	q := req.Question[0]
	log.Debug("dns server: question=%v type=%v remote_host=%v", q.Name, q.Qtype, w.RemoteAddr())

	switch q.Qtype {
	case dns.TypeA:
		h, _ := randomHost()
		if h == "" || !isIPv4(h) {
			if *f_randomhosts {
				h = randomIPv4Addr()
			} else {
				// return NXDOMAIN
				r.SetRcode(req, dns.RcodeNameError)
				break
			}
		}

		resp := new(dns.A)
		resp.Hdr = dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		resp.A = net.ParseIP(h)
		r.Answer = append(r.Answer, resp)
	case dns.TypeAAAA:
		h, _ := randomHost()
		if h == "" || !isIPv6(h) {
			if *f_randomhosts {
				h = randomIPv6Addr()
			} else {
				// return NXDOMAIN
				r.SetRcode(req, dns.RcodeNameError)
				break
			}
		}

		resp := new(dns.AAAA)
		resp.Hdr = dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		resp.AAAA = net.ParseIP(h)
		r.Answer = append(r.Answer, resp)
	case dns.TypeCNAME:
		resp := new(dns.CNAME)
		resp.Hdr = dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		resp.Target = fmt.Sprintf("cname.%s", q.Name)
		r.Answer = append(r.Answer, resp)
	case dns.TypeMX:
		resp := new(dns.MX)
		resp.Hdr = dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeMX,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		resp.Mx = fmt.Sprintf("mx.%s", q.Name)
		r.Answer = append(r.Answer, resp)
	case dns.TypeSOA:
		resp := new(dns.SOA)
		resp.Hdr = dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		resp.Ns = fmt.Sprintf("ns.%s", q.Name)
		resp.Mbox = fmt.Sprintf("admin-%s", q.Name)
		r.Answer = append(r.Answer, resp)
	}
	w.WriteMsg(r)
	dnsReportChan <- 1
}

func randomIPv4Addr() string {
	a := rand.Intn(256)
	b := rand.Intn(256)
	c := rand.Intn(256)
	d := rand.Intn(256)

	return fmt.Sprintf("%v.%v.%v.%v", a, b, c, d)
}

func randomIPv6Addr() string {
	ip := ""
	for i := 0; i < 8; i++ {
		if ip == "" {
			ip = fmt.Sprintf("%.4x", rand.Intn(65536))
		} else {
			ip = fmt.Sprintf("%s:%.4x", ip, rand.Intn(65536))
		}
	}
	return ip
}
