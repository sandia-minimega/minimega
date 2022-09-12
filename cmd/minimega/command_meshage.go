// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"sync"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// SourceMeshage is used to prevent some commands running via meshage such as
// `read` and `mesh send`.
const SourceMeshage = "meshage"

var meshageCommandLock sync.Mutex

func meshageHandler() {
	for {
		m := <-meshageCommandChan
		go func() {
			mCmd := m.Body.(meshageCommand)

			cmd, err := minicli.Compile(mCmd.Original)
			if err != nil {
				log.Error("invalid command from mesh: `%s`", mCmd.Original)
				return
			}

			// Copy the flags at each level of nested command
			for c, c2 := cmd, &mCmd.Command; c != nil && c2 != nil; {
				c.Record = c2.Record
				c.Source = c2.Source
				c.Preprocess = c2.Preprocess
				c, c2 = c.Subcommand, c2.Subcommand
			}

			resps := []minicli.Responses{}
			for resp := range runCommands(cmd) {
				resps = append(resps, resp)
			}

			if len(resps) > 1 || len(resps[0]) > 1 {
				// This should never happen because the only commands that
				// return multiple responses are `read` and `mesh send` which
				// aren't supposed to be sent across meshage.
				log.Error("unsure how to process multiple responses!!")
			}

			resp := meshageResponse{Response: *resps[0][0], TID: mCmd.TID}
			recipient := []string{m.Source}

			_, err = meshageNode.Set(recipient, resp)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}
