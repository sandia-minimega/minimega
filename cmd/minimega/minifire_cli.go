// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

const minifireBanner = `      (
     ) )
    ( ( (
     ) ) )
   .-'-'-.
   |_____|
minifire: Firecracker microVMs for minimega`

var minifireCLIHandlers = []minicli.Handler{
	{ // minifire
		HelpShort: "show the minifire banner",
		Patterns: []string{
			"minifire",
		},
		Hidden: true,
		Call: wrapSimpleCLI(func(_ *Namespace, _ *minicli.Command, resp *minicli.Response) error {
			resp.Response = minifireBanner
			return nil
		}),
	},
}
