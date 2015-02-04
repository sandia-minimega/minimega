// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// command line interface for minimega
//
// The command line interface wraps a number of commands listed in the
// cliCommands map. Each entry to the map defines a function that is called
// when the command is invoked on the command line, as well as short and long
// form help. The record parameter instructs the cli to put the command in the
// command history.
//
// The cli uses the readline library for command history and tab completion.
// A separate command history is kept and used for writing the buffer out to
// disk.

package main

import (
	"bufio"
	"fmt"
	"goreadline"
	"minicli"
	log "minilog"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	COMMAND_TIMEOUT = 10
)

// Copy of winsize struct defined by iotctl.h
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

var (
	// Prevents multiple commands from running at the same time
	cmdLock sync.Mutex
)

// Wrapper for minicli.ProcessString. Ensures that the command execution lock
// is acquired before running the command.
func runCommand(cmd *minicli.Command, record bool) chan minicli.Responses {
	cmdLock.Lock()
	defer cmdLock.Unlock()

	return minicli.ProcessCommand(cmd, record)
}

// local command line interface, wrapping readline
func cliLocal() {
	prompt := "minimega$ "

	for {
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			break // EOF
		}
		command := string(line)
		log.Debug("got from stdin:", command)

		cmd, err := minicli.CompileCommand(command)
		if err != nil {
			fmt.Printf("invalid command: `%s`\n", command)
			//fmt.Printf("closest match: TODO\n")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		// HAX: Don't record the read command
		record := !strings.HasPrefix(command, "read")

		for resp := range runCommand(cmd, record) {
			// print the responses
			pageOutput(resp.String())
		}
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

func pageOutput(output string) {
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

// sort and walk the api, emitting markdown for each entry
func docGen() {
	var keys []string
	// TODO
	//for k, _ := range cliCommands {
	//	keys = append(keys, k)
	//}
	sort.Strings(keys)

	fmt.Println("# minimega API")

	for _, k := range keys {
		fmt.Printf("<h2 id=%v>%v</h2>\n", k, k)
		//fmt.Println(cliCommands[k].Helplong)
	}
}

var poeticDeath = `
Willst du immer weiterschweifen?
Sieh, das Gute liegt so nah.
Lerne nur das Glück ergreifen,
denn das Glück ist immer da.
`
