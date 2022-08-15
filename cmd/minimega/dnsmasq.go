// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type dnsmasqServer struct {
	Addr            string
	MinRange        string
	MaxRange        string
	Path            string
	Hostdir         string
	DHCPdir         string
	DHCPoptsdir     string
	DHCPhosts       map[string]string // map MAC to IP address
	Hostnames       map[string]string // map IP to hostname
	DHCPopts        []string          // DHCP options
	UpstreamServers []string          // upstream DNS servers to use
}

var (
	dnsmasqServers map[int]*dnsmasqServer
	dnsmasqID      = NewCounter()
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
		HelpLong: `
Configuration options for running dnsmasq instances. Define a static IP
allocation, specify a hostname->IP mapping for DNS, configure upstream DNS
servers (useful when forwarding/NAT is enabled), or set DHCP options.

To list all existing static IP allocations on the first running dnsmasq
server, do the following:

	dnsmasq configure 0 ip

To set up a static IP allocation for a VM with the MAC address
00:11:22:33:44:55:

	dnsmasq configure 0 ip 00:11:22:33:44:55 172.17.0.50

To see DNS entries:

	dnsmasq configure 0 dns

To add a DNS entry:

	dnsmasq configure 0 dns 172.17.0.50 example.com

To see upstream DNS servers:

	dnsmasq configure 0 upstream

To add an upstream DNS server:

	dnsmasq configure 0 upstream server 1.1.1.1

To see a list of all DHCP options:

	dnsmasq configure 0 options

To add a DHCP option:

	dnsmasq configure 0 options option:dns-server,172.17.0.254
`,
		Patterns: []string{
			"dnsmasq configure <ID> <ip,>",
			"dnsmasq configure <ID> <ip,> <mac address> <ip>",
			"dnsmasq configure <ID> <dns,>",
			"dnsmasq configure <ID> <dns,> <ip> <hostname>",
			"dnsmasq configure <ID> <dns,> <upstream,>",
			"dnsmasq configure <ID> <dns,> <upstream,> server <ip>",
			"dnsmasq configure <ID> <options,>",
			"dnsmasq configure <ID> <options,> <optionstring>",
		},
		Call: wrapSimpleCLI(cliDnsmasqConfigure),
	},
}

func init() {
	dnsmasqServers = make(map[int]*dnsmasqServer)
}

func dnsmasqUpstreamInfo(c *minicli.Command, resp *minicli.Response) {
	// print info about upstream servers
	resp.Header = []string{"id", "upstream server"}
	resp.Tabular = [][]string{}

	if c.StringArgs["ID"] == Wildcard {
		for id, v := range dnsmasqServers {
			for _, upstream := range v.UpstreamServers {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), upstream})
			}
		}
	} else {
		id, err := strconv.Atoi(c.StringArgs["ID"])
		if err != nil {
			resp.Error = "Invalid dnsmasq ID"
			return
		}

		if _, ok := dnsmasqServers[id]; ok {
			for _, upstream := range dnsmasqServers[id].UpstreamServers {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), upstream})
			}
		} else {
			resp.Error = "Invalid dnsmasq ID"
		}
	}
}

func dnsmasqHostInfo(c *minicli.Command, resp *minicli.Response) {
	// print info about the mapping
	resp.Header = []string{"id", "mac", "ip"}
	resp.Tabular = [][]string{}
	if c.StringArgs["ID"] == Wildcard {
		for id, v := range dnsmasqServers {
			for mac, ip := range v.DHCPhosts {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), mac, ip})
			}
		}
	} else {
		id, err := strconv.Atoi(c.StringArgs["ID"])
		if err != nil {
			resp.Error = "Invalid dnsmasq ID"
			return
		}
		if _, ok := dnsmasqServers[id]; ok {
			for mac, ip := range dnsmasqServers[id].DHCPhosts {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), mac, ip})
			}
		} else {
			resp.Error = "Invalid dnsmasq ID"
		}
	}
}

func dnsmasqDNSInfo(c *minicli.Command, resp *minicli.Response) {
	// print info
	resp.Header = []string{"id", "ip", "hostname"}
	resp.Tabular = [][]string{}
	if c.StringArgs["ID"] == Wildcard {
		for id, v := range dnsmasqServers {
			for ip, host := range v.Hostnames {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), ip, host})
			}
		}
	} else {
		id, err := strconv.Atoi(c.StringArgs["ID"])
		if err != nil {
			resp.Error = "Invalid dnsmasq ID"
			return
		}
		if _, ok := dnsmasqServers[id]; ok {
			for ip, host := range dnsmasqServers[id].Hostnames {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), ip, host})
			}
		} else {
			resp.Error = "Invalid dnsmasq ID"
		}
	}
}

