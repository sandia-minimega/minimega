package main

import (
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	BIRD_CONFIG = "/etc/bird.conf"
)

type Bird struct {
	Static4      map[string]string
	Static6      map[string]string
	NamedStatic4 map[string]map[string]string
	NamedStatic6 map[string]map[string]string
	OSPF         map[string]*OSPF
	BGP          map[string]*BGP
	RouterID     string
	ExportOSPF   bool
}

var (
	birdData *Bird
	birdCmd  *exec.Cmd
	birdID   string
)

type OSPF struct {
	Area           string
	Interfaces     map[string]map[string]string
	Prefixes       map[string]bool
	Filternetworks map[string]bool
}

type BGP struct {
	ProcessName       string
	LocalIP           string
	LocalAS           int
	NeighborIP        string
	NeighborAS        int
	RouteReflector    bool
	ExportNetworks    map[string]bool
	AdvertiseInternal bool
}

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"bird <flush,>",
			"bird <commit,>",
			"bird <routerid,> <id>",
			"bird <static,> <network> <nh> <name>",
			"bird <ospf,> <area> <network or lo>",
			"bird <ospf,> <area> <network or lo> <option> <value>",
			"bird <ospf,> <area> <filter,> <filtername or IPv4/MASK>",
			"bird <bgp,> <processname> <local,neighbor> <IPv4> <asnumber>",
			"bird <bgp,> <processname> <rrclient,>",
			"bird <bgp,> <processname> <filter,> <filtername>",
		},
		Call: handleBird,
	})
	birdID = getRouterID()
	birdData = &Bird{
		Static4:      make(map[string]string),
		Static6:      make(map[string]string),
		NamedStatic4: make(map[string]map[string]string),
		NamedStatic6: make(map[string]map[string]string),
		OSPF:         make(map[string]*OSPF),
		BGP:          make(map[string]*BGP),
		RouterID:     birdID,
		ExportOSPF:   false,
	}
}

func handleBird(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()
	log.Debugln("bird: Parsing command")
	if c.BoolArgs["flush"] {
		birdData = &Bird{
			Static4:      make(map[string]string),
			Static6:      make(map[string]string),
			NamedStatic4: make(map[string]map[string]string),
			NamedStatic6: make(map[string]map[string]string),
			OSPF:         make(map[string]*OSPF),
			BGP:          make(map[string]*BGP),
			RouterID:     birdID,
		}
	} else if c.BoolArgs["commit"] {
		birdConfig()
		birdRestart()
	} else if c.BoolArgs["static"] {
		name := c.StringArgs["name"]
		network := c.StringArgs["network"]
		nh := c.StringArgs["nh"]
		if nh == "null" {
			nh = ""
		}

		if name == "null" && nh == "" {
			log.Warnln("skipping unnamed static route: next hop not provided")
		} else if name == "null" {
			if isIPv4(nh) {
				birdData.Static4[network] = nh
			} else if isIPv6(nh) {
				birdData.Static6[network] = nh
			}
		} else if isIPv4(nh) {
			if birdData.NamedStatic4[name] == nil {
				birdData.NamedStatic4[name] = make(map[string]string)
			}
			birdData.NamedStatic4[name][network] = nh
		} else if isIPv6(nh) {
			if birdData.NamedStatic6[name] == nil {
				birdData.NamedStatic6[name] = make(map[string]string)
			}
			birdData.NamedStatic6[name][network] = nh
		}
	} else if c.BoolArgs["ospf"] {
		area := c.StringArgs["area"]
		if c.BoolArgs["filter"] {
			o := OSPFFindOrCreate(area)
			birdData.ExportOSPF = true
			if strings.Contains(c.StringArgs["filtername"], "/") {
				o.Prefixes[c.StringArgs["filtername"]] = true
			} else {
				o.Filternetworks[c.StringArgs["filtername"]] = true
			}
		} else {
			network := c.StringArgs["network"]
			var iface string
			if network == "lo" {
				iface = "lo"
			} else {
				var idx int
				var err error
				// get an interface from the index
				idx, err = strconv.Atoi(network)
				if err != nil {
					log.Errorln(err)
					return
				}

				iface, err = findEth(idx)
				if err != nil {
					log.Errorln(err)
					return
				}
			}

			o := OSPFFindOrCreate(area)
			if _, ok := o.Interfaces[iface]; !ok {
				o.Interfaces[iface] = make(map[string]string)
			}

			// set options, if any
			if opt := c.StringArgs["option"]; opt != "" {
				o.Interfaces[iface][opt] = c.StringArgs["value"]
			}
		}
	} else if c.BoolArgs["bgp"] {
		var ip string
		processname := c.StringArgs["processname"]
		log.Debugln("bird: Looking for Bgp process")
		b := bgpFindOrCreate(processname)
		log.Debug("bird: Found BGP process %v", b.ProcessName)
		if c.BoolArgs["local"] || c.BoolArgs["neighbor"] {
			ip = c.StringArgs["IPv4"]
			as, err := strconv.Atoi(c.StringArgs["asnumber"])
			if err != nil {
				log.Errorln(err)
				return
			}
			if c.BoolArgs["local"] {
				log.Debug("bird: Setting local IP %v and AS %v\n", ip, as)
				b.LocalIP = ip
				b.LocalAS = as
			} else if c.BoolArgs["neighbor"] {
				log.Debug("bird: Setting neighbor IP %v and AS %v\n", ip, as)
				b.NeighborIP = ip
				b.NeighborAS = as
			}
		} else if c.BoolArgs["rrclient"] {
			b.RouteReflector = true
		} else if c.BoolArgs["filter"] {
			log.Debug("bird: adding filter %v", c.StringArgs["filtername"])
			if c.StringArgs["filtername"] != "all" {
				b.AdvertiseInternal = true
			}
			b.ExportNetworks[c.StringArgs["filtername"]] = true
		}
	} else if c.BoolArgs["routerid"] {
		birdData.RouterID = c.StringArgs["id"]
	}
}

