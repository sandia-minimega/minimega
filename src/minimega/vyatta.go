// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"text/tabwriter"
)

type vyattaConfig struct {
	ipv4  []string
	ipv6  []string
	dhcp  map[string]*vyattaDhcp
	ospf  []string
	ospf3 []string
}

type vyattaDhcp struct {
	gw    string
	start string
	stop  string
}

var vyatta vyattaConfig

func init() {
	vyatta.dhcp = make(map[string]*vyattaDhcp)
}

func cliVyattaClear() error {
	vyatta = vyattaConfig{
		dhcp: make(map[string]*vyattaDhcp),
	}
	return nil
}

func cliVyatta(c cliCommand) cliResponse {
	var ret cliResponse

	if len(c.Args) == 0 {
		var dhcpKeys []string
		for k, _ := range vyatta.dhcp {
			dhcpKeys = append(dhcpKeys, k)
		}

		// print vyatta info
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "IPv4 addresses\tIPv6 addresses\tDHCP servers\tOSPF\tOSPF3\n")
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", vyatta.ipv4, vyatta.ipv6, dhcpKeys, vyatta.ospf, vyatta.ospf3)
		w.Flush()
		ret.Response = o.String()
		return ret
	}

	switch c.Args[0] {
	case "dhcp":
		if len(c.Args) == 1 {
			// print dhcp info
			var o bytes.Buffer
			w := new(tabwriter.Writer)
			w.Init(&o, 5, 0, 1, ' ', 0)
			fmt.Fprintf(w, "Network\tGW\tStart address\tStop address\n")
			for k, v := range vyatta.dhcp {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", k, v.gw, v.start, v.stop)
			}
			w.Flush()
			ret.Response = o.String()
			return ret
		}

		switch c.Args[1] {
		case "add":
			if len(c.Args) != 6 {
				ret.Error = "invalid number of arguments"
				return ret
			}
			vyatta.dhcp[c.Args[2]] = &vyattaDhcp{
				gw:    c.Args[3],
				start: c.Args[4],
				stop:  c.Args[5],
			}
		case "delete":
			if len(c.Args) != 3 {
				ret.Error = "invalid number of arguments"
				return ret
			}
			if _, ok := vyatta.dhcp[c.Args[2]]; !ok {
				ret.Error = "no such dhcp service"
				return ret
			}
			delete(vyatta.dhcp, c.Args[2])
		default:
			ret.Error = "invalid vyatta dhcp command"
			return ret
		}
	case "interfaces":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.ipv4)
			break
		}
		vyatta.ipv4 = c.Args[1:]
	case "interfaces6":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.ipv6)
			break
		}
		vyatta.ipv6 = c.Args[1:]
	case "launch":
	case "ospf":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.ospf)
			break
		}
		vyatta.ospf = c.Args[1:]
	case "ospf3":
		if len(c.Args) == 1 {
			ret.Response = fmt.Sprintf("%v", vyatta.ospf3)
			break
		}
		vyatta.ospf3 = c.Args[1:]
	case "write":
	default:
		ret.Error = "invalid vyatta command"
		return ret
	}

	return ret
}
