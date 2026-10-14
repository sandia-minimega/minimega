// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minipager

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type Pager interface {
	Page(output string)
}

var DefaultPager Pager = &defaultPager{}

type defaultPager struct{}

func (_ defaultPager) Page(output string) {
	if output == "" {
		return
	}

	_, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		log.Error("unable to determine terminal size: %v", err)
		fmt.Println(output)
		return
	}

	lines := strings.Count(output, "\n")

	if lines < 2*rows {
		fmt.Println(output)
		return
	}

	fmt.Printf("-- sending %v lines to $PAGER --\n", lines)

	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	cmd := exec.Command(pager)
	cmd.Stdin = strings.NewReader(output)
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Error("problem paging: %s", err)
	}
}

// IsTerminal reports whether stdout is a terminal.
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
