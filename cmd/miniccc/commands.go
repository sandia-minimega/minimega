// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func processCommand(cmd *ron.Command) {
	log.Debug("processCommand %v", cmd)

	resp := &ron.Response{
		ID: cmd.ID,
	}

	// get any files needed for the command
	if len(cmd.FilesSend) != 0 {
		recvFiles(cmd.FilesSend)
	}

	// kill processes before starting new ones
	if cmd.PID != 0 {
		kill(cmd.PID)
	}
	if cmd.KillAll != "" {
		killAll(cmd.KillAll)
	}

	// adjust the log level, if a new level is provided
	if cmd.Level != nil {
		log.Info("setting level to: %v", *cmd.Level)
		log.SetLevelAll(*cmd.Level)
	}

	if len(cmd.Command) != 0 {
		resp.Stdout, resp.Stderr, resp.ExitCode = runCommand(cmd)
		// don't record exit code if this is a background command
		resp.RecordExitCode = !cmd.Background
	}

	if cmd.ConnTest != nil {
		resp.Stdout, resp.Stderr = testConnect(cmd.ConnTest)
	}

	if len(cmd.FilesRecv) != 0 {
		sendFiles(cmd.ID, cmd.FilesRecv)
	}

	appendResponse(resp)
}

// lookPath wraps exec.LookPath to check $PATH and the files path. A non-nil
// env provides the $PATH in place of the client's own environment.
func lookPath(file string, env []string) (string, error) {
	path, err := lookPathEnv(file, env)
	if err == nil {
		return path, nil
	}

	// file contains a slash, shouldn't search files path
	if strings.Contains(file, "/") {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}

	file = filepath.Join(*f_path, "files", file)
	return exec.LookPath(file)
}

// outputWait is how long miniccc waits for the end of a command's output to a
// pipe once the command has exited. The output only ends later than the
// command when a process that the command started still has it open.
var outputWait = 5 * time.Second

// commandOutput is the output of a command that goes to named pipes.
type commandOutput struct {
	child   []*os.File     // the command's ends, which miniccc closes once the command has started
	parent  []*os.File     // the ends that miniccc reads
	readers sync.WaitGroup // the goroutines that read them
	stop    chan struct{}  // closed to stop the readers
	wait    time.Duration  // outputWait for this command
}

// pipe returns a file to use as the command's stdout or stderr. The lines
// that the command writes to it go to the named pipe.
func (o *commandOutput) pipe(name string) (*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	out, err := NewPlumberWriter(name)
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}

	o.child = append(o.child, w)
	o.parent = append(o.parent, r)

	o.readers.Add(1)
	go func() {
		defer o.readers.Done()
		defer close(out)

		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			select {
			case out <- scanner.Text() + "\n":
			case <-o.stop:
				return
			}
		}

		// the read fails when finish closed output that did not end in time
		if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
			log.Errorln(err)
		}
	}()

	return w, nil
}

// started closes the command's ends of the output, so that the output ends
// when the last process that writes it has exited. Call it when the command
// has started.
func (o *commandOutput) started() {
	for _, f := range o.child {
		f.Close()
	}
	o.child = nil
}

// finish waits for the end of the output, but no longer than o.wait, and then
// closes it. Call it when the command has exited or could not be
// started.
func (o *commandOutput) finish() {
	o.started()

	ended := make(chan struct{})
	go func() {
		o.readers.Wait()
		close(ended)
	}()

	select {
	case <-ended:
	case <-time.After(o.wait):
		log.Debug("output is still open %v after the command exited, closing it", o.wait)
	}

	close(o.stop)
	for _, f := range o.parent {
		f.Close()
	}
	<-ended
}

