// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

var routerCLIHandlers = []minicli.Handler{
	{ // router
		HelpShort: "configure running minirouter VMs",
		HelpLong: `
Configure running minirouter VMs running minirouter and miniccc.

Routers are configured by specifying or updating a configuration, and then
applying that configuration with a commit command. For example, to configure a
router on a running VM named 'foo' to serve DHCP on 10.0.0.0/24 with a range of
IPs:

	router foo dhcp 10.0.0.0 range 10.0.0.100 10.0.0.200
	router foo commit

router takes a number of subcommands:

- 'log': Change the log level of the minirouter tool on the VM.

- 'interface': Set IPv4 or IPv6 addresses, or configure an interface to assign
  using DHCP. The interface field is an integer index of the interface defined
  with 'vm config net'. You could also specify if that interface will be a
  loopback interface For example, to configure the second interface of the
  router with a static IP and a loopback with a different IP:

	vm config net 100 200
	# ...
	router foo interface 1 10.0.0.1/24
	router foo interface 2 11.0.0.1/32 lo

- 'dhcp': Configure one or more DHCP servers on the router. The API allows you
  to set several options including static IP assignments and the default route
  and DNS server. For example, to serve a range of IPs, with 2 static IPs
  explicitly called out on router with IP 10.0.0.1:

	router vm foo dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
	router vm foo dhcp 10.0.0.0 static 00:11:22:33:44:55 10.0.0.10
	router vm foo dhcp 10.0.0.0 static 00:11:22:33:44:56 10.0.0.11

- 'dns': Set DNS records for IPv4 or IPv6 hosts.

- 'upstream': Set upstream server for DNS.

- 'gw': Set default gateway which will be used if there is no matching route.

- 'ra': Enable neighbor discovery protocol router advertisements for a given
  subnet.

- 'route': Set static, OSPF, or BGP routes. Static routes include a subnet,
  next-hop, and optionally a name for this router. For example to specify a
  static route(s):

	router foo route static 0.0.0.0/0 10.0.0.1 default-route

  OSPF routes include an area and a network index corresponding to the
  interface described in 'vm config net'. You can also specify what networks
  to advertise using the export command.

  For example, to enable OSPF on area 0 for both interfaces of a router:

	vm config net 100 200
	# ...
	router foo route ospf 0 0
	router foo route ospf 0 1

  For example, to advertise specific networks, advertise a static route or
  use a static route as a filter:

	router foo route static 11.0.0.0/24 0 bar-route
	router foo route static 12.0.0.0/24 0 bar-route
	router foo route ospf 0 export 10.0.0.0/24
	router foo route ospf 0 export default-route
	router foo route ospf 0 export bar-route

  To configure BGP must specify the process name for the specific bgp context, local ip address and AS,
  Neighbor ip address and AS, and what networks need to be advertised

  For example, local router is in AS 100 with an ip 10.0.0.1 and bgp peer is in AS 200 with an ip of 20.0.0.1
  and you want to advertise network 10.0.0.0/24:

	router foo route static 10.0.0.0/24 0 foo_out
	router foo bgp bar local 10.0.0.1 100
	router foo bgp bar neighbor 20.0.0.1 200
	router foo bgp bar export filter foo_out

  You can set up route reflection for BGP by using the rrclient command for that process.
  By using the command it indicates that the peer is a bgp client:

	router foo bgp bar rrclient

- 'rid': Sets the 32 bit router ID for the router. Typically this ID is unique
  across the organization's network and is used for various routing protocols ie OSPF

	router foo rid 1.1.1.1

- 'fw': specify flows to accept/drop/reject via iptables. For example, to
  globally globally drop all forwarded packets and accept HTTP traffic from any
  IP address to host 192.168.0.5 on the interface at index 0 (which is on the
  192.168.0.0/24 network):

	router foo fw default drop
	router foo fw accept out 0 192.168.0.5:80 tcp

  Note that we use 'out' here since we're applying the rule to the interface
  that's on the same network as the destination. The source and destination does
  not have to include a port.

  New iptables chains can also be created, providing a method for grouping rules
  together instead of adding rules at the global level. Chains are then applied
  to one or more interfaces using the interface index. For example, one could
  put the previous rule into a chain named "allow-http" and apply it to the
  interface at index 0 via the following:

	router foo fw chain allow-http default action drop
	router foo fw chain allow-http action accept 192.168.0.5:80 tcp
	router foo fw chain allow-http apply out 0
`,
		Patterns: []string{
			"router <vm>",
			"router <vm> <commit,>",
			"router <vm> <rid,> <id>",
			"router <vm> <log,> <level,> <fatal,error,warn,info,debug>",
			"router <vm> <interface,> <network> <IPv4/MASK or IPv6/MASK or dhcp> [lo,]",
			"router <vm> <dhcp,> <listen address> <range,> <low address> <high address>",
			"router <vm> <dhcp,> <listen address> <router,> <router address>",
			"router <vm> <dhcp,> <listen address> <dns,> <address>",
			"router <vm> <dhcp,> <listen address> <static,> <mac> <ip>",
			"router <vm> <dns,> <ip> <hostname>",
			"router <vm> <upstream,> <ip>",
			"router <vm> <gw,> <gw>",
			"router <vm> <ra,> <subnet>",
			"router <vm> <route,> <static,> <network> <next-hop> [staticroutename]",
			"router <vm> <route,> <ospf,> <area> <network>",
			"router <vm> <route,> <ospf,> <area> <network> <option> <value>",
			"router <vm> <route,> <ospf,> <area> <export,> <Ipv4/Mask or staticroutename>",
			"router <vm> <route,> <bgp,> <processname> <local,neighbor> <IPv4> <asnumber>",
			"router <vm> <route,> <bgp,> <processname> <rrclient,>",
			"router <vm> <route,> <bgp,> <processname> <export,> <all,filter> <filtername>",
			//"router <vm> <importbird,> <configfilepath>", TODO
			"router <vm> <fw,> <default,> <accept,drop>",
			"router <vm> <fw,> <accept,drop,reject> <in,out> <index> <dst> <proto>",
			"router <vm> <fw,> <accept,drop,reject> <in,out> <index> <src> <dst> <proto>",
			"router <vm> <fw,> chain <chain> <default,> action <accept,drop,reject>",
			"router <vm> <fw,> chain <chain> action <accept,drop,reject> <dst> <proto>",
			"router <vm> <fw,> chain <chain> action <accept,drop,reject> <src> <dst> <proto>",
			"router <vm> <fw,> chain <chain> apply <in,out> <index>",
		},
		Call:    wrapVMTargetCLI(cliRouter),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
	},
	{ // clear router
		HelpShort: "",
		HelpLong:  ``,
		Patterns: []string{
			"clear router",
			"clear router <vm>",
			"clear router <vm> <rid,>",
			"clear router <vm> <interface,>",
			"clear router <vm> <interface,> <network>",
			"clear router <vm> <interface,> <network> <IPv4/MASK or IPv6/MASK or dhcp or all> [lo,]",
			"clear router <vm> <dhcp,>",
			"clear router <vm> <dhcp,> <listen address>",
			"clear router <vm> <dhcp,> <listen address> <range,>",
			"clear router <vm> <dhcp,> <listen address> <router,>",
			"clear router <vm> <dhcp,> <listen address> <dns,>",
			"clear router <vm> <dhcp,> <listen address> <static,>",
			"clear router <vm> <dhcp,> <listen address> <static,> <mac>",
			"clear router <vm> <dns,>",
			"clear router <vm> <dns,> <ip>",
			"clear router <vm> <upstream,>",
			"clear router <vm> <gw,>",
			"clear router <vm> <ra,>",
			"clear router <vm> <ra,> <subnet>",
			"clear router <vm> <route,>",
			"clear router <vm> <route,> <static,namedstatic>",
			"clear router <vm> <route,> <static,> <network or all> [staticroutename]",
			"clear router <vm> <route,> <ospf,>",
			"clear router <vm> <route,> <ospf,> <area>",
			"clear router <vm> <route,> <ospf,> <area> <network>",
			"clear router <vm> <route,> <ospf,> <area> <export,> <Ipv4/Mask or staticroutename>",
			"clear router <vm> <route,> <bgp,> <processname>",
			"clear router <vm> <route,> <bgp,> <processname> <rrclient,>",
			"clear router <vm> <route,> <bgp,> <processname> <local,neighbor>",
			"clear router <vm> <fw,>",
		},
		Call:    wrapVMTargetCLI(cliClearRouter),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
	},
}

