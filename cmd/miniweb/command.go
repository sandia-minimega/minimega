// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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

// unknownNamespaceHeader marks responses for namespaces that don't exist so
// that the web UI can tell them apart from other errors.
const unknownNamespaceHeader = "X-Minimega-Unknown-Namespace"

// listNamespacesFunc is a test seam for the namespace lookup.
var listNamespacesFunc = listNamespaces

// listNamespaces returns the namespaces that minimega lists. It returns an
// error when minimega does not answer, so that an outage is not mistaken for
// an empty listing.
func listNamespaces() ([]string, error) {
	var (
		names    []string
		errs     []error
		answered bool
	)

	// read every response: the client blocks until the channel is drained
	for resps := range run(&Command{Command: "namespace"}) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				errs = append(errs, errors.New(resp.Error))
				continue
			}

			answered = true
			for _, row := range resp.Tabular {
				names = append(names, tabularToMap(resp, row)["namespace"])
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if !answered {
		return nil, errors.New("no response from minimega")
	}

	return names, nil
}

// requireNamespace responds with 404 to requests for a namespace that doesn't
// exist. Otherwise, the "namespace <name> ..." commands that miniweb runs for
// the request (e.g. from a page still polling a cleared namespace) would
// silently recreate it.
func requireNamespace(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// a namespace from the -namespace flag is ours to recreate
		if *f_namespace == "" {
			name := r.URL.Query().Get("namespace")
			if name != "" {
				names, err := listNamespacesFunc()
				if err != nil {
					log.Error("checking namespace %v: %v", name, err)
					http.Error(w, fmt.Sprintf("cannot check namespace %v: %v", name, err), http.StatusBadGateway)
					return
				}

				if !slices.Contains(names, name) {
					w.Header().Set(unknownNamespaceHeader, "true")
					http.Error(w, fmt.Sprintf("unknown namespace: %v", name), http.StatusNotFound)
					return
				}
			}
		}

		f(w, r)
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
