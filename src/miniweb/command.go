// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"net/http"
	"strings"
)

type Command struct {
	Command   string   `json:"command"`
	Columns   []string `json:"columns"`
	Filters   []string `json:"filters"`
	Namespace string   `json:"-"`
}

// NewCommand creates a command with the correct namespace.
func NewCommand(r *http.Request) *Command {
	namespace := *f_namespace
	if namespace == "" {
		namespace = r.URL.Query().Get("namespace")
	}

	return &Command{
		Namespace: namespace,
	}
}

func (c *Command) String() string {
	cmd := c.Command

	// apply filters first so we don't need to worry about the columns not
	// including the filtered fields.
	for _, f := range c.Filters {
		cmd = fmt.Sprintf(".filter %v %v", f, cmd)
	}

	if len(c.Columns) > 0 {
		columns := make([]string, len(c.Columns))

		// quote all the columns in case there are spaces
		for i := range c.Columns {
			columns[i] = fmt.Sprintf("%q", c.Columns[i])
		}

		cmd = fmt.Sprintf(".columns %v %v", strings.Join(columns, ","), cmd)
	}

	// if there's a namespace, use it
	if c.Namespace != "" {
		cmd = fmt.Sprintf("namespace %q %v", c.Namespace, cmd)
	}

	// don't record command in history
	cmd = fmt.Sprintf(".record false %v", cmd)

	log.Debug("built command: `%v`", cmd)
	return cmd
}
