// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type dnsmasqServer struct {
	Addr     string
	MinRange string
	MaxRange string
	Path     string
	dnsmasqHostdir string
	dnsmasqDHCPdir string
	dnsmasqDHCPoptsdir string
	dnsmasqHostIPs map[string]string	// map MAC to IP address
	dnsmasqDNSentries	map[string]string	// map IP to hostname
	dnsmasqDHCPopts	[]string	// DHCP options
}

var (
	dnsmasqServers map[int]*dnsmasqServer
	dnsmasqIdChan  = makeIDChan()
)

var dnsmasqCLIHandlers = []minicli.Handler{
	{ // dnsmasq
		HelpShort: "start a dhcp/dns server on a specified ip",
		HelpLong: `
Start a dhcp/dns server on a specified IP with a specified range. For example,
to start a DHCP server on IP 10.0.0.1 serving the range 10.0.0.2 -
10.0.254.254:

	dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254

To start only a from a config file:

	dnsmasq start /path/to/config

To list running dnsmasq servers, invoke dnsmasq with no arguments. To kill a
running dnsmasq server, specify its ID from the list of running servers. For
example, to kill dnsmasq server 2:

	dnsmasq kill 2

To kill all running dnsmasq servers, pass all as the ID:

	dnsmasq kill all

dnsmasq will provide DNS service from the host, as well as from /etc/hosts. You
can specify an additional config file for dnsmasq by providing a file as an
additional argument.

	dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254 /tmp/dnsmasq-extra.conf

NOTE: If specifying an additional config file, you must provide the full path
to the file.`,
		Patterns: []string{
			"dnsmasq",
			"dnsmasq start <listen address> <low dhcp range> <high dhcp range> [config]",
			"dnsmasq start <config>",
			"dnsmasq kill <id or all>",
		},
		Call: wrapSimpleCLI(cliDnsmasq),
	},
	{
		HelpShort: "configure dhcp/dns options",
		HelpLong: ``,
		Patterns: []string{
			"dnsmasq configure <ID> <ip,>",
			"dnsmasq configure <ID> <ip,> <mac address> <ip>",
			"dnsmasq configure <ID> <dns,>",
			"dnsmasq configure <ID> <dns,> <ip> <hostname>",
			"dnsmasq configure <ID> <options,>",
			"dnsmasq configure <ID> <options,> <optionstring>",
		},
		Call: wrapSimpleCLI(cliDnsmasqConfigure),
	},
}

func init() {
	dnsmasqServers = make(map[int]*dnsmasqServer)
}

func dnsmasqHostInfo(c *minicli.Command, resp *minicli.Response) {
			// print info about the mapping
			resp.Header = []string{"ID", "MAC", "IP"}
			resp.Tabular = [][]string{}
			if c.StringArgs["ID"] == Wildcard {
				for id, v := range dnsmasqServers {
					for mac, ip := range v.dnsmasqHostIPs {
						resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), mac, ip })
					}
				}
			} else {
				id, err := strconv.Atoi(c.StringArgs["ID"])
				if err != nil {
					resp.Error = "Invalid dnsmasq ID"
				}
				if _, ok := dnsmasqServers[id]; ok {
					for mac, ip := range dnsmasqServers[id].dnsmasqHostIPs {
						resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), mac, ip })
					}
				} else {
					resp.Error = "Invalid dnsmasq ID"
				}
			}
}

func dnsmasqDNSInfo(c *minicli.Command, resp *minicli.Response) {
			// print info
			resp.Header = []string{"ID", "IP", "Hostname"}
			resp.Tabular = [][]string{}
			if c.StringArgs["ID"] == Wildcard {
				for id, v := range dnsmasqServers {
					for ip, host := range v.dnsmasqDNSentries {
						resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), ip, host })
					}
				}
			} else {
				id, err := strconv.Atoi(c.StringArgs["ID"])
				if err != nil {
					resp.Error = "Invalid dnsmasq ID"
				}
				if _, ok := dnsmasqServers[id]; ok {
					for ip, host := range dnsmasqServers[id].dnsmasqDNSentries {
						resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), ip, host })
					}
				} else {
					resp.Error = "Invalid dnsmasq ID"
				}
			}
}