func runCommand(c *ron.Command) (string, string, int) {
	var bufout, buferr bytes.Buffer

	cmd := &exec.Cmd{
		Args: c.Command,
	}

	// done is closed when the command has exited, or could not be started
	done := make(chan struct{})

	// output is the command's output that goes to pipes
	output := &commandOutput{stop: make(chan struct{}), wait: outputWait}

	// finish gives up the command's pipes
	finish := func() {
		close(done)
		output.finish()
	}

	// a command in the background runs on when this function returns, and
	// gives up its pipes when it exits
	background := false
	defer func() {
		if !background {
			finish()
		}
	}()

	if c.Session != "" {
		release, err := setSession(cmd, c.Session)
		if err != nil {
			log.Errorln(err)
			return "", err.Error(), -1
		}
		defer release()
	}

	path, err := lookPath(c.Command[0], cmd.Env)
	if err != nil {
		log.Errorln(err)
		return "", err.Error(), -1
	}
	cmd.Path = path

	if c.Stdin != "" {
		pStdin, err := cmd.StdinPipe()
		if err != nil {
			log.Errorln(err)
			return "", "", -1
		}

		cStdin, err := NewPlumberReader(c.Stdin)
		if err != nil {
			log.Errorln(err)
			return "", "", -1
		}

		go func() {
			<-done
			cStdin.Close()
		}()

		go func() {
			// give up the pipe when the command no longer reads its input
			defer cStdin.Close()

			for v := range cStdin.C {
				_, err := pStdin.Write([]byte(v))
				if errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed) {
					// not an error: the command closed its input or exited
					log.Debug("%v no longer reads its input: %v", c.Command, err)
					return
				} else if err != nil {
					log.Errorln(err)
					return
				}
			}
			pStdin.Close()
		}()
	}

	if c.Stdout != "" {
		f, err := output.pipe(c.Stdout)
		if err != nil {
			log.Errorln(err)
			return "", "", -1
		}
		cmd.Stdout = f
	} else {
		cmd.Stdout = &bufout
	}

	if c.Stderr != "" {
		f, err := output.pipe(c.Stderr)
		if err != nil {
			log.Errorln(err)
			return "", "", -1
		}
		cmd.Stderr = f
	} else {
		cmd.Stderr = &buferr
	}

	log.Info("executing: %v", c.Command)

	if c.Background {
		log.Debug("starting in background")
		if err := cmd.Start(); err != nil {
			log.Errorln(err)
			return "", buferr.String(), -1
		}
		output.started()
		background = true

		pid := cmd.Process.Pid

		client.Lock()
		defer client.Unlock()
		client.Processes[pid] = &Process{
			PID:     pid,
			Command: c.Command,
			process: cmd.Process,
		}

		go func() {
			cmd.Wait()

			log.Debug("command exited: %v", c.Command)
			if bufout.Len() > 0 {
				log.Debug("%s", bufout.String())
			}
			if buferr.Len() > 0 {
				log.Debug("%s", buferr.String())
			}

			client.Lock()
			delete(client.Processes, pid)
			client.Unlock()

			finish()
		}()

		return "", "", 0
	}

	// To avoid returning a false positive, default to -1 in case error below
	// isn't an exec.ExitError.
	exitCode := -1

	err = cmd.Start()
	if err == nil {
		output.started()
		err = cmd.Wait()
	}

	if err == nil {
		exitCode = 0
	} else {
		log.Errorln(err)

		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// the deferred finish waits for the end of output that goes to pipes
	return bufout.String(), buferr.String(), exitCode
}

func kill(pid int) {
	client.Lock()
	defer client.Unlock()

	if pid == -1 {
		// Wildcard
		log.Info("killing all processes")
		for _, p := range client.Processes {
			if err := p.process.Kill(); err != nil {
				log.Errorln(err)
			}
		}

		return
	}

	log.Info("killing PID %v", pid)
	if p, ok := client.Processes[pid]; ok {
		if err := p.process.Kill(); err != nil {
			log.Errorln(err)
		}

		return
	}

	log.Error("no such process: %v", pid)
}

func killAll(needle string) {
	client.Lock()
	defer client.Unlock()

	log.Info("killing all processes matching `%v`", needle)

	for _, p := range client.Processes {
		if strings.Contains(strings.Join(p.Command, " "), needle) {
			log.Info("killing matched process: %v", p.Command)
			if err := p.process.Kill(); err != nil {
				log.Errorln(err)
			}
		}
	}
}

func testConnect(test *ron.ConnTest) (string, string) {
	log.Debug("testConnect called with %v", *test)

	uri, err := url.Parse(test.Endpoint)
	if err != nil {
		return "", fmt.Sprintf("unable to parse test URI %s: %v", test.Endpoint, err)
	}

	timeout := time.After(test.Wait)

	for {
		select {
		case <-timeout:
			return fmt.Sprintf("%s | fail", uri.Host), ""
		default:
			if conn, err := net.DialTimeout(uri.Scheme, uri.Host, 500*time.Millisecond); err == nil {
				defer conn.Close()

				if uri.Scheme == "udp" {
					if err := conn.SetDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
						return fmt.Sprintf("%s | fail", uri.Host), ""
					}

					if len(test.Packet) > 0 {
						if _, err := conn.Write(test.Packet); err != nil {
							return fmt.Sprintf("%s | fail", uri.Host), ""
						}
					}

					buf := make([]byte, 4096)

					if _, err := conn.Read(buf); err != nil {
						return fmt.Sprintf("%s | fail", uri.Host), ""
					}
				}

				return fmt.Sprintf("%s | pass", uri.Host), ""
			}
		}
	}
}
