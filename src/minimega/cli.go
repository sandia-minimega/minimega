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
	"encoding/json"
	"flag"
	"fmt"
	"goreadline"
	"io"
	"minicli"
	log "minilog"
	"net"
	"os"
	"os/signal"
	"path"
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

type localResponse struct {
	Resp     minicli.Responses
	Rendered string
	More     bool // whether there are more responses coming
}

type MinimegaConn struct {
	url string

	conn net.Conn

	enc *json.Encoder
	dec *json.Decoder
}

var (
	// Prevents multiple commands from running at the same time
	cmdLock sync.Mutex
)

// Wrapper for minicli.ProcessCommand. Ensures that the command execution lock
// is acquired before running the command.
func runCommand(cmd *minicli.Command, record bool) chan minicli.Responses {
	cmdLock.Lock()
	defer cmdLock.Unlock()

	return minicli.ProcessCommand(cmd, record)
}

// Wrapper for minicli.ProcessCommand for commands that use meshage.
// Specifically, for `mesh send all ...`, runs the subcommand locally and
// across meshage, combining the results from the two channels into a single
// channel. This is useful if you want to get the output of a command from all
// nodes in the cluster without having to run a command locally and over
// meshage.
func runCommandGlobally(cmd *minicli.Command, record bool) chan minicli.Responses {
	cmdStr := fmt.Sprintf("mesh send %s %s", Wildcard, cmd.Original)
	cmd, err := minicli.CompileCommand(cmdStr)
	if err == nil {
		log.Fatal("cannot run `%v` globally -- %v", cmd.Original, err)
	}

	cmdLock.Lock()
	defer cmdLock.Unlock()

	var wg sync.WaitGroup

	out := make(chan minicli.Responses)

	// Run the command (should be `mesh send all ...` and the subcommand which
	// should run locally).
	ins := []chan minicli.Responses{
		minicli.ProcessCommand(cmd, record),
		minicli.ProcessCommand(cmd.Subcommand, record),
	}

	// De-mux ins into out
	for _, in := range ins {
		wg.Add(1)
		go func(in chan minicli.Responses) {
			defer wg.Done()
			for v := range in {
				out <- v
			}
		}(in)
	}

	// Wait until everything has been read before closing out
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
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
			log.Error("%v", err)
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

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}

func cliAttach() {
	// try to connect to the local minimega
	mm, err := DialMinimega()
	if err != nil {
		log.Fatalln(err)
	}

	// set up signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		if *f_panic {
			panic("teardown")
		}
		log.Debug("caught signal, disconnecting")
		goreadline.Rlcleanup()
		os.Exit(0)
	}()

	// start our own rlwrap
	fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
	fmt.Println("use 'disconnect' or ^d to exit just the minimega command line")
	fmt.Println()
	defer goreadline.Rlcleanup()

	var exitNext bool
	for {
		prompt := fmt.Sprintf("minimega:%v$ ", mm.url)
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			return
		}
		command := string(line)
		log.Debug("got from stdin: `%s`", line)

		// HAX: Shortcut some commands without using minicli
		if command == "disconnect" {
			log.Debugln("disconnecting")
			return
		} else if command == "quit" {
			if !exitNext {
				fmt.Println("CAUTION: calling 'quit' will cause the minimega daemon to exit")
				fmt.Println("If you really want to stop the minimega daemon, enter 'quit' again")
				exitNext = true
				continue
			}
		}

		exitNext = false

		cmd, err := minicli.CompileCommand(command)
		if err != nil {
			log.Error("%v", err)
			//fmt.Println("closest match: TODO")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		for resp := range mm.runCommand(cmd) {
			pageOutput(resp.Rendered)

			errs := resp.Resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}

		if command == "quit" {
			return
		}
	}
}

func localCommand() {
	a := flag.Args()

	log.Debugln("got args:", a)

	command := strings.Join(a, " ")

	// TODO: Need to escape?
	cmd, err := minicli.CompileCommand(command)
	if err != nil {
		log.Fatal(err.Error())
	}

	if cmd == nil {
		log.Debugln("cmd is nil")
		return
	}

	log.Infoln("got command:", cmd)

	mm, err := DialMinimega()
	if err != nil {
		log.Fatalln(err)
	}

	for resp := range mm.runCommand(cmd) {
		if resp.Rendered != "" {
			fmt.Println(resp.Rendered)
		}

		errs := resp.Resp.Error()
		if errs != "" {
			fmt.Fprintln(os.Stderr, errs)
		}
	}
}

func DialMinimega() (*MinimegaConn, error) {
	var mm = &MinimegaConn{
		url: path.Join(*f_base, "minimega"),
	}
	var err error

	// try to connect to the local minimega
	mm.conn, err = net.Dial("unix", mm.url)
	if err != nil {
		return nil, err
	}

	mm.enc = json.NewEncoder(mm.conn)
	mm.dec = json.NewDecoder(mm.conn)

	return mm, nil
}

// runCommand runs a command through a JSON pipe.
func (mm *MinimegaConn) runCommand(cmd *minicli.Command) chan *localResponse {
	err := mm.enc.Encode(*cmd)
	if err != nil {
		log.Errorln("local command gob encode: %v", err)
		return nil
	}
	log.Debugln("encoded command:", cmd)

	respChan := make(chan *localResponse)

	go func() {
		defer close(respChan)

		for {
			var r localResponse
			err = mm.dec.Decode(&r)
			if err != nil {
				if err == io.EOF {
					log.Infoln("server disconnected")
					return
				}

				log.Errorln("local command gob decode: %v", err)
				return
			}

			respChan <- &r
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}()

	return respChan
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
