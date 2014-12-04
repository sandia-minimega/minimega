// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

var HelpLongQuit = `
	Usage: quit [delay]

Quit. An optional integer argument X allows deferring the quit call for X
seconds. This is useful for telling a mesh of minimega nodes to quit.

quit will not return a response to the cli, control socket, or meshage, it will
simply exit. meshage connected nodes catch this and will remove the quit node
from the mesh. External tools interfacing minimega must check for EOF on stdout
or the control socket as an indication that minimega has quit.`

func init() {
	minicli.Register(&minicli.Handler{
		Patterns:  []string{"quit [delay]"},
		HelpShort: "quit",
		HelpLong:  HelpLongQuit,
		Call:      cliQuit,
	})
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

func cliDebug(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		// create output
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "Go Version:\t%v\n", runtime.Version)
		fmt.Fprintf(w, "Goroutines:\t%v\n", runtime.NumGoroutine())
		fmt.Fprintf(w, "CGO calls:\t%v\n", runtime.NumCgoCall())
		w.Flush()

		return cliResponse{
			Response: o.String(),
		}
	}

	switch strings.ToLower(c.Args[0]) {
	case "panic":
		panicOnQuit = true
		host, err := os.Hostname()
		if err != nil {
			log.Errorln(err)
			teardown()
		}
		return cliResponse{
			Response: fmt.Sprintf("%v wonders what you're up to...", host),
		}
	case "numcpus":
		if len(c.Args) == 1 {
			return cliResponse{
				Response: fmt.Sprintf("%v", runtime.GOMAXPROCS(0)),
			}
		}
		cpus, err := strconv.Atoi(c.Args[1])
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("numcpus: %v", err),
			}
		}
		runtime.GOMAXPROCS(cpus)
		return cliResponse{}
	default:
		return cliResponse{
			Error: "usage: debug [panic]",
		}
	}
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
		//	vm_info id=<vm> [name]
		// if that doesn't work, return not found
		log.Debugln("remote host")

		cmd := cliCommand{
			Args: []string{host, "vm_info", "output=quiet", fmt.Sprintf("name=%v", vm), "[id]"},
		}
		r := meshageSet(cmd)
		if r.Error != "" {
			e := strings.TrimSpace(r.Error)
			return VM_NOT_FOUND, "", fmt.Errorf(e)
		}
		d := strings.TrimSpace(r.Response)

		log.Debug("got response %v", d)

		v, err := strconv.Atoi(d)
		if err == nil {
			log.Debug("got vm: %v %v %v", host, v, vm)
			return v, vm, nil
		}

		// nope, try the vm id instead
		cmd = cliCommand{
			Args: []string{host, "vm_info", "output=quiet", fmt.Sprintf("id=%v", vm), "[name]"},
		}
		r = meshageSet(cmd)
		if r.Error != "" {
			e := strings.TrimSpace(r.Error)
			return VM_NOT_FOUND, "", fmt.Errorf(e)
		}
		d = strings.TrimSpace(r.Response)

		log.Debug("got response %v", d)

		d = strings.TrimSpace(d)
		if d != "" {
			log.Debug("got vm: %v %v %v", host, v, d)
			return v, d, nil
		}
	}
	return VM_NOT_FOUND, "", fmt.Errorf("vm not found")
}

func cliQuit(c *minicli.Command) minicli.Responses {
	log.Debugln("cliQuit")

	r := &minicli.Response{}

	if v, ok := c.StringArgs["delay"]; ok {
		delay, err := strconv.Atoi(v)
		if err != nil {
			r.Error = err.Error()
		} else {
			go func() {
				time.Sleep(time.Duration(delay) * time.Second)
				teardown()
			}()
			r.Response = fmt.Sprintf("quitting after %v seconds", delay)
		}
	} else {
		teardown()
	}
	return minicli.Responses{r}
}
