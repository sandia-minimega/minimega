// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"version"
)

var miscCLIHandlers = []minicli.Handler{
	{ // quit
		HelpShort: "quit minimega",
		HelpLong: `
Quit. An optional integer argument X allows deferring the quit call for X
seconds. This is useful for telling a mesh of minimega nodes to quit.

quit will not return a response to the cli, control socket, or meshage, it will
simply exit. meshage connected nodes catch this and will remove the quit node
from the mesh. External tools interfacing minimega must check for EOF on stdout
or the control socket as an indication that minimega has quit.`,
		Patterns: []string{
			"quit [delay]",
		},
		Call: wrapSimpleCLI(cliQuit),
	},
	{ // help
		HelpShort: "show command help",
		HelpLong: `
Show help on a command. If called with no arguments, show a summary of all
commands.`,
		Patterns: []string{
			"help [command]...",
		},
		Call: wrapSimpleCLI(cliHelp),
	},
	{ // read
		HelpShort: "read and execute a command file",
		HelpLong: `
Read a command file and execute it. This has the same behavior as if you typed
the file in manually.`,
		Patterns: []string{
			"read <file>",
		},
		Call: cliRead,
	},
	{ // debug
		HelpShort: "display internal debug information",
		Patterns: []string{
			"debug",
		},
		Call: wrapSimpleCLI(cliDebug),
	},
	{ // version
		HelpShort: "display the minimega version",
		Patterns: []string{
			"version",
		},
		Call: wrapSimpleCLI(cliVersion),
	},
	{ // echo
		HelpShort: "display text after macro expansion and comment removal",
		Patterns: []string{
			"echo [args]...",
		},
		Call: wrapSimpleCLI(cliEcho),
	},
}

func init() {
	registerHandlers("misc", miscCLIHandlers)
}