func dnsmasqDHCPOptionInfo(c *minicli.Command, resp *minicli.Response) {
	resp.Header = []string{"id", "option"}
	resp.Tabular = [][]string{}
	if c.StringArgs["ID"] == Wildcard {
		for id, v := range dnsmasqServers {
			for _, ent := range v.DHCPopts {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), ent})
			}
		}
	} else {
		id, err := strconv.Atoi(c.StringArgs["ID"])
		if err != nil {
			resp.Error = "Invalid dnsmasq ID"
			return
		}
		if _, ok := dnsmasqServers[id]; ok {
			for _, ent := range dnsmasqServers[id].DHCPopts {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(id), ent})
			}
		} else {
			resp.Error = "Invalid dnsmasq ID"
		}
	}
}

func (d *dnsmasqServer) writeUpstreamServersFile() {
	// Generate the new file contents
	var upstreamfile string
	for _, upstream := range d.UpstreamServers {
		upstreamfile = upstreamfile + fmt.Sprintf("nameserver %s\n", upstream)
	}

	// ioutil.WriteFile to save it
	ioutil.WriteFile(filepath.Join(d.Path, "resolv.conf"), []byte(upstreamfile), 0644)
}

func (d *dnsmasqServer) writeHostFile() {
	// Generate the new file contents
	var hostsfile string
	for ip, host := range d.Hostnames {
		hostsfile = hostsfile + fmt.Sprintf("%s	%s\n", ip, host)
	}

	// ioutil.WriteFile to save it
	ioutil.WriteFile(filepath.Join(d.Hostdir, "hosts"), []byte(hostsfile), 0755)
}

func (d *dnsmasqServer) writeDHCPhosts() {
	// Generate the contents
	var contents string
	for mac, ip := range d.DHCPhosts {
		contents = contents + fmt.Sprintf("%s,%s\n", mac, ip)
	}

	ioutil.WriteFile(filepath.Join(d.DHCPdir, "dhcp"), []byte(contents), 0755)
}

func (d *dnsmasqServer) writeDHCPopts() {
	// Generate the contents
	var contents string
	for _, opt := range d.DHCPopts {
		contents = contents + fmt.Sprintf("%s\n", opt)
	}

	ioutil.WriteFile(filepath.Join(d.DHCPoptsdir, "dhcp-options"), []byte(contents), 0755)
}

func cliDnsmasqConfigure(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	argID := c.StringArgs["ID"]
	id, err := strconv.Atoi(argID)
	if argID != Wildcard && err != nil {
		return errors.New("invalid dnsmasq ID")
	} else if err == nil {
		// Make sure we have a dnsmasq instance with that ID
		if _, ok := dnsmasqServers[id]; !ok {
			return errors.New("invalid dnsmasq ID")
		}
	}

	if c.BoolArgs["ip"] {
		mac := c.StringArgs["mac"]
		ip := c.StringArgs["ip"]

		// They either want info, or they want to configure an IP
		if mac != "" && ip != "" {
			// Configure a mac->ip mapping
			if argID == Wildcard {
				for _, v := range dnsmasqServers {
					v.DHCPhosts[mac] = ip
					v.writeDHCPhosts()
				}
			} else {
				dnsmasqServers[id].DHCPhosts[mac] = ip
				dnsmasqServers[id].writeDHCPhosts()
			}
		} else {
			dnsmasqHostInfo(c, resp)
		}

		return nil
	} else if c.BoolArgs["dns"] {
		if c.BoolArgs["upstream"] {
			ip := c.StringArgs["ip"]

			if ip == "" {
				dnsmasqUpstreamInfo(c, resp)
			} else {
				if argID == Wildcard {
					for _, v := range dnsmasqServers {
						v.UpstreamServers = append(v.UpstreamServers, ip)
						v.writeUpstreamServersFile()
					}
				} else {
					dnsmasqServers[id].UpstreamServers = append(dnsmasqServers[id].UpstreamServers, ip)
					dnsmasqServers[id].writeUpstreamServersFile()
				}
			}

			return nil
		}

		hostname := c.StringArgs["hostname"]
		ip := c.StringArgs["ip"]

		if hostname != "" && ip != "" {
			// Configure an ip->hostname mapping
			if argID == Wildcard {
				for _, v := range dnsmasqServers {
					v.Hostnames[ip] = hostname
					v.writeHostFile()
				}
			} else {
				dnsmasqServers[id].Hostnames[ip] = hostname
				dnsmasqServers[id].writeHostFile()
			}
		} else {
			dnsmasqDNSInfo(c, resp)
		}

		return nil
	} else if c.BoolArgs["options"] {
		optionstring := c.StringArgs["optionstring"]

		if optionstring != "" {
			if argID == Wildcard {
				for _, v := range dnsmasqServers {
					v.DHCPopts = append(v.DHCPopts, optionstring)
					v.writeDHCPopts()
				}
			} else {
				dnsmasqServers[id].DHCPopts = append(dnsmasqServers[id].DHCPopts, optionstring)
				dnsmasqServers[id].writeDHCPopts()
			}
		} else {
			dnsmasqDHCPOptionInfo(c, resp)
		}

		return nil
	}

	return unreachable()
}

