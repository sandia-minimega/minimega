// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

type vyattaConfig struct {
	Ipv4       []string
	Ipv6       []string
	Rad        []string
	Dhcp       map[string]*vyattaDhcp
	Ospf       []string
	Ospf3      []string
	Routes     []*vyattaRoute
	ConfigFile string
}

type vyattaDhcp struct {
	Gw    string
	Start string
	Stop  string
	Dns   string
}

type vyattaRoute struct {
	Route   string
	NextHop string
}

var vyatta vyattaConfig

var vyattaCLIHandlers = []minicli.Handler{
	{ // vyatta
		HelpShort: "define vyatta configuration images",
		HelpLong: `
Define and write out vyatta router floppy disk images.

vyatta takes a number of subcommands:

- 'dhcp': Add DHCP service to a particular network by specifying the network,
default gateway, and start and stop addresses. For example, to serve dhcp on
10.0.0.0/24, with a default gateway of 10.0.0.1:

	vyatta dhcp add 10.0.0.0/24 10.0.0.1 10.0.0.2 10.0.0.254

An optional DNS argument can be used to override the nameserver. For example,
to do the same as above with a nameserver of 8.8.8.8:

	vyatta dhcp add 10.0.0.0/24 10.0.0.1 10.0.0.2 10.0.0.254 8.8.8.8

Optionally, you can specify "none" for the default gateway.

- 'interfaces': Add IPv4 addresses using CIDR notation. Optionally, 'dhcp' or
'none' may be specified. The order specified matches the order of VLANs used in
vm_net. This number of arguments must either be 0 or equal to the number of
arguments in 'interfaces6' For example:

	vyatta interfaces 10.0.0.1/24 dhcp

- 'interfaces6': Add IPv6 addresses similar to 'interfaces'. The number of
arguments must either be 0 or equal to the number of arguments in 'interfaces'.

- 'rad': Enable router advertisements for IPv6. Valid arguments are IPv6
prefixes or "none". Order matches that of interfaces6. For example:

	vyatta rad 2001::/64 2002::/64

- 'ospf': Route networks using OSPF. For example:

	vyatta ospf 10.0.0.0/24 12.0.0.0/24

- 'ospf3': Route IPv6 interfaces using OSPF3. For example:

	vyatta ospf3 eth0 eth1

- 'routes': Set static routes. Routes are specified as

	<network>,<next-hop> ...

For example:

	vyatta routes 2001::0/64,123::1 10.0.0.0/24,12.0.0.1

- 'config': Override all other options and use a specified file as the config
file. For example: vyatta config /tmp/myconfig.boot

- 'write': Write the current configuration to file. If a filename is omitted, a
random filename will be used and the file placed in the path specified by the
-filepath flag. The filename will be returned.`,
		Patterns: []string{
			"vyatta",
			"vyatta <dhcp,>",
			"vyatta <dhcp,> add <network> <gateway or none> <low dhcp range> <high dhcp range> [dns server]",
			"vyatta <dhcp,> delete <network>",
			"vyatta <interfaces,> [net A.B.C.D/MASK or dhcp or none]...",
			"vyatta <interfaces6,> [net IPv6 address/MASK or none]...",
			"vyatta <rad,> [prefix]...",
			"vyatta <ospf,> [network]...",
			"vyatta <ospf3,> [network]...",
			"vyatta <routes,> [network and next-hop separated by comma]...",
			"vyatta <config,> [filename]",
			"vyatta <write,> [filename]",
		},
		Call: wrapSimpleCLI(cliVyatta),
	},
	{ // clear vyatta
		HelpShort: "reset vyatta state",
		HelpLong: `
Resets state for vyatta. See "help vyatta" for more information.`,
		Patterns: []string{
			"clear vyatta",
		},
		Call: wrapSimpleCLI(cliVyattaClear),
	},
}

func init() {
	vyatta.Dhcp = make(map[string]*vyattaDhcp)
}