func birdConfig() {
	log.Debugln("bird: preparing template")
	t, err := template.New("bird").Parse(birdTmpl)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debugln("bird: creating file")
	// First, IPv4
	f, err := os.Create(BIRD_CONFIG)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debugln("bird: executing template")
	err = t.Execute(f, birdData)
	if err != nil {
		log.Errorln(err)
		return
	}
}

func birdRestart() {
	if birdCmd != nil {
		err := birdCmd.Process.Kill()
		if err != nil {
			log.Errorln(err)
			return
		}
		_, err = birdCmd.Process.Wait()
		if err != nil {
			log.Errorln(err)
			return
		}
	}

	birdCmd = exec.Command("bird", "-f", "-s", "/bird.sock", "-P", "/bird.pid", "-c", BIRD_CONFIG)
	err := birdCmd.Start()
	if err != nil {
		log.Errorln(err)
		birdCmd = nil
	}
}

// Returns OSPF Area for the router
func OSPFFindOrCreate(area string) *OSPF {
	if o, ok := birdData.OSPF[area]; ok {
		return o
	}
	o := &OSPF{
		Area:           area,
		Interfaces:     make(map[string]map[string]string),
		Prefixes:       make(map[string]bool),
		Filternetworks: make(map[string]bool),
	}
	birdData.OSPF[area] = o
	return o
}

// Returns BGP Area for the router
func bgpFindOrCreate(bgpprocess string) *BGP {
	if b, ok := birdData.BGP[bgpprocess]; ok {
		return b
	}
	b := &BGP{
		ProcessName:    bgpprocess,
		ExportNetworks: make(map[string]bool),
	}
	birdData.BGP[bgpprocess] = b
	return b
}

// In case Minirouter doesnt get a router ID
func getRouterID() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	p := make([]byte, 4)
	_, err := r.Read(p)
	if err != nil {
		log.Fatalln(err)
	}
	ip := net.IPv4(p[0], p[1], p[2], p[3])
	return ip.String()
}

// Generates the filter config lines for the template
func (b *BGP) GenerateFilter() string {
	filter := ""
	if _, ok := b.ExportNetworks["all"]; ok {
		filter = "source = RTS_BGP"
	}
	if b.AdvertiseInternal {
		for rt := range b.ExportNetworks {
			if rt != "all" {
				if filter != "" {
					filter += " || "
				}
				filter += "proto = \"static_" + rt + "\""
			}
		}
	}
	return filter
}

