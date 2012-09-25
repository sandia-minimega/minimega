// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// routine to clean up the minimega state after a bad crash
package main

import (
	"errors"
	log "minilog"
	"os"
	"os/exec"
)

// clean up after an especially bad crash, hopefully we don't have to call
// this one much :)
// currently this will:
// 	kill all qemu instances
//	kill all taps
//	remove everything inside of info.Base_path (careful, that's dangerous)
// TODO: clean up from pid and tap files
func nuke(c cli_command) cli_response { // the cli_response return is just so we can fit in the cli model
	if len(c.Args) != 0 {
		return cli_response{
			Error: errors.New("nuke does not take any arguments"),
		}
	}
	// kill all qemu
	qemu := process("qemu")
	log.Info("killing all instances of: %v", qemu)
	cmd := exec.Command("killall", qemu)
	err := cmd.Start()
	if err != nil {
		return cli_response{
			Error: err,
		}
	}
	err = cmd.Wait()

	// clean up the base path
	log.Info("cleaning up base path: %v", *f_base)
	err = os.RemoveAll(*f_base)
	if err != nil {
		return cli_response{
			Error: err,
		}
	}
	teardown()
	return cli_response{}
}
