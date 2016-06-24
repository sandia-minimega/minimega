package main

import (
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"text/template"
)

const (
	DNSMASQ_CONFIG = "/etc/dnsmasq.conf"
)

type Dnsmasq struct {
	DHCP map[string]*Dhcp
}

type Dhcp struct {
	Addr   string
	Low    string
	High   string
	Router string
	DNS    string
	Static map[string]string
}

var (
	dnsmasqData *Dnsmasq
	dnsmasqCmd  *exec.Cmd
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"dnsmasq <flush,>",
			"dnsmasq <commit,>",
			"dnsmasq <range,> <addr> <low> <high>",
			"dnsmasq option <router,> <addr> <server>",
			"dnsmasq option <dns,> <addr> <server>",
			"dnsmasq <static,> <addr> <mac> <ip>",
		},
		Call: handleDnsmasq,
	})
	dnsmasqData = &Dnsmasq{
		DHCP: make(map[string]*Dhcp),
	}
}

func handleDnsmasq(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {
		dnsmasqData = &Dnsmasq{
			DHCP: make(map[string]*Dhcp),
		}
	} else if c.BoolArgs["commit"] {
		dnsmasqConfig()
		dnsmasqRestart()
	} else if c.BoolArgs["range"] {
		addr := c.StringArgs["addr"]
		low := c.StringArgs["low"]
		high := c.StringArgs["high"]
		d := DHCPFindOrCreate(addr)
		d.Low = low
		d.High = high
	} else if c.BoolArgs["router"] {
		addr := c.StringArgs["addr"]
		server := c.StringArgs["server"]
		d := DHCPFindOrCreate(addr)
		d.Router = server
	} else if c.BoolArgs["dns"] {
		addr := c.StringArgs["addr"]
		server := c.StringArgs["server"]
		d := DHCPFindOrCreate(addr)
		d.DNS = server
	} else if c.BoolArgs["static"] {
		addr := c.StringArgs["addr"]
		mac := c.StringArgs["mac"]
		ip := c.StringArgs["ip"]
		d := DHCPFindOrCreate(addr)
		d.Static[mac] = ip
	}
}

func dnsmasqConfig() {
	t, err := template.New("dnsmasq").Parse(dnsmasqTmpl)
	if err != nil {
		log.Errorln(err)
		return
	}

	f, err := os.Create(DNSMASQ_CONFIG)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debug("executing with data:\n")
	for _, v := range dnsmasqData.DHCP {
		log.Debugln(v)
	}

	err = t.Execute(f, dnsmasqData)
	if err != nil {
		log.Errorln(err)
		return
	}
}

func DHCPFindOrCreate(addr string) *Dhcp {
	if d, ok := dnsmasqData.DHCP[addr]; ok {
		return d
	}
	d := &Dhcp{
		Addr:   addr,
		Static: make(map[string]string),
	}
	dnsmasqData.DHCP[addr] = d
	return d
}

func dnsmasqRestart() {
	if dnsmasqCmd != nil {
		err := dnsmasqCmd.Process.Kill()
		if err != nil {
			log.Errorln(err)
			return
		}
		_, err = dnsmasqCmd.Process.Wait()
		if err != nil {
			log.Errorln(err)
			return
		}
	}

	dnsmasqCmd = exec.Command("dnsmasq", "-k")
	err := dnsmasqCmd.Start()
	if err != nil {
		log.Errorln(err)
		dnsmasqCmd = nil
	}
}

var dnsmasqTmpl = `
# minirouter dnsmasq template

# don't read /etc/resolv.conf
no-resolv

# dns entries
# address=/foo.com/1.2.3.4

# dhcp
dhcp-lease-max=4294967295
{{ range $v := .DHCP }}
# {{ $v.Addr }}
{{ if ne $v.Low "" }}
	dhcp-range=set:{{ $v.Addr }},{{ $v.Low }},{{ $v.High }}
{{ end }}
{{ range $mac, $ip := $v.Static }}
	dhcp-host=set:{{ $v.Addr }},{{ $mac }},{{ $ip }}
{{ end }}
{{ if ne $v.Router "" }}
	dhcp-option = tag:{{ $v.Addr }}, option:router, {{ $v.Router }}
{{ end }}
{{ if ne $v.DNS "" }}
	dhcp-option = tag:{{ $v.Addr }}, option:dns-server, {{ $v.DNS }}
{{ end }}
{{ end }}

# todo: ipv6 route advertisements for SLAAC

# todo: logging, stats, etc. that minirouter can consume
`