// generate a random ipv4 mac address and return as a string
func randomMac() string {
	b := make([]byte, 5)
	rand.Read(b)
	mac := fmt.Sprintf("00:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4])
	log.Info("generated mac: %v", mac)
	return mac
}

func generateUUID() string {
	log.Debugln("generateUUID")
	uuid, err := ioutil.ReadFile("/proc/sys/kernel/random/uuid")
	if err != nil {
		log.Error("generateUUID: %v", err)
		return "00000000-0000-0000-0000-000000000000"
	}
	uuid = uuid[:len(uuid)-1]
	log.Debug("generated UUID: %v", string(uuid))
	return string(uuid)
}

func isMac(mac string) bool {
	match, err := regexp.MatchString("^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$", mac)
	if err != nil {
		return false
	}
	return match
}

func hostid(s string) (string, int) {
	k := strings.Split(s, ":")
	if len(k) != 2 {
		log.Error("hostid cannot split host vmid pair: %v", k)
		return "", -1
	}
	val, err := strconv.Atoi(k[1])
	if err != nil {
		log.Error("parse hostid: %v", err)
		return "", -1
	}
	return k[0], val
}

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
// 	Example: a b "c d"
// 	will return: ["a", "b", "c d"]
func fieldsQuoteEscape(c string, input string) []string {
	log.Debug("fieldsQuoteEscape splitting on %v: %v", c, input)
	f := strings.Fields(input)
	var ret []string
	trace := false
	temp := ""

	for _, v := range f {
		if trace {
			if strings.Contains(v, c) {
				trace = false
				temp += " " + trimQuote(c, v)
				ret = append(ret, temp)
			} else {
				temp += " " + v
			}
		} else if strings.Contains(v, c) {
			temp = trimQuote(c, v)
			if strings.HasSuffix(v, c) {
				// special case, single word like 'foo'
				ret = append(ret, temp)
			} else {
				trace = true
			}
		} else {
			ret = append(ret, v)
		}
	}
	log.Debug("generated: %#v", ret)
	return ret
}

func trimQuote(c string, input string) string {
	if c == "" {
		log.Errorln("cannot trim empty space")
		return ""
	}
	var ret string
	for _, v := range input {
		if v != rune(c[0]) {
			ret += string(v)
		}
	}
	return ret
}

func unescapeString(input []string) string {
	var ret string
	for _, v := range input {
		containsWhite := false
		for _, x := range v {
			if unicode.IsSpace(x) {
				containsWhite = true
				break
			}
		}
		if containsWhite {
			ret += fmt.Sprintf(" \"%v\"", v)
		} else {
			ret += fmt.Sprintf(" %v", v)
		}
	}
	log.Debug("unescapeString generated: %v", ret)
	return ret
}

// cmdTimeout runs the command c and returns a timeout if it doesn't complete
// after time t. If a timeout occurs, cmdTimeout will kill the process.
func cmdTimeout(c *exec.Cmd, t time.Duration) error {
	log.Debug("cmdTimeout: %v", c)
	err := c.Start()
	if err != nil {
		return fmt.Errorf("cmd start: %v", err)
	}

	done := make(chan error)
	go func() {
		done <- c.Wait()
	}()

	select {
	case <-time.After(t):
		err = c.Process.Kill()
		if err != nil {
			return err
		}
		return <-done
	case err = <-done:
		return err
	}
}

// findRemoteVM attempts to find the VM ID of a VM by name or ID on a remote
// minimega node. It returns the ID of the VM on the remote host or an error,
// which may also be an error communicating with the remote node.
func findRemoteVM(host, vm string) (int, string, error) {
	log.Debug("findRemoteVM: %v %v", host, vm)

	// check for our own host
	hostname, _ := os.Hostname()
	if host == hostname {
		log.Debugln("host is local node")
		id := vms.findByName(vm)
		if id == VM_NOT_FOUND {
			// check for VM id
			id, err := strconv.Atoi(vm)
			if err != nil {
				return VM_NOT_FOUND, "", fmt.Errorf("vm not found")
			}
			if v, ok := vms.vms[id]; ok {
				log.Debug("got vm: %v %v %v", host, id, v.Name)
				return id, v.Name, nil
			}
		} else {
			log.Debug("got vm: %v %v %v", host, id, vm)
			return id, vm, nil
		}
	} else {
		// message the remote node for this info with:
		// 	vm_info name=<vm> [id]
		// if that doesn't work, then try:
		//	vm_info id=<vm> [id,name]
		// if that doesn't work, return not found
		log.Debugln("remote host")

		var cmdStr string
		v, err := strconv.Atoi(vm)
		if err == nil {
			cmdStr = fmt.Sprintf("vm info search id=%v mask name,id", v)
		} else {
			cmdStr = fmt.Sprintf("vm info search id=%v mask name,id", v)
		}

		cmd, err := minicli.CompileCommand(cmdStr)
		if err != nil {
			// Should never happen
			panic(err)
		}

		remoteRespChan := make(chan minicli.Responses)
		go meshageBroadcast(cmd, remoteRespChan)

		for resps := range remoteRespChan {
			// Find a response that is not an error
			for _, resp := range resps {
				if resp.Error == "" && len(resp.Tabular) > 0 {
					// Found it!
					row := resp.Tabular[0] // should be name,id
					name := row[0]
					id, err := strconv.Atoi(row[1])
					if err != nil {
						log.Debug("malformed response: %#v", resp)
					} else {
						return id, name, nil
					}
				}
			}
		}
	}

	return VM_NOT_FOUND, "", fmt.Errorf("vm not found")
}

// isClearCommand checks whether the command starts with "clear".
func isClearCommand(c *minicli.Command) bool {
	return strings.HasPrefix(c.Original, "clear")
}

// registerHandlers registers all the provided handlers with minicli, panicking
// if any of the handlers fail to register.
func registerHandlers(name string, handlers []minicli.Handler) {
	for i := range handlers {
		err := minicli.Register(&handlers[i])
		if err != nil {
			panic(fmt.Sprintf("invalid handler, %s:%d -- %v", name, i, err))
		}
	}
}

func wrapSimpleCLI(fn func(*minicli.Command) *minicli.Response) minicli.HandlerFunc {
	return func(c *minicli.Command, respChan chan minicli.Responses) {
		resp := fn(c)
		respChan <- minicli.Responses{resp}
	}
}

func cliQuit(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if v, ok := c.StringArgs["delay"]; ok {
		delay, err := strconv.Atoi(v)
		if err != nil {
			resp.Error = err.Error()
		} else {
			go func() {
				time.Sleep(time.Duration(delay) * time.Second)
				teardown()
			}()
			resp.Response = fmt.Sprintf("quitting after %v seconds", delay)
		}
	} else {
		teardown()
	}

	return resp
}

func cliHelp(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	input := ""
	if args, ok := c.ListArgs["command"]; ok {
		input = strings.Join(args, " ")
	}

	resp.Response = minicli.Help(input)
	return resp
}

func cliRead(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	file, err := os.Open(c.StringArgs["file"])
	if err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
		return
	}
	defer file.Close()

	count := 0

	r := bufio.NewReader(file)
	for ; ; count++ {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			break
		} else if err != nil {
			resp.Error = err.Error()
			break
		}
		command := string(line)
		log.Debug("read command: %v", command) // commands don't have their newlines removed

		// HAX: Make sure we don't have a recursive read command
		if strings.HasPrefix(command, "read") {
			resp.Error = "read cannot run other read commands"
			break
		}

		// TODO: Should we run a write command?

		cmd, err := minicli.CompileCommand(command)
		if err != nil {
			resp.Error = err.Error()
			break
		}

		for resp := range minicli.ProcessCommand(cmd, true) {
			respChan <- resp

			// Stop processing at the first error if there is one response.
			// TODO: What should we do if the command was mesh send and there
			// is a mixture of success and failure?
			if len(resp) == 1 && resp[0].Error != "" {
				break
			}
		}
	}

	// TODO: Should we send the response from read?
	resp.Response = fmt.Sprintf("read processed %d commands", count)
	respChan <- minicli.Responses{resp}
}

func cliDebug(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:   hostname,
		Header: []string{"Go version", "Goroutines", "CGO calls"},
		Tabular: [][]string{
			[]string{
				runtime.Version(),
				strconv.Itoa(runtime.NumGoroutine()),
				strconv.FormatInt(runtime.NumCgoCall(), 10),
			},
		},
	}
}

func cliVersion(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:     hostname,
		Response: fmt.Sprintf("minimega %v %v", version.Revision, version.Date),
	}
}

func cliEcho(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:     hostname,
		Response: strings.Join(c.ListArgs["args"], " "),
	}
}
