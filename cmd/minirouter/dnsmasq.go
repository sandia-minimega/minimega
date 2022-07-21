package main

import (
	"os"
	"os/exec"
	"text/template"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	DNSMASQ_CONFIG = "/etc/dnsmasq.conf"
)

type Dnsmasq struct {
	DHCP     map[string]*Dhcp
	DNS      map[string][]string
	RAD      map[string]bool
	Upstream string
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
			"dnsmasq <dhcp,> <range,> <addr> <low> <high>",
			"dnsmasq <dhcp,> option <router,> <addr> <server>",
			"dnsmasq <dhcp,> option <dns,> <addr> <server>",
			"dnsmasq <dhcp,> <static,> <addr> <mac> <ip>",
			"dnsmasq <dns,> <ip> <host>",
			"dnsmasq <upstream,> <ip>",
			"dnsmasq <ra,> <subnet>",
		},
		Call: handleDnsmasq,
	})
	dnsmasqData = &Dnsmasq{
		DHCP: make(map[string]*Dhcp),
		DNS:  make(map[string][]string),
		RAD:  make(map[string]bool),
	}
}

func handleDnsmasq(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {
		dnsmasqData = &Dnsmasq{
			DHCP:     make(map[string]*Dhcp),
			DNS:      make(map[string][]string),
			RAD:      make(map[string]bool),
			Upstream: "",
		}
	} else if c.BoolArgs["commit"] {
		dnsmasqConfig()
		dnsmasqRestart()
	} else if c.BoolArgs["dhcp"] {
		if c.BoolArgs["range"] {
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
	} else if c.BoolArgs["dns"] {
		ip := c.StringArgs["ip"]
		host := c.StringArgs["host"]
		dnsmasqData.DNS[host] = append(dnsmasqData.DNS[host], ip)
		log.Debug("added ip %v to host %v", ip, host)
	} else if c.BoolArgs["upstream"] {
		ip := c.StringArgs["ip"]
		dnsmasqData.Upstream = ip
	} else if c.BoolArgs["ra"] {
		subnet := c.StringArgs["subnet"]
		dnsmasqData.RAD[subnet] = true
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

	dnsmasqCmd = exec.Command("dnsmasq", "-k", "-u", "root")
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

{{ if ne .Upstream "" }}
	server={{.Upstream}}
{{ end }}

# dns entries
# address=/foo.com/1.2.3.4

# dhcp
dhcp-lease-max=4294967295
{{ range $v := .DHCP }}
# {{ $v.Addr }}
{{ if ne $v.Low "" }}
	dhcp-range=set:{{ $v.Addr }},{{ $v.Low }},{{ $v.High }}
{{ else }}
	dhcp-range=set:{{ $v.Addr }},{{ $v.Addr }},static
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
{{ range $host, $ips := .DNS }}
host-record={{ $host }}{{ range $v := $ips }},{{ $v }}{{ end }}
{{ end }}
{{ range $rad, $option := .RAD }}
dhcp-range={{ $rad }},ra-names
{{ end }}
`