func dnsmasqDHCPOptionInfo(c *minicli.Command, resp *minicli.Response) {
	resp.Header = []string{"ID", "Option"}
	resp.Tabular = [][]string{}
	if c.StringArgs["ID"] == Wildcard {
		for id, v := range dnsmasqServers {
			for _, ent := range v.dnsmasqDHCPopts {
				resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), ent })
			}
		}
	} else {
		id, err := strconv.Atoi(c.StringArgs["ID"])
		if err != nil {
			resp.Error = "Invalid dnsmasq ID"
		}
		if _, ok := dnsmasqServers[id]; ok {
			for _, ent := range dnsmasqServers[id].dnsmasqDHCPopts {
				resp.Tabular = append(resp.Tabular, []string{ strconv.Itoa(id), ent })
			}
		} else {
			resp.Error = "Invalid dnsmasq ID"
		}
	}
}

func cliDnsmasqConfigure(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Figure out the appropriate dnsmasq ID

	if c.BoolArgs["ip"] {
		mac := c.StringArgs["mac"]
		ip := c.StringArgs["ip"]
		// They either want info, or they want to configure an IP
		if mac != "" && ip != "" {
			// Configure a mac->ip mapping
			if c.StringArgs["ID"] == Wildcard {
				for _, v := range dnsmasqServers {
					v.dnsmasqHostIPs[mac] = ip
				}
			} else {
				id, err := strconv.Atoi(c.StringArgs["ID"])
				if err != nil {
					resp.Error = "Invalid dnsmasq ID"
				}
				if _, ok := dnsmasqServers[id]; ok {
					dnsmasqServers[id].dnsmasqHostIPs[mac] = ip
				} else {
					resp.Error = "Invalid dnsmasq ID"
				}
			}
		} else {
			dnsmasqHostInfo(c, resp)
		}
	} else if c.BoolArgs["dns"] {
		hostname := c.StringArgs["hostname"]
		ip := c.StringArgs["ip"]
		if hostname != "" && ip != "" {
			// Configure an ip->hostname mapping
			if c.StringArgs["ID"] == Wildcard {
				for _, v := range dnsmasqServers {
					v.dnsmasqDNSentries[ip] = hostname
				}
			} else {
				id, err := strconv.Atoi(c.StringArgs["ID"])
				if err != nil {
					resp.Error = "Invalid dnsmasq ID"
				}
				if _, ok := dnsmasqServers[id]; ok {
					dnsmasqServers[id].dnsmasqDNSentries[ip] = hostname
				} else {
					resp.Error = "Invalid dnsmasq ID"
				}
			}
		} else {
			dnsmasqDNSInfo(c, resp)
		}
	} else if c.BoolArgs["options"] {
		optionstring := c.StringArgs["optionstring"]
		if optionstring != "" {
			if c.StringArgs["ID"] == Wildcard {
				for _, v := range dnsmasqServers {
					v.dnsmasqDHCPopts = append(v.dnsmasqDHCPopts, optionstring)
				}
			} else {
				id, err := strconv.Atoi(c.StringArgs["ID"])
				if err != nil {
					resp.Error = "Invalid dnsmasq ID"
				}
				dnsmasqServers[id].dnsmasqDHCPopts = append(dnsmasqServers[id].dnsmasqDHCPopts, optionstring)
			}
		} else {
			dnsmasqDHCPOptionInfo(c, resp)
		}
	} else {
		// wtf
		resp.Error = "Bad command"
	}
	return resp
}