func cliRouter(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	vmName := c.StringArgs["vm"]

	vm := ns.FindVM(vmName)
	if vm == nil {
		return vmNotFound(vmName)
	}

	if vmName != "" && len(c.BoolArgs) == 0 { // a summary of a specific router
		rtr := ns.FindRouter(vm)
		if rtr == nil {
			return fmt.Errorf("vm %v not a router", vmName)
		}
		resp.Response = rtr.String()
	}

	rtr := ns.FindOrCreateRouter(vm)

	if c.BoolArgs["commit"] {
		return rtr.Commit(ns)
	} else if c.BoolArgs["log"] {
		var level string
		if c.BoolArgs["fatal"] {
			level = "fatal"
		} else if c.BoolArgs["error"] {
			level = "error"
		} else if c.BoolArgs["warn"] {
			level = "warn"
		} else if c.BoolArgs["info"] {
			level = "info"
		} else if c.BoolArgs["debug"] {
			level = "debug"
		}
		rtr.LogLevel(level)
		return nil
	} else if c.BoolArgs["interface"] {
		network, err := strconv.Atoi(c.StringArgs["network"])
		if err != nil {
			return fmt.Errorf("invalid network: %v : %v", c.StringArgs["network"], err)
		}
		ip := c.StringArgs["IPv4/MASK"]
		loopback := c.BoolArgs["lo"]
		return rtr.InterfaceAdd(network, ip, loopback)
	} else if c.BoolArgs["dhcp"] {
		addr := c.StringArgs["listen"]
		if c.BoolArgs["range"] {
			low := c.StringArgs["low"]
			high := c.StringArgs["high"]
			return rtr.DHCPAddRange(addr, low, high)
		} else if c.BoolArgs["router"] {
			r := c.StringArgs["router"]
			return rtr.DHCPAddRouter(addr, r)
		} else if c.BoolArgs["dns"] {
			dns := c.StringArgs["address"]
			return rtr.DHCPAddDNS(addr, dns)
		} else if c.BoolArgs["static"] {
			mac := c.StringArgs["mac"]
			ip := c.StringArgs["ip"]
			return rtr.DHCPAddStatic(addr, mac, ip)
		}
	} else if c.BoolArgs["dns"] {
		ip := c.StringArgs["ip"]
		hostname := c.StringArgs["hostname"]
		rtr.DNSAdd(ip, hostname)
		return nil
	} else if c.BoolArgs["upstream"] {
		ip := c.StringArgs["ip"]
		rtr.Upstream(ip)
		return nil
	} else if c.BoolArgs["gw"] {
		rtr.Gateway(c.StringArgs["gw"])
		return nil
	} else if c.BoolArgs["ra"] {
		subnet := c.StringArgs["subnet"]
		rtr.RADAdd(subnet)
		return nil
	} else if c.BoolArgs["route"] {
		if c.BoolArgs["static"] {
			network := c.StringArgs["network"]
			nh := c.StringArgs["next-hop"]
			if c.StringArgs["staticroutename"] != "" {
				rtr.RouteStaticAdd(network, nh, c.StringArgs["staticroutename"])
			} else {
				rtr.RouteStaticAdd(network, nh, "")
			}
			return nil
		} else if c.BoolArgs["ospf"] {
			area := c.StringArgs["area"]
			if c.StringArgs["network"] != "" {
				iface := c.StringArgs["network"]
				rtr.RouteOSPFAdd(area, iface, "")

				if opt := c.StringArgs["option"]; opt != "" {
					rtr.RouteOSPFOption(area, iface, opt, c.StringArgs["value"])
				}
			} else if c.BoolArgs["export"] {
				filter := c.StringArgs["Ipv4/Mask"]
				rtr.RouteOSPFAdd(area, "", filter)
			}
		} else if c.BoolArgs["bgp"] {
			var ip string
			islocal := false
			processname := c.StringArgs["processname"]
			if c.BoolArgs["export"] {
				if c.BoolArgs["all"] {
					rtr.ExportBGP(processname, true, "0.0.0.0/0")
				} else if c.BoolArgs["filter"] {
					rtr.ExportBGP(processname, false, c.StringArgs["filtername"])
				}
			} else if c.BoolArgs["local"] || c.BoolArgs["neighbor"] {
				ip = c.StringArgs["IPv4"]
				as, _ := strconv.Atoi(c.StringArgs["asnumber"])
				if c.BoolArgs["local"] {
					islocal = true
				}
				rtr.RouteBGPAdd(islocal, processname, ip, as)
			} else if c.BoolArgs["rrclient"] {
				rtr.bgpFindOrCreate(processname).routeReflector = true
			}
		}
	} else if c.BoolArgs["rid"] {
		if net.ParseIP(c.StringArgs["id"]) == nil {
			return fmt.Errorf("invalid routerid: %v", c.StringArgs["id"])
		}
		rtr.routerID = c.StringArgs["id"]
	} else if c.BoolArgs["fw"] {
		if vm.GetType() == CONTAINER {
			return fmt.Errorf("firewall rules cannot be applied to minirouter containers")
		}

		if chain := c.StringArgs["chain"]; chain != "" {
			if c.BoolArgs["default"] {
				if c.BoolArgs["accept"] {
					return rtr.FirewallChainDefault(chain, "accept")
				} else if c.BoolArgs["drop"] {
					return rtr.FirewallChainDefault(chain, "drop")
				} else if c.BoolArgs["reject"] {
					return rtr.FirewallChainDefault(chain, "reject")
				}

				return fmt.Errorf("unexpected default fw chain action")
			}

			if c.BoolArgs["accept"] || c.BoolArgs["drop"] || c.BoolArgs["reject"] {
				var src, dst string

				if src = c.StringArgs["src"]; src != "" {
					fields := strings.Split(src, ":")

					switch len(fields) {
					case 1: // all good here
					case 2:
						if _, err := strconv.Atoi(fields[1]); err != nil {
							return fmt.Errorf("validating fw source port %s: %v", fields[1], err)
						}
					default:
						return fmt.Errorf("malformed fw source %s", src)
					}
				}

				if dst = c.StringArgs["dst"]; dst != "" {
					fields := strings.Split(dst, ":")

					switch len(fields) {
					case 1: // all good here
					case 2:
						if _, err := strconv.Atoi(fields[1]); err != nil {
							return fmt.Errorf("validating fw destination port %s: %v", fields[1], err)
						}
					default:
						return fmt.Errorf("malformed fw destination %s", dst)
					}
				}

				var (
					proto  = c.StringArgs["proto"]
					action string
				)

				if c.BoolArgs["accept"] {
					action = "accept"
				} else if c.BoolArgs["drop"] {
					action = "drop"
				} else if c.BoolArgs["reject"] {
					action = "reject"
				}

				return rtr.FirewallChainAdd(chain, src, dst, proto, action)
			}

			if c.BoolArgs["in"] || c.BoolArgs["out"] {
				idx, err := strconv.Atoi(c.StringArgs["index"])
				if err != nil {
					return fmt.Errorf("converting fw chain interface index: %v", err)
				}

				return rtr.FirewallChainApply(idx, c.BoolArgs["in"], chain)
			}

			// if we get here, it's an unexpected error...
			return fmt.Errorf("error processing fw chain")
		}

		if c.BoolArgs["default"] {
			if c.BoolArgs["accept"] {
				return rtr.FirewallDefault("accept")
			} else if c.BoolArgs["drop"] {
				return rtr.FirewallDefault("drop")
			}

			return fmt.Errorf("unexpected default fw action")
		}

		if c.BoolArgs["accept"] || c.BoolArgs["drop"] || c.BoolArgs["reject"] {
			idx, err := strconv.Atoi(c.StringArgs["index"])
			if err != nil {
				return fmt.Errorf("converting fw interface index: %v", err)
			}

			var (
				in    = c.BoolArgs["in"]
				proto = c.StringArgs["proto"]
				src   string
				dst   string
			)

			if src = c.StringArgs["src"]; src != "" {
				fields := strings.Split(src, ":")

				switch len(fields) {
				case 1: // all good here
				case 2:
					if _, err = strconv.Atoi(fields[1]); err != nil {
						return fmt.Errorf("validating fw source port %s: %v", fields[1], err)
					}
				default:
					return fmt.Errorf("malformed fw source %s", src)
				}
			}

			if dst = c.StringArgs["dst"]; dst != "" {
				fields := strings.Split(dst, ":")

				switch len(fields) {
				case 1: // all good here
				case 2:
					if _, err = strconv.Atoi(fields[1]); err != nil {
						return fmt.Errorf("validating fw destination port %s: %v", fields[1], err)
					}
				default:
					return fmt.Errorf("malformed fw destination %s", dst)
				}
			}

			var action string

			if c.BoolArgs["accept"] {
				action = "accept"
			} else if c.BoolArgs["drop"] {
				action = "drop"
			} else if c.BoolArgs["reject"] {
				action = "reject"
			}

			return rtr.FirewallAdd(idx, in, src, dst, proto, action)
		}
	}

	return nil
}

