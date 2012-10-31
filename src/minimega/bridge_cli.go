package main

// routines for interfacing bridge mechanisms with the cli
import (
	"errors"
)


func host_tap_create(c cli_command) cli_response {
	if len(c.Args) != 1 {
		return cli_response{
			Error: errors.New("host_tap takes one argument"),
		}
	}
	lan_err, ok := current_bridge.Lan_create(c.Args[0])
	if !ok {
		return cli_response{
			Error: lan_err,
		}
	}

	// create the tap
	tap, err := current_bridge.Tap_create(c.Args[0], true)
	if err != nil {
		return cli_response{
			Error: err,
		}
	}
	return cli_response{
		Response: tap,
	}
}