func cliVyatta(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["dhcp"] {
		net := c.StringArgs["network"]

		if len(c.StringArgs) == 0 {
			// List the existing DHCP services
			resp.Header = []string{"Network", "GW", "Start address", "Stop address", "DNS"}
			resp.Tabular = [][]string{}
			for k, v := range vyatta.Dhcp {
				resp.Tabular = append(resp.Tabular, []string{k, v.Gw, v.Start, v.Stop, v.Dns})
			}
		} else if c.StringArgs["gateway"] != "" {
			// Add a new DHCP service
			vyatta.Dhcp[net] = &vyattaDhcp{
				Gw:    c.StringArgs["gateway"],
				Start: c.StringArgs["low"],
				Stop:  c.StringArgs["high"],
				Dns:   c.StringArgs["dns"],
			}

			log.Debug("vyatta add dhcp %v", vyatta.Dhcp[net])
		} else {
			// Deleting a DHCP service
			if _, ok := vyatta.Dhcp[net]; !ok {
				resp.Error = "no such Dhcp service"
			} else {
				log.Debug("vyatta delete dhcp %v", net)
				delete(vyatta.Dhcp, net)
			}
		}
	} else if c.BoolArgs["interfaces"] {
		// Get or update IPv4 interfaces
		if len(c.ListArgs) == 0 {
			resp.Response = fmt.Sprintf("%v", vyatta.Ipv4)
		} else {
			vyatta.Ipv4 = c.ListArgs["net"]
		}
	} else if c.BoolArgs["interfaces6"] {
		// Get or update IPv6 interfaces
		if len(c.ListArgs) == 0 {
			resp.Response = fmt.Sprintf("%v", vyatta.Ipv6)
		} else {
			vyatta.Ipv6 = c.ListArgs["net"]
		}
	} else if c.BoolArgs["rad"] {
		// Get or update rad
		if len(c.ListArgs) == 0 {
			resp.Response = fmt.Sprintf("%v", vyatta.Rad)
		} else {
			vyatta.Rad = c.ListArgs["prefix"]
		}
	} else if c.BoolArgs["ospf"] {
		// Get or update ospf
		if len(c.ListArgs) == 0 {
			resp.Response = fmt.Sprintf("%v", vyatta.Ospf)
		} else {
			vyatta.Ospf = c.ListArgs["network"]
		}
	} else if c.BoolArgs["ospf3"] {
		// Get or update ospf
		if len(c.ListArgs) == 0 {
			resp.Response = fmt.Sprintf("%v", vyatta.Ospf3)
		} else {
			vyatta.Ospf3 = c.ListArgs["network"]
		}
	} else if c.BoolArgs["routes"] {
		if len(c.ListArgs) == 0 {
			resp.Header = []string{"Network", "Route"}
			resp.Tabular = [][]string{}

			for _, v := range vyatta.Routes {
				resp.Tabular = append(resp.Tabular, []string{v.Route, v.NextHop})
			}
		} else {
			err := vyattaUpdateRoutes(c.ListArgs["network"])
			if err != nil {
				resp.Error = err.Error()
			}
		}
	} else if c.BoolArgs["config"] {
		// override everything and just cram the listed file into the floppy
		// image
		if len(c.StringArgs) == 0 {
			resp.Response = vyatta.ConfigFile
		} else {
			vyatta.ConfigFile = c.StringArgs["filename"]
		}
	} else if c.BoolArgs["write"] {
		var err error
		resp.Response, err = vyattaWrite(c.StringArgs["filename"])
		if err != nil {
			resp.Error = err.Error()
		}
	} else {
		// Display info about running services
		var dhcpKeys []string
		for k, _ := range vyatta.Dhcp {
			dhcpKeys = append(dhcpKeys, k)
		}

		var routes []string
		for _, k := range vyatta.Routes {
			routes = append(routes, k.Route)
		}

		resp.Header = []string{
			"IPv4 addresses",
			"IPv6 addresses",
			"RAD",
			"DHCP servers",
			"OSPF",
			"OSPF3",
			"Routes",
		}
		resp.Tabular = [][]string{[]string{
			fmt.Sprintf("%v", vyatta.Ipv4),
			fmt.Sprintf("%v", vyatta.Ipv6),
			fmt.Sprintf("%v", vyatta.Rad),
			fmt.Sprintf("%v", dhcpKeys),
			fmt.Sprintf("%v", vyatta.Ospf),
			fmt.Sprintf("%v", vyatta.Ospf3),
			fmt.Sprintf("%v", routes),
		}}
	}

	return resp
}

func cliVyattaClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vyatta = vyattaConfig{
		Dhcp: make(map[string]*vyattaDhcp),
	}

	return resp
}

func vyattaUpdateRoutes(routes []string) error {
	var newRoutes []*vyattaRoute

	for _, route := range routes {
		parts := strings.Split(route, ",")
		if len(parts) != 2 {
			return errors.New(`malformed route argument, expected "<network>,<next hop>"`)
		}

		for _, part := range parts {
			if !isIPv4N(part) && !isIPv6N(part) {
				return fmt.Errorf("%v not a valid IPv4 or IPv6 network", part)
			}

		}

		newRoutes = append(newRoutes, &vyattaRoute{
			Route:   parts[0],
			NextHop: parts[1],
		})
	}

	vyatta.Routes = newRoutes
	return nil
}