func cliDnsmasq(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.StringArgs["id"] == Wildcard {
		// Must be "kill all"
		return dnsmasqKillAll()
	} else if c.StringArgs["id"] != "" {
		// Must be "kill <id>"
		id, err := strconv.Atoi(c.StringArgs["id"])
		if err != nil {
			return err
		}

		return dnsmasqKill(id)
	} else if c.StringArgs["listen"] != "" || c.StringArgs["config"] != "" {
		// Must be "start"
		// We don't need to differentiate between the two start commands
		// because dnsmasqStart expects the zero string value when values
		// are not specified.
		return dnsmasqStart(
			c.StringArgs["listen"],
			c.StringArgs["low"],
			c.StringArgs["high"],
			c.StringArgs["config"])
	}

	// Must be "list"
	resp.Header = []string{"id", "address", "min", "max", "path", "pid"}
	resp.Tabular = [][]string{}
	for id, s := range dnsmasqServers {
		pid := dnsmasqPID(id)
		resp.Tabular = append(resp.Tabular, []string{
			strconv.Itoa(id),
			s.Addr,
			s.MinRange,
			s.MaxRange,
			s.Path,
			strconv.Itoa(pid)})
	}

	return nil
}

func dnsmasqKillAll() error {
	errs := []error{}

	for c, _ := range dnsmasqServers {
		errs = append(errs, dnsmasqKill(c))
	}

	return makeErrSlice(errs)
}

func dnsmasqKill(id int) error {
	pid := dnsmasqPID(id)
	log.Debug("dnsmasq id %v has pid %v", id, pid)
	if pid == -1 {
		return fmt.Errorf("invalid id")
	}

	log.Infoln("killing dnsmasq server:", pid)
	return syscall.Kill(pid, syscall.SIGTERM)
}

func dnsmasqStart(ip, min, max, config string) error {
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

	d.DHCPhosts = make(map[string]string)
	d.Hostnames = make(map[string]string)
	d.DHCPopts = []string{}

	d.Hostdir = filepath.Join(path, "hostdir")
	d.DHCPdir = filepath.Join(path, "dhcpdir")
	d.DHCPoptsdir = filepath.Join(path, "dhcpoptsdir")

	if err := os.MkdirAll(d.Hostdir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(d.DHCPdir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(d.DHCPoptsdir, 0755); err != nil {
		return err
	}

	p, err := process("dnsmasq")
	if err != nil {
		return err
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer

	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"--keep-in-foreground",
			"--user=root",
			fmt.Sprintf("--dhcp-hostsdir=%v", d.DHCPdir),
			fmt.Sprintf("--dhcp-optsdir=%v", d.DHCPoptsdir),
			fmt.Sprintf("--hostsdir=%v", d.Hostdir),
			fmt.Sprintf("--pid-file=%v/dnsmasq.pid", d.Path),
			fmt.Sprintf("--resolv-file=%v/resolv.conf", d.Path),
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

	if config != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--conf-file=%v", config))
	}

	log.Debug("starting dnsmasq server with command: %v", cmd)

	if err := cmd.Start(); err != nil {
		return err
	}

	id := dnsmasqID.Next()
	dnsmasqServers[id] = d

	// wait on the server to finish or be killed
	go func() {
		if err := cmd.Wait(); err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("killing dnsmasq: %v %v", err, sErr.String())
			}
		}
		// remove it from the list of dnsmasq servers
		delete(dnsmasqServers, id)

		// and clean up the directory
		if err := os.RemoveAll(d.Path); err != nil {
			log.Error("removing dnsmasq directory: %v", err)
		}

		log.Info("dnsmasq server %v quit", id)
	}()

	return nil
}

func dnsmasqPath() (string, error) {
	path, err := ioutil.TempDir(*f_base, "dnsmasq_")
	os.Chmod(path, 0755)
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