func cliDnsmasq(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	if c.StringArgs["id"] == Wildcard {
		// Must be "kill all"
		err = dnsmasqKillAll()
	} else if c.StringArgs["id"] != "" {
		// Must be "kill <id>"
		var id int
		id, err = strconv.Atoi(c.StringArgs["id"])
		if err == nil {
			err = dnsmasqKill(id)
		}
	} else if c.StringArgs["listen"] != "" || c.StringArgs["config"] != "" {
		// Must be "start"
		// We don't need to differentiate between the two start commands
		// because dnsmasqStart expects the zero string value when values
		// are not specified.
		err = dnsmasqStart(
			c.StringArgs["listen"],
			c.StringArgs["low"],
			c.StringArgs["high"],
			c.StringArgs["config"])
	} else {
		// Must be "list"
		resp.Header = []string{"ID", "Listening Address", "Min", "Max", "Path", "PID"}
		resp.Tabular = [][]string{}
		for id, c := range dnsmasqServers {
			pid := dnsmasqPID(id)
			resp.Tabular = append(resp.Tabular, []string{
				strconv.Itoa(id),
				c.Addr,
				c.MinRange,
				c.MaxRange,
				c.Path,
				strconv.Itoa(pid)})
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func dnsmasqKillAll() error {
	errs := []string{}

	for c, _ := range dnsmasqServers {
		err := dnsmasqKill(c)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%v", err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func dnsmasqKill(id int) error {
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

	id := <-dnsmasqIdChan

	d := &dnsmasqServer{
		Addr:     ip,
		MinRange: min,
		MaxRange: max,
		Path:     path,
	}

	d.dnsmasqHostIPs = make(map[string]string)
	d.dnsmasqDNSentries = make(map[string]string)
	d.dnsmasqDHCPopts = []string{}

	d.dnsmasqHostdir = filepath.Join(path, "hostdir")
	d.dnsmasqDHCPdir = filepath.Join(path, "dhcpdir")
	d.dnsmasqDHCPoptsdir = filepath.Join(path, "dhcpoptsdir")
	err = os.MkdirAll(d.dnsmasqHostdir, 0755)
	if err != nil { return err }
	err = os.MkdirAll(d.dnsmasqDHCPdir, 0755)
	if err != nil { return err }
	err = os.MkdirAll(d.dnsmasqDHCPoptsdir, 0755)
	if err != nil { return err }

	p := process("dnsmasq")
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			fmt.Sprintf("--pid-file=%v/dnsmasq.pid", d.Path),
			"-o",
			"-k",
			fmt.Sprintf("--hostsdir=%v", d.dnsmasqHostdir),
			fmt.Sprintf("--dhcp-hostsdir=%v", d.dnsmasqDHCPdir),
			fmt.Sprintf("--dhcp-optsdir=%v", d.dnsmasqDHCPoptsdir),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	if ip != "" {
		cmd.Args = append(cmd.Args, "--except-interface")
		cmd.Args = append(cmd.Args, "lo")
		cmd.Args = append(cmd.Args, "--listen-address")
		cmd.Args = append(cmd.Args, ip)
		cmd.Args = append(cmd.Args, "--bind-interfaces")
		cmd.Args = append(cmd.Args, "--dhcp-range")
		cmd.Args = append(cmd.Args, fmt.Sprintf("%v,%v", min, max))
		cmd.Args = append(cmd.Args, fmt.Sprintf("--dhcp-leasefile=%v/dnsmasq.leases", d.Path))
		cmd.Args = append(cmd.Args, "--dhcp-lease-max=4294967295")
	}
	if hosts != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--conf-file=%v", hosts))
	}
	log.Debug("starting dnsmasq server with command: %v", cmd)
	err = cmd.Start()
	if err != nil {
		return err
	}

	dnsmasqServers[id] = d

	// wait on the server to finish or be killed
	go func() {
		err = cmd.Wait()
		if err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("killing dnsmasq: %v %v", err, sErr.String())
			}
		}
		// remove it from the list of dnsmasq servers
		delete(dnsmasqServers, id)

		// and clean up the directory
		err = os.RemoveAll(d.Path)
		if err != nil {
			log.Error("removing dnsmasq directory: %v", err)
		}
		log.Info("dnsmasq server %v quit", id)
	}()
	return nil
}

func dnsmasqPath() (string, error) {
	path, err := ioutil.TempDir(*f_base, "dnsmasq_")
	log.Infoln("created dnsmasq server path: ", path)
	return path, err
}

func dnsmasqPID(id int) int {
	c, ok := dnsmasqServers[id]
	if !ok {
		return -1
	}
	path := c.Path

	buf, err := ioutil.ReadFile(filepath.Join(path, "dnsmasq.pid"))
	if err != nil {
		log.Error("read dnsmasq pidfile: %v", err)
		return -1
	}

	valString := strings.TrimSpace(string(buf))

	val, err := strconv.Atoi(valString)
	if err != nil {
		log.Error("parse dnsmasq pid: %v", err)
		return -1
	}

	return val
}
