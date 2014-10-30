// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
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

func init() {
	vyatta.Dhcp = make(map[string]*vyattaDhcp)
}

func cliVyattaClear() error {
	vyatta = vyattaConfig{
		Dhcp: make(map[string]*vyattaDhcp),
	}
	return nil
}

func cliVyatta(c cliCommand) cliResponse {
	var ret cliResponse

	if len(c.Args) == 0 {
		var dhcpKeys []string
		for k, _ := range vyatta.Dhcp {
			dhcpKeys = append(dhcpKeys, k)
		}

		var routes []string
		for _, k := range vyatta.Routes {
			routes = append(routes, k.Route)
		}

		// print vyatta info
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "IPv4 addresses\tIPv6 addresses\tRAD\tDHCP servers\tOSPF\tOSPF3\tRoutes\n")
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\n", vyatta.Ipv4, vyatta.Ipv6, vyatta.Rad, dhcpKeys, vyatta.Ospf, vyatta.Ospf3, routes)
		w.Flush()
		ret.Response = o.String()
		return ret
	}

	switch c.Args[0] {
	case "dhcp":
		if len(c.Args) == 1 {
			// print Dhcp info
			var o bytes.Buffer
			w := new(tabwriter.Writer)
			w.Init(&o, 5, 0, 1, ' ', 0)
			fmt.Fprintf(w, "Network\tGW\tStart address\tStop address\tDNS\n")
			for k, v := range vyatta.Dhcp {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", k, v.Gw, v.Start, v.Stop, v.Dns)
			}
			w.Flush()
			ret.Response = o.String()
			return ret
		}

		switch c.Args[1] {
		case "add":
			if len(c.Args) != 6 && len(c.Args) != 7 {
				ret.Error = "invalid number of arguments"
				return ret
			}
			vyatta.Dhcp[c.Args[2]] = &vyattaDhcp{
				Gw:    c.Args[3],
				Start: c.Args[4],
				Stop:  c.Args[5],
			}
			// optional dns
			if len(c.Args) == 7 {
				vyatta.Dhcp[c.Args[2]].Dns = c.Args[6]
			}
			log.Debug("vyatta add dhcp %v", vyatta.Dhcp[c.Args[2]])
		case "delete":
			if len(c.Args) != 3 {
				ret.Error = "invalid number of arguments"
				return ret
			}
			if _, ok := vyatta.Dhcp[c.Args[2]]; !ok {
				ret.Error = "no such Dhcp service"
				return ret
			}
			log.Debug("vyatta delete dhcp %v", vyatta.Dhcp[c.Args[2]])
			delete(vyatta.Dhcp, c.Args[2])
		default:
			ret.Error = "invalid vyatta Dhcp command"
			return ret
		}
	case "interfaces":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.Ipv4)
			break
		}
		vyatta.Ipv4 = c.Args[1:]
	case "interfaces6":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.Ipv6)
			break
		}
		vyatta.Ipv6 = c.Args[1:]
	case "ospf":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.Ospf)
			break
		}
		vyatta.Ospf = c.Args[1:]
	case "ospf3":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.Ospf3)
			break
		}
		vyatta.Ospf3 = c.Args[1:]
	case "rad":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.Rad)
			break
		}
		vyatta.Rad = c.Args[1:]
	case "routes":
		var ret cliResponse
		if len(c.Args) == 1 {
			// print route info
			var o bytes.Buffer
			w := new(tabwriter.Writer)
			w.Init(&o, 5, 0, 1, ' ', 0)
			fmt.Fprintf(w, "Network\tRoute\n")
			for _, v := range vyatta.Routes {
				fmt.Fprintf(w, "%v\t%v\n", v.Route, v.NextHop)
			}
			w.Flush()
			ret.Response = o.String()
			return ret
		}
		var routes []*vyattaRoute
		for _, v := range c.Args[1:] {
			d := strings.Split(v, ",")
			if len(d) != 2 {
				ret.Error = "malformed route argument"
				return ret
			}
			r := d[0]
			n := d[1]
			if !isIPv4N(r) && !isIPv6N(r) {
				ret.Error = fmt.Sprintf("%v not a valid IPv4 or IPv6 network", r)
				return ret
			}
			if !isIPv4(n) && !isIPv6(n) {
				ret.Error = fmt.Sprintf("%v not a valid IPv4 or IPv6 address", n)
				return ret
			}
			routes = append(routes, &vyattaRoute{
				Route:   r,
				NextHop: n,
			})
		}
		vyatta.Routes = routes
	case "config":
		// override everything and just cram the listed file into the floppy image
		switch len(c.Args[1:]) {
		case 0:
			ret.Response = vyatta.ConfigFile
		case 1:
			vyatta.ConfigFile = c.Args[1]
		default:
			ret.Error = "vyatta config takes 0 or 1 arguments"
		}
	case "write":
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
		var err error
		if len(c.Args) == 1 { // temporary file
			f, err = ioutil.TempFile(*f_iomBase, "vyatta_")
			if err != nil {
				log.Errorln(err)
				teardown()
			}
		} else if len(c.Args) == 2 { // named file
			filename := c.Args[1]
			if !strings.Contains(filename, "/") {
				filename = *f_iomBase + filename
			}
			f, err = os.Create(filename)
			if err != nil {
				ret.Error = err.Error()
				return ret
			}
		}
		f.Truncate(1474560)
		f.Close()

		// mkdosfs
		out, err := exec.Command(process("mkdosfs"), f.Name(), "1440").CombinedOutput()
		if err != nil {
			os.Remove(f.Name())
			ret.Error = string(out) + err.Error()
			return ret
		}

		// mount
		td, err := ioutil.TempDir(*f_base, "vyatta_")
		if err != nil {
			os.Remove(f.Name())
			ret.Error = err.Error()
			return ret
		}
		defer os.RemoveAll(td)
		out, err = exec.Command(process("mount"), "-o", "loop", f.Name(), td).CombinedOutput()
		if err != nil {
			os.Remove(f.Name())
			ret.Error = string(out) + err.Error()
			return ret
		}

		// create <floppy>/config/config.boot from vc
		err = os.Mkdir(td+"/config", 0774)
		if err != nil {
			ret.Error = err.Error()
			out, err = exec.Command(process("umount"), td).CombinedOutput()
			if err != nil {
				log.Errorln(string(out), err)
				teardown()
			}
			os.Remove(f.Name())
			return ret
		}

		if vyatta.ConfigFile == "" {
			vc := vyattaGenConfig()

			err = ioutil.WriteFile(td+"/config/config.boot", []byte(vc), 0664)
			if err != nil {
				ret.Error = err.Error()
				out, err = exec.Command(process("umount"), td).CombinedOutput()
				if err != nil {
					log.Errorln(string(out), err)
					teardown()
				}
				os.Remove(f.Name())
				return ret
			}
		} else {
			vc, err := ioutil.ReadFile(vyatta.ConfigFile)
			if err != nil {
				ret.Error = err.Error()
				out, err = exec.Command(process("umount"), td).CombinedOutput()
				if err != nil {
					log.Errorln(string(out), err)
					teardown()
				}
				os.Remove(f.Name())
				return ret
			}
			err = ioutil.WriteFile(td+"/config/config.boot", vc, 0664)
			if err != nil {
				ret.Error = err.Error()
				out, err = exec.Command(process("umount"), td).CombinedOutput()
				if err != nil {
					log.Errorln(string(out), err)
					teardown()
				}
				os.Remove(f.Name())
				return ret
			}
		}

		// umount
		out, err = exec.Command(process("umount"), td).CombinedOutput()
		if err != nil {
			os.Remove(f.Name())
			ret.Error = string(out) + err.Error()
			return ret
		}

		ret.Response = f.Name()

	default:
		ret.Error = "invalid vyatta command"
		return ret
	}

	return ret
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
