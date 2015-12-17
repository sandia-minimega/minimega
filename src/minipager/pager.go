// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minipager

import (
	"bufio"
	"fmt"
	"goreadline"
	log "minilog"
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

	log.Debug("term height: %d", size.Row)

	prompt := "-- press [ENTER] to show more, EOF to discard --"

	scanner := bufio.NewScanner(strings.NewReader(output))
outer:
	for {
		for i := uint16(0); i < size.Row-1; i++ {
			if scanner.Scan() {
				fmt.Println(scanner.Text()) // Println will add back the final '\n'
			} else {
				break outer // finished consuming from scanner
			}
		}

		_, err := goreadline.Rlwrap(prompt, false)
		if err != nil {
			fmt.Println()
			break outer // EOF
		}
	}

	if err := scanner.Err(); err != nil {
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
