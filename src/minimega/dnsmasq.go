// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
//

// minimega dhcp and dns server support
package main

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

type dnsmasqServer struct {
	Addr     string
	MinRange string
	MaxRange string
	Path     string
}

var (
	dnsmasqServers     map[int]*dnsmasqServer
	dnsmasqServerCount int
)

func init() {
	dnsmasqServers = make(map[int]*dnsmasqServer)
}

// generate paths for the leases and pid files (should be unique) so we can support multiple dnsmasq servers
// maintain a map of dnsmasq servers that can be listed
// allow killing dnsmasq servers with dnsmasq kill 

func dnsmasqCLI(c cliCommand) cliResponse {
	var ret cliResponse
	switch len(c.Args) {
	case 0:
		// show the list of dnsmasq servers
		ret.Response = dnsmasqList()
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
		err = dnsmasqKill(val)
		if err != nil {
			ret.Error = err.Error()
		}
	case 4, 5:
		if c.Args[0] != "start" {
			ret.Error = "malformed command"
			break
		}
		var err error
		if len(c.Args) == 4 {
			err = dnsmasqStart(c.Args[1], c.Args[2], c.Args[3], "")
		} else {
			err = dnsmasqStart(c.Args[1], c.Args[2], c.Args[3], c.Args[4])
		}
		if err != nil {
			ret.Error = err.Error()
		}
	default:
		ret.Error = "malformed command"
	}
	return ret
}

func dnsmasqList() string {
	w := new(tabwriter.Writer)
	buf := new(bytes.Buffer)
	w.Init(buf, 0, 8, 1, ' ', 0)
	fmt.Fprintf(w, "ID\t:\tListening Address\tMin\tMax\tPath\tPID\n")
	for id, c := range dnsmasqServers {
		pid := dnsmasqPID(id)
		fmt.Fprintf(w, "%v\t:\t%v\t%v\t%v\t%v\t%v\n", id, c.Addr, c.MinRange, c.MaxRange, c.Path, pid)
	}
	w.Flush()
	return buf.String()
}

func dnsmasqKill(id int) error {
	if id == -1 {
		var e string
		for c, _ := range dnsmasqServers {
			err := dnsmasqKill(c)
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

	pid := dnsmasqPID(id)
	log.Debug("dnsmasq id %v has pid %v", id, pid)
	if pid == -1 {
		return fmt.Errorf("invalid id")
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("kill")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			fmt.Sprintf("%v", pid),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Infoln("killing dnsmasq server:", pid)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func dnsmasqStart(ip, min, max, hosts string) error {
	path, err := dnsmasqPath()
	if err != nil {
		return err
	}

	d := &dnsmasqServer{
		Addr:     ip,
		MinRange: min,
		MaxRange: max,
		Path:     path,
	}

	p := process("dnsmasq")
	var sOut bytes.Buffer
	var sErr bytes.Buffer
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
		Stdout: &sOut,
		Stderr: &sErr,
	}
	if hosts != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--addn-hosts=%v", hosts))
	}
	log.Debug("starting dnsmasq server with command: %v", cmd)
	err = cmd.Start()
	if err != nil {
		return err
	}

	id := dnsmasqServerCount
	dnsmasqServerCount++
	dnsmasqServers[id] = d

	// wait on the server to finish or be killed
	go func() {
		err = cmd.Wait()
		if err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("%v %v", err, sErr.String())
			}
		}
		// remove it from the list of dnsmasq servers
		delete(dnsmasqServers, id)

		// and clean up the directory
		err = os.RemoveAll(d.Path)
		if err != nil {
			log.Errorln(err)
		}
		log.Infoln("dnsmasq server %v quit", id)
	}()
	return nil
}

func dnsmasqPath() (string, error) {
	path, err := ioutil.TempDir(*f_base, "dnsmasq_")
	log.Infoln("created dnsmasq server path: %v", path)
	return path, err
}

func dnsmasqPID(id int) int {
	c, ok := dnsmasqServers[id]
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
