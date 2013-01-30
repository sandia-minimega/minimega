// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
//

// minimega dhcp server support
package main

// TODO: add support for killing dhcp servers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
)

type dhcpServer struct {
	Addr     string
	MinRange string
	MaxRange string
	Path     string
}

var (
	dhcpServers     map[int]*dhcpServer
	dhcpServerCount int
)

func init() {
	dhcpServers = make(map[int]*dhcpServer)
}

// generate paths for the leases and pid files (should be unique) so we can support multiple dhcp servers
// maintain a map of dhcp servers that can be listed
// allow killing dhcp servers with dhcp kill 

func dhcpCLI(c cli_command) cli_response {
	var ret cli_response
	switch len(c.Args) {
	case 0:
		// show the list of dhcp servers
		ret.Response = dhcpList()
	case 2:
		if c.Args[0] != "kill" {
			ret.Error = "malformed command"
			break
		}
		val, err := strconv.Atoi(c.Args[1])
		if err != nil {
			ret.Error = err.Error()
			break
		}
		err = dhcpKill(val)
		if err != nil {
			ret.Error = err.Error()
		}
	case 4:
		if c.Args[0] != "start" {
			ret.Error = "malformed command"
			break
		}
		err := dhcpStart(c.Args[1], c.Args[2], c.Args[3])
		if err != nil {
			ret.Error = err.Error()
		}
	default:
		ret.Error = "malformed command"
	}
	return ret
}

func dhcpList() string {
	w := new(tabwriter.Writer)
	buf := new(bytes.Buffer)
	w.Init(buf, 0, 8, 1, ' ', 0)
	fmt.Fprintf(w, "ID\t:\tListening Address\tMin\tMax\tPath\tPID\n")
	for id, c := range dhcpServers {
		pid := dhcpPID(id)
		fmt.Fprintf(w, "%v\t:\t%v\t%v\t%v\t%v\t%v\n", id, c.Addr, c.MinRange, c.MaxRange, c.Path, pid)
	}
	w.Flush()
	return buf.String()
}

func dhcpKill(id int) error {
	if id == -1 {
		var e string
		for c, _ := range dhcpServers {
			err := dhcpKill(c)
			if err != nil {
				e = fmt.Sprintf("%v\n%v", e, err)
			}
		}
		if e == "" {
			return nil
		} else {
			return fmt.Errorf("%v", e)
		}
	}

	pid := dhcpPID(id)
	log.Debug("dhcp id %v has pid %v", id, pid)
	if pid == -1 {
		return fmt.Errorf("invalid id")
	}

	var s_out bytes.Buffer
	var s_err bytes.Buffer
	p := process("kill")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			fmt.Sprintf("%v", pid),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Infoln("killing dhcp server:", pid)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func dhcpStart(ip, min, max string) error {
	path, err := dhcpPath()
	if err != nil {
		return err
	}

	d := &dhcpServer{
		Addr:     ip,
		MinRange: min,
		MaxRange: max,
		Path:     path,
	}

	p := process("dnsmasq")
	var s_out bytes.Buffer
	var s_err bytes.Buffer
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"--bind-interfaces",
			fmt.Sprintf("--pid-file=%v/dnsmasq.pid", d.Path),
			"--except-interface",
			"lo",
			"--listen-address",
			ip,
			"--dhcp-range",
			fmt.Sprintf("%v,%v", min, max),
			fmt.Sprintf("--dhcp-leasefile=%v/dnsmasq.leases", d.Path),
			"--dhcp-lease-max=4294967295",
			"-o",
			"-k",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Debug("starting dhcp server with command: %v", cmd)
	err = cmd.Start()
	if err != nil {
		return err
	}

	id := dhcpServerCount
	dhcpServerCount++
	dhcpServers[id] = d

	// wait on the server to finish or be killed
	go func() {
		err = cmd.Wait()
		if err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("%v %v", err, s_err.String())
			}
		}
		// remove it from the list of dhcp servers
		delete(dhcpServers, id)

		// and clean up the directory
		err = os.RemoveAll(d.Path)
		if err != nil {
			log.Errorln(err)
		}
		log.Info("dhcp server %v quit", id)
	}()
	return nil
}

func dhcpPath() (string, error) {
	path, err := ioutil.TempDir(*f_base, "dhcp_")
	log.Info("created dhcp server path: %v", path)
	return path, err
}

func dhcpPID(id int) int {
	c, ok := dhcpServers[id]
	if !ok {
		return -1
	}
	path := c.Path

	buf, err := ioutil.ReadFile(path + "/dnsmasq.pid")
	if err != nil {
		log.Errorln(err)
		return -1
	}

	valString := strings.TrimSpace(string(buf))

	val, err := strconv.Atoi(valString)
	if err != nil {
		log.Errorln(err)
		return -1
	}

	return val
}
