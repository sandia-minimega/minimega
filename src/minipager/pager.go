// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minipager

import (
	"fmt"
	log "minilog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

type Pager interface {
	Page(output string)
}

// Copy of winsize struct defined by iotctl.h
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

var DefaultPager Pager = &defaultPager{}

type defaultPager struct{}

func (_ defaultPager) Page(output string) {
	if output == "" {
		return
	}

	size := termSize()
	if size == nil {
		fmt.Println(output)
		return
	}

	lines := strings.Count(output, "\n")

	if lines < 2*int(size.Row) {
		fmt.Println(output)
		return
	}

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

func termSize() *winsize {
	ws := &winsize{}
	res, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(res) == -1 {
		log.Error("unable to determine terminal size (errno: %d)", errno)
		return nil
	}

	return ws
}