func vyattaGenConfig() string {
	tmpl, err := template.New("vyattaTemplate").Funcs(template.FuncMap{
		"dhcp": func(m map[string]*vyattaDhcp) bool {
			if len(m) > 0 {
				return true
			}
			return false
		},
		"ospf": func(s []string) bool {
			if len(s) > 0 {
				return true
			}
			return false
		},
		"ipv4": func(i int) bool {
			if len(vyatta.Ipv4) > i && vyatta.Ipv4[i] != "none" {
				return true
			}
			return false
		},
		"ipv6": func(i int) bool {
			if len(vyatta.Ipv6) > i && vyatta.Ipv6[i] != "none" {
				return true
			}
			return false
		},
		"rad": func(i int) bool {
			if len(vyatta.Rad) > i && vyatta.Ipv6[i] != "none" {
				return true
			}
			return false
		},
		"staticroutes": func() bool {
			if len(vyatta.Routes) > 0 {
				return true
			}
			return false
		},
		"route": func(r vyattaRoute) bool {
			if isIPv4N(r.Route) {
				return true
			}
			return false
		},
		"route6": func(r vyattaRoute) bool {
			if isIPv6N(r.Route) {
				return true
			}
			return false
		},
		"dns": func(d vyattaDhcp) bool {
			if d.Dns != "" {
				return true
			}
			return false
		},
		"gateway": func(d vyattaDhcp) bool {
			if d.Gw == "none" {
				return false
			}
			return true
		},
	}).Parse(vyattaConfigText)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	var o bytes.Buffer
	err = tmpl.Execute(&o, vyatta)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	log.Debugln("vyatta generated config: ", o.String())
	return o.String()
}

func isIPv4N(n string) bool {
	d := strings.Split(n, "/")

	if len(d) != 2 {
		return false
	}

	subnet, err := strconv.Atoi(d[1])
	if err != nil {
		log.Errorln(err)
		return false
	}

	if subnet < 0 || subnet > 31 {
		return false
	}

	return isIPv4(d[0])
}

func isIPv6N(n string) bool {
	d := strings.Split(n, "/")

	if len(d) != 2 {
		return false
	}

	subnet, err := strconv.Atoi(d[1])
	if err != nil {
		log.Errorln(err)
		return false
	}

	if subnet < 0 || subnet > 127 {
		return false
	}

	return isIPv6(d[0])
}

func isIPv4(ip string) bool {
	d := strings.Split(ip, ".")
	if len(d) != 4 {
		return false
	}

	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 255 {
			return false
		}
	}

	return true
}

func isIPv6(ip string) bool {
	d := strings.Split(ip, ":")
	if len(d) > 8 || len(d) < 2 {
		return false
	}

	// if there are zero or one empty groups, and all the others are <= 16 bit hex, we're good.
	// a special case is a leading ::, as in ::1, which will generate two empty groups.
	empty := false
	for i, v := range d {
		if v == "" && i == 0 {
			continue
		}
		if v == "" && !empty {
			empty = true
			continue
		}
		if v == "" {
			return false
		}
		// check for dotted quad
		if len(d) <= 6 && i == len(d)-1 && isIPv4(v) {
			return true
		}
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 65535 {
			return false
		}
	}

	return true
}

