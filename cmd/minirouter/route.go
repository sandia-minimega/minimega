package main

import (
	"os/exec"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"route <add,> default gw <gw>",
			"route <del,> default",
		},
		Call: handleRoute,
	})
}

func handleRoute(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["add"] {
		gw := c.StringArgs["gw"]
		log.Debug("setting default gw: %v", gw)

		out, err := exec.Command("route", "add", "default", "gw", gw).CombinedOutput()
		if err != nil {
			log.Error("unable to set default route: %v: %v", err, string(out))
		}
	} else if c.BoolArgs["del"] {
		log.Debug("deleting default route")

		out, err := exec.Command("route", "del", "default").CombinedOutput()
		// supress error if we tried to delete a default route when there
		// wasn't one
		if err != nil && !strings.Contains(string(out), "No such process") {
			log.Error("unable to delete default route: %v: %v", err, string(out))
		}
	}
}
