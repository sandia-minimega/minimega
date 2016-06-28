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

var (
	birdCmd *exec.Cmd
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"bird <flush,>",
			"bird <commit,>",
		},
		Call: handleBird,
	})
}

func handleBird(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {

	} else if c.BoolArgs["commit"] {
		birdConfig()
		birdRestart()
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

	err = t.Execute(f, nil)
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

	birdCmd = exec.Command("bird", "-f")
	err := birdCmd.Start()
	if err != nil {
		log.Errorln(err)
		birdCmd = nil
	}
}

var birdTmpl = `
# minirouter bird template
`
