package main

import (
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"strconv"
	"text/template"
)

const (
	BIRD_CONFIG = "/etc/bird.conf"
)

type Bird struct {
	Static map[string]string
	OSPF   map[string]*OSPF
}

var (
	birdData *Bird
	birdCmd  *exec.Cmd
)

type OSPF struct {
	Area       string
	Interfaces map[string]bool // bool placeholder for later options
}

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"bird <flush,>",
			"bird <commit,>",
			"bird <static,> <network> <nh>",
			"bird <ospf,> <area> <network>",
		},
		Call: handleBird,
	})
	birdData = &Bird{
		Static: make(map[string]string),
		OSPF:   make(map[string]*OSPF),
	}
}

func handleBird(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {
		birdData = &Bird{
			Static: make(map[string]string),
			OSPF:   make(map[string]*OSPF),
		}
	} else if c.BoolArgs["commit"] {
		birdConfig()
		birdRestart()
	} else if c.BoolArgs["static"] {
		network := c.StringArgs["network"]
		nh := c.StringArgs["nh"]
		birdData.Static[network] = nh
	} else if c.BoolArgs["ospf"] {
		area := c.StringArgs["area"]
		network := c.StringArgs["network"]

		// get an interface from the index
		idx, err := strconv.Atoi(network)
		if err != nil {
			log.Errorln(err)
			return
		}

		iface, err := findEth(idx)
		if err != nil {
			log.Errorln(err)
			return
		}

		o := OSPFFindOrCreate(area)
		o.Interfaces[iface] = true
	}
}

func birdConfig() {
	t, err := template.New("bird").Parse(birdTmpl)
	if err != nil {
		log.Errorln(err)
		return
	}

	f, err := os.Create(BIRD_CONFIG)
	if err != nil {
		log.Errorln(err)
		return
	}

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

func OSPFFindOrCreate(area string) *OSPF {
	if o, ok := birdData.OSPF[area]; ok {
		return o
	}
	o := &OSPF{
		Area:       area,
		Interfaces: make(map[string]bool),
	}
	birdData.OSPF[area] = o
	return o
}

var birdTmpl = `
# minirouter bird template

protocol kernel {
        scan time 60;
        import none;
        export all;   # Actually insert routes into the kernel routing table
}

# The Device protocol is not a real routing protocol. It doesn't generate any
# routes and it only serves as a module for getting information about network
# interfaces from the kernel.
protocol device {
        scan time 60;
}

{{ $DOSTATIC := len .Static }}
{{ if ne $DOSTATIC 0 }}
protocol static {
	check link;
{{ range $network, $nh := .Static }}
	route {{ $network }} via {{ $nh }};
{{ end }}
}
{{ end }}

{{ $DOOSPF := len .OSPF }}
{{ if ne $DOOSPF 0 }}
protocol ospf {
{{ range $v := .OSPF }} 
	area {{ $v.Area }} {
		{{ range $int, $options := $v.Interfaces }}
		interface "{{ $int }}";
		{{ end }}
	};
{{ end }}
}
{{ end }}
`