var birdTmpl = `
# minirouter bird template

router id {{ .RouterID }};

protocol kernel kernel_ipv4 {
  scan time 60;

  ipv4 {
    import none;
    export all;   # Actually insert routes into the kernel routing table
  };

	learn;
}

protocol kernel kernel_ipv6 {
  scan time 60;

  ipv6 {
    import none;
    export all;   # Actually insert routes into the kernel routing table
  };

	learn;
}

# The Device protocol is not a real routing protocol. It doesn't generate any
# routes and it only serves as a module for getting information about network
# interfaces from the kernel.
protocol device {
  scan time 60;
}

{{ $DOSTATIC := len .Static4 }}
{{ if ne $DOSTATIC 0 }}
#static routes
protocol static static_ipv4 {
	check link;

  ipv4 {
    import all;
  };

{{ range $network, $nh := .Static4 }}
	route {{ $network }} via {{ $nh }};
{{ end }}
}
{{ end }}

{{ $DOSTATIC := len .Static6 }}
{{ if ne $DOSTATIC 0 }}
#static routes
protocol static static_ipv6 {
	check link;

  ipv6 {
    import all;
  };

{{ range $network, $nh := .Static6 }}
	route {{ $network }} via {{ $nh }};
{{ end }}
}
{{ end }}

{{ $DOSTATIC := len .NamedStatic4 }}
{{ if ne $DOSTATIC 0 }}
#Named IPv4 static routes
{{ range $name, $network := .NamedStatic4 }}
protocol static static_{{$name}}_ipv4 {
	check link;

  ipv4 {
    import all;
  };

{{ range $net, $nh := $network }}
	{{ if ne $nh "" }}
	route {{ $net }} via {{ $nh }};
	{{ else }}
	route {{ $net }} reject;
	{{ end }}
{{ end }}
}
{{ end }}
{{ end }}

{{ $DOSTATIC := len .NamedStatic6 }}
{{ if ne $DOSTATIC 0 }}
#Named IPv6 static routes
{{ range $name, $network := .NamedStatic6 }}
protocol static static_{{$name}}_ipv6 {
	check link;

  ipv6 {
    import all;
  };

{{ range $net, $nh := $network }}
	{{ if ne $nh "" }}
	route {{ $net }} via {{ $nh }};
	{{ else }}
	route {{ $net }} reject;
	{{ end }}
{{ end }}
}
{{ end }}
{{ end }}

{{ $DOOSPF := len .OSPF }}
{{ if ne $DOOSPF 0 }}
protocol ospf {
	ipv4 {
		import all;
		{{ if .ExportOSPF}}
		export filter {
			{{ range $v := .OSPF }}
			{{ range $f , $options := $v.Filternetworks }}
			if proto = "static_{{ $f }}" then
				accept;
			{{ end }}
			{{ end }}
		};
		{{ end }}
	};

  {{ range $v := .OSPF }}
	area {{ $v.Area }} {
		{{ $DONETWORK := len $v.Prefixes }}
		{{ if ne $DONETWORK 0 }}
		networks {
			{{ range $p, $options := $v.Prefixes }}
			{{ $p }};
			{{ end }}
		};
		{{ end }}
		{{ range $int, $options := $v.Interfaces }}
		interface "{{ $int }}" {
			{{ range $k, $v := $options }}
			{{ $k }} {{ $v }};
			{{ end }}
		};
		{{ end }}
	};
  {{ end }}
}
{{ end }}

{{ $DOBGP := len .BGP }}
{{ if ne $DOBGP 0 }}

{{ range $v := .BGP }}
protocol bgp {{ $v.ProcessName }} {
	local {{ $v.LocalIP }} as {{ $v.LocalAS }};
	neighbor {{ $v.NeighborIP }} as {{ $v.NeighborAS }};
	{{ if $v.RouteReflector }}
	rr client;
	{{ end }}

	ipv4 {
	  import all;
		{{ $EXPORT := len .ExportNetworks }}
		{{ if ne $EXPORT 0 }}
		export filter {
			if {{$v.GenerateFilter}} then
				accept;
			else reject;
		};
		{{ end }}
	};
}
{{ end }}
{{ end }}

`