func vyattaWrite(filename string) (string, error) {
	var err, err2 error

	// make sure fields are sane
	for len(vyatta.Ipv4) != len(vyatta.Ipv6) {
		if len(vyatta.Ipv4) < len(vyatta.Ipv6) {
			vyatta.Ipv4 = append(vyatta.Ipv4, "none")
		} else {
			vyatta.Ipv6 = append(vyatta.Ipv6, "none")
		}
	}

	// create a 1.44MB file (1474560)
	var f *os.File
	if filename == "" { // temporary file
		f, err = ioutil.TempFile(*f_iomBase, "vyatta_")
		if err != nil {
			log.Errorln(err)
			teardown()
		}
	} else { // named file
		if !strings.Contains(filename, "/") {
			filename = filepath.Join(*f_iomBase, filename)
		}
		f, err = os.Create(filename)
		if err != nil {
			return "", err
		}
	}
	f.Truncate(1474560)
	f.Close()

	// mkdosfs
	out, err := processWrapper("mkdosfs", f.Name(), "1440")
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("%v %v", out, err.Error())
	}

	// mount
	td, err := ioutil.TempDir(*f_base, "vyatta_")
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	defer os.RemoveAll(td)

	out, err = processWrapper("mount", "-o", "loop", f.Name(), td)
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("%v %v", out, err.Error())
	}

	// create <floppy>/config/config.boot from vc
	err = os.Mkdir(filepath.Join(td, "config"), 0774)
	if err != nil {
		out, err2 = processWrapper("umount", td)
		if err2 != nil {
			log.Errorln(out, err)
			teardown()
		}
		os.Remove(f.Name())
		return "", err
	}

	if vyatta.ConfigFile == "" {
		vc := vyattaGenConfig()

		err = ioutil.WriteFile(filepath.Join(td, "config", "config.boot"), []byte(vc), 0664)
		if err != nil {
			out, err2 = processWrapper("umount", td)
			if err2 != nil {
				log.Errorln(out, err)
				teardown()
			}
			os.Remove(f.Name())
			return "", err
		}
	} else {
		vc, err := ioutil.ReadFile(vyatta.ConfigFile)
		if err != nil {
			out, err2 = processWrapper("umount", td)
			if err2 != nil {
				log.Errorln(out, err)
				teardown()
			}
			os.Remove(f.Name())
			return "", err
		}

		err = ioutil.WriteFile(filepath.Join(td, "config", "config.boot"), vc, 0664)
		if err != nil {
			out, err2 = processWrapper("umount", td)
			if err2 != nil {
				log.Errorln(out, err)
				teardown()
			}
			os.Remove(f.Name())
			return "", err
		}
	}

	// umount
	out, err = processWrapper("umount", td)
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("%v %v", out, err.Error())
	}

	return f.Name(), nil
}

var vyattaConfigText = `
interfaces {
    {{range $i, $v := .Ipv4}}
    ethernet eth{{$i}} {
	{{if ipv4 $i}}address {{index $.Ipv4 $i}}{{end}}
	{{if ipv6 $i}}address {{index $.Ipv6 $i}}{{end}}
	{{if rad $i}}ipv6 {
		dup-addr-detect-transmits 1
		router-advert {
			cur-hop-limit 64
			link-mtu 0
			managed-flag false
			max-interval 600
			other-config-flag false
			prefix {{index $.Rad $i}} {
				autonomous-flag true
				on-link-flag true
				valid-lifetime 2592000
			}
			reachable-time 0
			retrans-timer 0
			send-advert true
		}
	}{{end}}
        duplex auto
        smp_affinity auto
        speed auto
    }
    {{end}}
    loopback lo
}
service {
    {{if dhcp .Dhcp}}
    dhcp-server {
        disabled false
	{{range $i, $v := .Dhcp}}
        shared-network-name minimega_{{$v.Gw}} {
            authoritative disable
            subnet {{$i}} {
		{{if gateway $v}}default-router {{$v.Gw}}{{end}}
		{{if dns $v}}dns-server {{$v.Dns}}{{end}}
                lease 86400
                start {{$v.Start}} {
                    stop {{$v.Stop}}
                }
            }
        }
	{{end}}
    }
    {{end}}
}
protocols {
    {{if ospf .Ospf}}
    ospf {
        area 0.0.0.0 {
	    {{range $v := .Ospf}}
            network {{$v}}
	    {{end}}
        }
        parameters {
            abr-type cisco
        }
    }
    {{end}}
    {{if ospf .Ospf3}}
    ospfv3 {
        area 0.0.0.0 {
	    {{range $v := .Ospf3}}
            interface {{$v}}
	    {{end}}
        }
        parameters {
            abr-type cisco
        }
    }
    {{end}}
    {{if staticroutes}}static {
    {{range $i, $v := .Routes}}
    {{if route $v}}route{{end}}{{if route6 $v}}route6{{end}} {{$v.Route}} {
	    next-hop {{$v.NextHop}} {
	    }
    }
    {{end}}
    }
    {{end}}
}
system {
    config-management {
        commit-revisions 20
    }
    console {
        device ttyS0 {
            speed 9600
        }
    }
    host-name vyatta
    login {
        user vyatta {
            authentication {
                encrypted-password $1$4XHPj9eT$G3ww9B/pYDLSXC8YVvazP0
            }
            level admin
        }
    }
    syslog {
        global {
            facility all {
                level notice
            }
            facility protocols {
                level debug
            }
        }
    }
    time-zone GMT
}

/* Warning: Do not remove the following line. */
/* === vyatta-config-version: "cluster@1:config-management@1:conntrack-sync@1:conntrack@1:Dhcp-relay@1:Dhcp-server@4:firewall@5:ipsec@4:nat@4:qos@1:quagga@2:system@6:vrrp@1:wanloadbalance@3:webgui@1:webproxy@1:zone-policy@1" === */
/* Release version: VC6.6R1 */
`
