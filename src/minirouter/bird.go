package main

import (
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"text/template"
)

const (
	BIRD_CONFIG = "/etc/bird.conf"
)

type Bird struct {
	Static map[string]string
}

var (
	birdData *Bird
	birdCmd  *exec.Cmd
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"bird <flush,>",
			"bird <commit,>",
			"bird <static,> <network> <nh>",
		},
		Call: handleBird,
	})
	birdData = &Bird{
		Static: make(map[string]string),
	}
}

func handleBird(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {
		birdData = &Bird{
			Static: make(map[string]string),
		}
	} else if c.BoolArgs["commit"] {
		birdConfig()
		birdRestart()
	} else if c.BoolArgs["static"] {
		network := c.StringArgs["network"]
		nh := c.StringArgs["nh"]
		birdData.Static[network] = nh
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

	birdCmd = exec.Command("bird", "-f", "-s", "/bird.sock", "-P", "/bird.pid")
	err := birdCmd.Start()
	if err != nil {
		log.Errorln(err)
		birdCmd = nil
	}
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

protocol static {
	check link;
{{ range $network, $nh := .Static }}
	route {{ $network }} via {{ $nh }};
{{ end }}
}
`