func cliClearRouter(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	vmName := c.StringArgs["vm"]

	// clear all routers
	if vmName == "" {
		// this is safe to do because the only reference to the router
		// map is in CLI calls
		ns.routers = make(map[int]*Router)
		return nil
	}

	vm := ns.FindVM(vmName)
	if vm == nil {
		return fmt.Errorf("no such vm %v", vmName)
	}
	rtr := ns.FindRouter(vm)
	if rtr == nil {
		return fmt.Errorf("no such router %v", vmName)
	}

	if c.BoolArgs["interface"] {
		network := c.StringArgs["network"]
		ip := c.StringArgs["IPv4/MASK"]
		err := rtr.InterfaceDel(network, ip, c.BoolArgs["lo"])
		if err != nil {
			return err
		}
	} else if c.BoolArgs["dhcp"] {
		addr := c.StringArgs["listen"]

		if addr == "" {
			// clear all of it
			rtr.dhcp = make(map[string]*dhcp)
			return nil
		}

		d, ok := rtr.dhcp[addr]
		if !ok {
			return fmt.Errorf("no such address: %v", addr)
		}

		if c.BoolArgs["range"] {
			d.low = ""
			d.high = ""
		} else if c.BoolArgs["router"] {
			d.router = ""
		} else if c.BoolArgs["dns"] {
			d.dns = ""
		} else if c.BoolArgs["static"] {
			mac := c.StringArgs["mac"]
			if mac == "" {
				d.static = make(map[string]string)
			} else {
				if _, ok := d.static[mac]; ok {
					delete(d.static, mac)
				} else {
					return fmt.Errorf("no such mac: %v", mac)
				}
			}
		} else {
			delete(rtr.dhcp, addr)
		}
	} else if c.BoolArgs["dns"] {
		ip := c.StringArgs["ip"]
		return rtr.DNSDel(ip)
	} else if c.BoolArgs["upstream"] {
		return rtr.UpstreamDel()
	} else if c.BoolArgs["gw"] {
		return rtr.GatewayDel()
	} else if c.BoolArgs["ra"] {
		subnet := c.StringArgs["subnet"]
		return rtr.RADDel(subnet)
	} else if c.BoolArgs["route"] {
		if c.BoolArgs["static"] {
			network := c.StringArgs["network"]
			rtname := c.StringArgs["staticroutename"]
			if rtname == "" {
				return rtr.RouteStaticDel(network)
			}
			return rtr.NamedRouteStaticDel(network, rtname)

		} else if c.BoolArgs["namedstatic"] {
			return rtr.NamedRouteStaticDel("", "")

		} else if c.BoolArgs["ospf"] {
			area := c.StringArgs["area"]
			if c.StringArgs["network"] != "" {
				iface := c.StringArgs["network"]
				return rtr.RouteOSPFDel(area, iface)
			}
			if c.BoolArgs["export"] {
				filter := c.StringArgs["Ipv4/Mask"]
				return rtr.RouteOSPFDelFilter(area, filter)
			}
			if err := rtr.RouteOSPFDel(area, ""); err != nil {
				return err
			}
			if err := rtr.RouteOSPFDelFilter(area, ""); err != nil {
				return err
			}
			return nil
		} else if c.BoolArgs["bgp"] {
			processname := c.StringArgs["processname"]
			if c.BoolArgs["rrclient"] {
				return rtr.RouteBGPRRDel(processname)
			} else if c.BoolArgs["local"] || c.BoolArgs["neighbor"] {
				local := false
				if c.BoolArgs["local"] {
					local = true
				}
				return rtr.RouteBGPDel(processname, local, false)
			}
			return rtr.RouteBGPDel(processname, false, true)

		} else {
			// clear all routes on all protocols
			rtr.RouteStaticDel("")
			rtr.NamedRouteStaticDel("", "")
			rtr.RouteOSPFDel("", "")
			rtr.RouteBGPDel("", false, true)
		}
	} else if c.BoolArgs["rid"] {
		rtr.routerID = "0.0.0.0"
		return nil
	} else if c.BoolArgs["fw"] {
		return rtr.FirewallFlush()
	} else {
		// remove everything about this router
		err := rtr.InterfaceDel("", "", true)
		if err != nil {
			return err
		}
		rtr.DNSDel("")
		rtr.RADDel("")
		rtr.RouteStaticDel("")
		rtr.NamedRouteStaticDel("", "")
		rtr.RouteOSPFDel("", "")
		rtr.dhcp = make(map[string]*dhcp)
		rtr.RouteBGPDel("", false, true)
		rtr.routerID = "0.0.0.0"
		rtr.FirewallFlush()
	}
	return nil
}
