// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

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
