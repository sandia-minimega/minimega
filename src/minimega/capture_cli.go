// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	"strconv"
)

var captureCLIHandlers = []minicli.Handler{
	{ // capture listing
		HelpShort: "show active captures",
		Patterns: []string{
			"capture",
			"capture <netflow,>",
			"capture <pcap,>",
		},
		Call: wrapBroadcastCLI(cliCaptureList),
	},
	{ // capture for VM
		HelpShort: "capture experiment data for a VM",
		Patterns: []string{
			"capture <pcap,> vm <name> <interface index> <filename>",
			"capture <pcap,> <delete,> vm <name>",
		},
		Call: wrapVMTargetCLI(cliCaptureVM),
	},
	{ // capture
		HelpShort: "capture experiment data",
		HelpLong: `
Note: the capture API is not fully namespace-aware and should be used with
caution. See note below.

Capture experiment data including netflow and PCAP. Netflow capture obtains
netflow data from any local openvswitch switch, and can write to file, another
socket, or both. Netflow data can be written out in raw or ascii format, and
file output can be compressed on the fly. Multiple netflow writers can be
configured.

PCAP capture can be from a bridge or VM interface. No filters are applied, and
all data seen on that interface is captured to file.

For example, to capture netflow data on bridge mega_bridge to file in ascii
mode and with gzip compression:

	capture netflow bridge mega_bridge file foo.netflow ascii gzip

You can change the active flow timeout with:

	capture netflow timeout <timeout>

With <timeout> in seconds.

To capture pcap on bridge 'foo' to file 'foo.pcap':

	capture pcap bridge foo foo.pcap

To capture pcap on VM 'foo' to file 'foo.pcap', using the 2nd interface on that
VM:

	capture pcap vm foo 0 foo.pcap

When run without arguments, capture prints all running captures. To stop a
capture, use the delete commands:

	capture netflow delete bridge <bridge>
	capture pcap delete bridge <bridge>
	capture pcap delete vm <name>

To stop all captures of a particular kind, replace id with "all". To stop all
capture of all types, use "clear capture". If a VM has multiple interfaces and
there are multiple captures running, calling "capture pcap delete vm <name>"
stops all the captures for that VM.

Notes with namespaces:
 * "capture [netflow,pcap]" lists captures across the namespace
 * "capture pcap vm ..." captures traffic for a VM in the current namespace
 * "capture netflow ..." and "capture pcap ..." only run on the local node --
   you must manually "mesh send all ..." if you wish to use them.
 * if you capture traffic from a bridge, you will see traffic from other
   experiments.
 * "clear capture" clears captures across the namespace.`,
		Patterns: []string{
			"capture <netflow,> <timeout,> [timeout]",
			"capture <netflow,> <bridge,> <bridge>",
			"capture <netflow,> <bridge,> <bridge> <file,> <filename>",
			"capture <netflow,> <bridge,> <bridge> <file,> <filename> <raw,ascii> [gzip]",
			"capture <netflow,> <bridge,> <bridge> <socket,> <tcp,udp> <hostname:port> <raw,ascii>",
			"capture <netflow,> <delete,> bridge <bridge>",

			"capture <pcap,> bridge <bridge> <filename>",
			"capture <pcap,> <delete,> bridge <bridge>",
		},
		Call: wrapSimpleCLI(cliCapture),
	},
	{ // clear capture
		HelpShort: "reset capture state",
		HelpLong: `
Resets state for captures across the namespace. See "help capture" for more
information.`,
		Patterns: []string{
			"clear capture [netflow,pcap]",
		},
		Call: wrapBroadcastCLI(cliCaptureClear),
	},
}

func cliCapture(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["netflow"] {
		// Capture to netflow
		return cliCaptureNetflow(ns, c, resp)
	} else if c.BoolArgs["pcap"] {
		// Capture to pcap
		return cliCapturePcap(ns, c, resp)
	}

	return errors.New("unreachable")
}

func cliCaptureList(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{"bridge"}

	if !c.BoolArgs["netflow"] && !c.BoolArgs["pcap"] {
		resp.Header = append(resp.Header, "type")
	}

	if !c.BoolArgs["netflow"] {
		resp.Header = append(resp.Header, "interface")
	}
	if !c.BoolArgs["pcap"] {
		resp.Header = append(resp.Header, "mode", "compress")
	}

	resp.Header = append(resp.Header, "path")

	resp.Tabular = [][]string{}
	for _, v := range ns.captures.m {
		row := []string{
			v.Bridge,
		}

		if !c.BoolArgs["netflow"] && !c.BoolArgs["pcap"] {
			row = append(row, v.Type)
		}

		if !c.BoolArgs["netflow"] || (c.BoolArgs["pcap"] && v.Type == "pcap") {
			if v.VM != nil {
				row = append(row, fmt.Sprintf("%v:%v", v.VM.GetName(), v.Interface))
			} else {
				row = append(row, "N/A")
			}
		}

		if !c.BoolArgs["pcap"] || (c.BoolArgs["netflow"] && v.Type == "netflow") {
			row = append(row, v.Mode, strconv.FormatBool(v.Compress))
		}

		row = append(row, v.Path)

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliCaptureVM(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["name"]
	fname := c.StringArgs["filename"]
	iface := c.StringArgs["interface"]

	// stopping capture for one or all VMs
	if c.BoolArgs["delete"] {
		return ns.captures.StopVM(name, "pcap")
	}

	vm := ns.FindVM(name)
	if vm == nil {
		return vmNotFound(name)
	}

	// capture VM:interface -> pcap
	num, err := strconv.Atoi(iface)
	if err != nil {
		return fmt.Errorf("invalid interface: `%v`", iface)
	}

	return ns.captures.CapturePcap(vm, num, fname)
}

func cliCaptureClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	return ns.captures.StopAll()
}

// cliCapturePcap manages the CLI for starting and stopping captures to pcap.
func cliCapturePcap(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]
	fname := c.StringArgs["filename"]

	if c.BoolArgs["delete"] {
		return ns.captures.StopBridge(b, "pcap")
	}

	return ns.captures.CapturePcapBridge(b, fname)
}

// cliCaptureNetflow manages the CLI for starting and stopping captures to netflow.
func cliCaptureNetflow(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]

	if c.BoolArgs["delete"] {
		return ns.captures.StopBridge(b, "netflow")
	} else if c.BoolArgs["timeout"] {
		// Set or get the netflow timeout
		timeout := c.StringArgs["timeout"]
		val, err := strconv.Atoi(timeout)
		if timeout != "" {
			resp.Response = strconv.Itoa(captureNFTimeout)
		} else if err != nil {
			return fmt.Errorf("invalid timeout parameter: `%v`", timeout)
		} else {
			captureNFTimeout = val
			updateNetflowTimeouts()
		}

		return nil
	} else if c.BoolArgs["file"] {
		// Capture -> netflow (file)
		return ns.captures.CaptureNetflowFile(
			b,
			c.StringArgs["filename"],
			c.BoolArgs["ascii"],
			c.BoolArgs["gzip"],
		)
	} else if c.BoolArgs["socket"] {
		// Capture -> netflow (socket)
		transport := "tcp"
		if c.BoolArgs["udp"] {
			transport = "udp"
		}

		return ns.captures.CaptureNetflowSocket(
			b,
			transport,
			c.StringArgs["hostname:port"],
			c.BoolArgs["ascii"],
		)
	}

	return errors.New("unreachable")
}
