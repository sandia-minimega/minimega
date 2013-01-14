package main

import (
	"strconv"
)

// routines for interfacing bridge mechanisms with the cli
func host_tap_create(c cli_command) cli_response {
	if len(c.Args) != 1 {
		return cli_response{
			Error: "host_tap takes one argument",
		}
	}
	r, err := strconv.Atoi(c.Args[0])
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}
	lan_err, ok := current_bridge.Lan_create(r)
	if !ok {
		return cli_response{
			Error: lan_err.Error(),
		}
	}

	// create the tap
	tap, err := current_bridge.Tap_create(r, true)
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}
	return cli_response{
		Response: tap,
	}
}
