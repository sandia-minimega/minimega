// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var deployCLIHandlers = []minicli.Handler{
	{ // deploy
		HelpShort: "copy and run minimega on remote nodes",
		HelpLong: `
deploy copies and runs minimega on remote nodes, facilitating the deployment of
minimega to a cluster. By default, deploy will launch minimega with the same
flags used when starting this minimega, and add the -nostdin flag so that the
remote minimega can be backgrounded. For example, to launch minimega on nodes
kn1 and kn2:

	deploy launch kn[1-2]

deploy uses scp/ssh to copy and run minimega. By default, minimega will attempt
to login to remote nodes using the current user. This can be changed by
providing a username. If using a different username, you can optionally specify
the use of sudo when launching minimega (you typically need to run minimega as
root).

In order to override the flags passed to remote minimega instances, provide
flags with 'deploy flags'. For example:

	deploy flags -base=/opt/minimega -level=debug`,
		Patterns: []string{
			"deploy <launch,> <hosts>",
			"deploy <launch,> <hosts> <user> [sudo,]",
			"deploy <flags,> [minimega flags]...",
		},
		Call: wrapSimpleCLI(cliDeploy),
	},
	{ // clear deploy
		HelpShort: "reset deploy flags",
		HelpLong: `
Reset the deploy flags to their default value, which is equal to the launch
flags used when launching minimega.`,
		Patterns: []string{
			"clear deploy flags",
		},
		Call: wrapSimpleCLI(cliDeployClear),
	},
}

var deployFlags []string

func init() {
	registerHandlers("deploy", deployCLIHandlers)
}

func cliDeploy(c *minicli.Command) *minicli.Response {
	log.Debugln("deploy")

	resp := &minicli.Response{Host: hostname}

	hosts := c.StringArgs["hosts"]
	user := c.StringArgs["user"]
	sudo := c.BoolArgs["sudo"]
	flagsList := c.ListArgs["minimega"]

	if c.BoolArgs["flags"] {
		if flagsList == nil {
			resp.Response = deployGetFlags()
		} else {
			deployFlags = flagsList
		}
		return resp
	}

	hostsExpanded := getRecipients(hosts)
	log.Debug("got expanded hosts: %v", hostsExpanded)

	suffix := rand.New(rand.NewSource(time.Now().UnixNano())).Int31()
	remotePath := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_deploy_%v", suffix))
	log.Debug("remotePath: %v", remotePath)

	// copy minimega
	errs := deployCopy(hostsExpanded, user, remotePath)
	if errs != nil {
		// just report the errors and keep trying
		for _, e := range errs {
			resp.Error += fmt.Sprintf("%v\n", e.Error())
		}
	}

	// launch minimega on each remote node
	errs = deployRun(hostsExpanded, user, remotePath, sudo)
	if errs != nil {
		for _, e := range errs {
			resp.Error += fmt.Sprintf("%v\n", e.Error())
		}
	}

	return resp
}

func deployCopy(hosts []string, user, remotePath string) []error {
	log.Debug("deployCopy: %v, %v", hosts, user)

	var errs []error

	minimegaBinary := fmt.Sprintf("/proc/%v/exe", os.Getpid())
	log.Debug("minimega binary: %v", minimegaBinary)

	// scp to each host
	scp := process("scp")

	for _, host := range hosts {
		command := []string{"-B", "-o", "StrictHostKeyChecking=no", minimegaBinary}
		if user != "" {
			command = append(command, fmt.Sprintf("%v@%v:%v", user, host, remotePath))
		} else {
			command = append(command, fmt.Sprintf("%v:%v", host, remotePath))
		}
		log.Debug("scp command: %v %v", scp, command)

		cmd := exec.Command(scp, command...)

		out, err := cmd.CombinedOutput()
		if err != nil {
			errs = append(errs, fmt.Errorf("%v: %v", err, string(out)))
		}
	}

	return errs
}

func deployRun(hosts []string, user, remotePath string, sudo bool) []error {
	log.Debug("deployRun: %v, %v", hosts, user)

	var errs []error

	// ssh to each host
	ssh := process("ssh")

	// minimega command
	flags := deployGetFlags()
	log.Debug("minimega flags: %v", flags)

	var minimegaCommand string
	if sudo {
		minimegaCommand = fmt.Sprintf("sudo -b nohup %v %v > /dev/null 2>&1 &", remotePath, flags)
	} else {
		minimegaCommand = fmt.Sprintf("nohup %v %v > /dev/null 2>&1 &", remotePath, flags)
	}

	for _, host := range hosts {
		command := []string{"-o", "StrictHostKeyChecking=no"}
		if user != "" {
			command = append(command, fmt.Sprintf("%v@%v", user, host))
		} else {
			command = append(command, fmt.Sprintf("%v", host))
		}
		command = append(command, minimegaCommand)
		log.Debug("ssh command: %v %v", ssh, command)

		cmd := exec.Command(ssh, command...)

		out, err := cmd.CombinedOutput()
		if err != nil {
			errs = append(errs, fmt.Errorf("%v: %v", err, string(out)))
		}
	}

	return errs
}

func deployGetFlags() string {
	if deployFlags != nil {
		f := strings.Join(deployFlags, " ")
		if !strings.Contains(f, "nostdin") {
			f += " -nostdin=true"
		}
		return f
	}
	var flags []string
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "nostdin" {
			flags = append(flags, fmt.Sprintf("-%v=true", f.Name))
		} else {
			flags = append(flags, fmt.Sprintf("-%v=%v", f.Name, f.Value.String()))
		}
	})
	return strings.Join(flags, " ")
}

func cliDeployClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	deployFlags = nil

	return resp
}
