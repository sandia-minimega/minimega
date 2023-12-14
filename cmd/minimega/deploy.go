// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/ranges"
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

	deploy flags -base=/opt/minimega -level=debug

To customize stdout and stderr, use 'deploy stdout' and 'deploy stderr':

	deploy stdout /var/log/minimega.out
	deploy stderr /var/log/minimega.err

By default, stdout and stderr are written to /dev/null.
	`,
		Patterns: []string{
			"deploy <launch,> <hosts>",
			"deploy <launch,> <hosts> <user> [sudo,]",
			"deploy <flags,> [minimega flags]...",
			"deploy <stdout,> [path]",
			"deploy <stderr,> [path]",
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
var deployStdout string
var deployStderr string

func cliDeploy(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	hosts := c.StringArgs["hosts"]
	user := c.StringArgs["user"]
	sudo := c.BoolArgs["sudo"]
	flagsList := c.ListArgs["minimega"]

	if c.BoolArgs["flags"] {
		if flagsList == nil {
			resp.Response = deployGetFlags()
			return nil
		}

		deployFlags = flagsList
		return nil
	} else if c.BoolArgs["stdout"] {
		if c.StringArgs["path"] == "" {
			resp.Response = deployStdout
			return nil
		}

		deployStdout = c.StringArgs["path"]
		return nil
	} else if c.BoolArgs["stderr"] {
		if c.StringArgs["path"] == "" {
			resp.Response = deployStderr
			return nil
		}

		deployStderr = c.StringArgs["path"]
		return nil
	}

	hostsExpanded, err := ranges.SplitList(hosts)
	if err != nil {
		return err
	}
	log.Debug("got expanded hosts: %v", hostsExpanded)

	// Append timestamp to filename so that each deploy produces a new binary
	// on the remote system. Using the timestamp allows us to quickly identify
	// the latest binary after multiple deployments.
	fname := fmt.Sprintf("minimega_deploy_%v", time.Now().Unix())
	remotePath := filepath.Join(os.TempDir(), fname)
	log.Debug("remotePath: %v", remotePath)

	// copy minimega
	errs := deployCopy(hostsExpanded, user, remotePath)

	// launch minimega on each remote node
	errs2 := deployRun(hostsExpanded, user, remotePath, sudo)

	return makeErrSlice(append(errs, errs2...))
}

func deployCopy(hosts []string, user, remotePath string) []error {
	log.Debug("deployCopy: %v, %v", hosts, user)

	var errs []error

	minimegaBinary := fmt.Sprintf("/proc/%v/exe", os.Getpid())
	log.Debug("minimega binary: %v", minimegaBinary)

	for _, host := range hosts {
		command := []string{"scp", "-B", "-o", "StrictHostKeyChecking=no", minimegaBinary}
		if user != "" {
			command = append(command, fmt.Sprintf("%v@%v:%v", user, host, remotePath))
		} else {
			command = append(command, fmt.Sprintf("%v:%v", host, remotePath))
		}
		log.Debug("scp command: %v", command)

		out, err := processWrapper(command...)
		if err != nil {
			errs = append(errs, fmt.Errorf("%v: %v", err, out))
		}
	}

	return errs
}

func deployRun(hosts []string, user, remotePath string, sudo bool) []error {
	log.Debug("deployRun: %v, %v", hosts, user)

	var errs []error

	// minimega command
	flags := deployGetFlags()
	log.Debug("minimega flags: %v", flags)

	stdout := "/dev/null"
	if deployStdout != "" {
		stdout = deployStdout
	}
	stderr := "/dev/null"
	if deployStderr != "" {
		stderr = deployStderr
	}

	var minimegaCommand string
	if sudo {
		minimegaCommand = fmt.Sprintf("sudo -b nohup %v %v > %v 2> %v &", remotePath, flags, stdout, stderr)
	} else {
		minimegaCommand = fmt.Sprintf("nohup %v %v > %v 2>%v &", remotePath, flags, stdout, stderr)
	}

	for _, host := range hosts {
		command := []string{"ssh", "-o", "StrictHostKeyChecking=no"}
		if user != "" {
			command = append(command, fmt.Sprintf("%v@%v", user, host))
		} else {
			command = append(command, fmt.Sprintf("%v", host))
		}
		command = append(command, minimegaCommand)
		log.Debug("ssh command: %v", command)

		out, err := processWrapper(command...)
		if err != nil {
			errs = append(errs, fmt.Errorf("%v: %v", err, out))
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

	// Default to having all mesh nodes using this node as the head node for
	// logging and file transfer purposes.
	flags := []string{fmt.Sprintf("-headnode=%s", hostname)}

	flag.VisitAll(func(f *flag.Flag) {
		// ignore headnode setting for this node (will likely be empty)
		if f.Name == "headnode" {
			return
		}

		if f.Name == "nostdin" {
			flags = append(flags, fmt.Sprintf("-%v=true", f.Name))
		} else {
			flags = append(flags, fmt.Sprintf("-%v=%v", f.Name, f.Value.String()))
		}
	})

	return strings.Join(flags, " ")
}

func cliDeployClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	deployFlags = nil
	return nil
}
