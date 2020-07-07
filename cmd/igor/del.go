// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

var cmdDel = &Command{
	UsageLine: "del <reservation name>",
	Short:     "delete reservation",
	Long: `
Delete an existing reservation.
	`,
}

func init() {
	// break init cycle
	cmdDel.Run = runDel
}

// Remove the specified reservation.
func runDel(cmd *Command, args []string) {
	// reservation name should be the only argument
	if len(args) != 1 {
		log.Fatalln("Invalid arguments")
	}

	name := args[0]

	r := igor.Find(name)
	if r == nil {
		log.Fatal("reservation does not exist: %v", name)
	}

	if !r.IsWritable(igor.User) {
		log.Fatal("insufficient privileges to delete reservation: %v", name)
	}

	if err := igor.Delete(r.ID); err != nil {
		log.Fatalln(err)
	}
}
