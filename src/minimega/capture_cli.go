// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"strconv"
)

var captureCLIHandlers = []minicli.Handler{
	{ // capture
		HelpShort: "capture experiment data",
		HelpLong: `
Capture experiment data including netflow and PCAP. Netflow capture obtains
netflow data from any local openvswitch switch, and can write to file, another
socket, or both. Netflow data can be written out in raw or ascii format, and
file output can be compressed on the fly. Multiple netflow writers can be
configured.

PCAP capture can be from a bridge or VM interface. No filters are applied, and
all data seen on that interface is captured to file.

For example, to capture netflow data on bridge mega_bridge to file in ascii
mode and with gzip compression:

	capture netflow mega_bridge file foo.netflow ascii gzip

You can change the active flow timeout with:

	capture netflow mega_bridge timeout <timeout>

With <timeout> in seconds.

To capture pcap on bridge 'foo' to file 'foo.pcap':

	capture pcap bridge foo foo.pcap

To capture pcap on VM 'foo' to file 'foo.pcap', using the 2nd interface on that
VM:

	capture pcap vm foo 0 foo.pcap

When run without arguments, capture prints all running captures. To stop a
capture, use the delete commands:

	capture netflow delete <id>
	capture pcap delete <id>

To stop all captures of a particular kind, replace id with "all". To stop all
capture of all types, use "clear capture".

Note: capture is not a namespace-aware command.`,
		Patterns: []string{
			"capture",

			"capture <netflow,>",
			"capture <netflow,> <timeout,> [timeout]",
			"capture <netflow,> <bridge,> <bridge>",
			"capture <netflow,> <bridge,> <bridge> <file,> <filename>",
			"capture <netflow,> <bridge,> <bridge> <file,> <filename> <raw,ascii> [gzip]",
			"capture <netflow,> <bridge,> <bridge> <socket,> <tcp,udp> <hostname:port> <raw,ascii>",
			"capture <netflow,> <delete,> <id or all>",

			"capture <pcap,>",
			"capture <pcap,> bridge <bridge> <filename>",
			"capture <pcap,> vm <vm id or name> <interface index> <filename>",
			"capture <pcap,> <delete,> <id or all>",
		},
		Call: wrapSimpleCLI(cliCapture),
	},
	{ // clear capture
		HelpShort: "reset capture state",
		HelpLong: `
Resets state for captures. See "help capture" for more information.`,
		Patterns: []string{
			"clear capture [netflow,pcap]",
		},
		Call: wrapSimpleCLI(cliCaptureClear),
	},
}

func cliCapture(c *minicli.Command) *minicli.Response {
	if c.BoolArgs["netflow"] {
		// Capture to netflow
		return cliCaptureNetflow(c)
	} else if c.BoolArgs["pcap"] {
		// Capture to pcap
		return cliCapturePcap(c)
	}

	resp := &minicli.Response{Host: hostname}

	// Print capture info
	resp.Header = []string{
		"ID",
		"Type",
		"Bridge",
		"VM/interface",
		"Path",
		"Mode",
		"Compress",
	}

	resp.Tabular = [][]string{}
	for _, v := range captureEntries {
		row := []string{
			strconv.Itoa(v.ID),
			v.Type,
			v.Bridge,
			fmt.Sprintf("%v/%v", v.VM, v.Interface),
			v.Path,
			v.Mode,
			strconv.FormatBool(v.Compress),
		}
		resp.Tabular = append(resp.Tabular, row)
	}

	// TODO: How does this fit in?
	//
	// get netflow stats for each bridge
	//var nfstats string
	//b := enumerateBridges()
	//for _, v := range b {
	//	nf, err := getNetflowFromBridge(v)
	//	if err != nil {
	//		if !strings.Contains(err.Error(), "has no netflow object") {
	//			return cliResponse{
	//				Error: err.Error(),
	//			}
	//		}
	//		continue
	//	}
	//	nfstats += fmt.Sprintf("Bridge %v:\n", v)
	//	nfstats += fmt.Sprintf("minimega listening on port: %v\n", nf.GetPort())
	//	nfstats += nf.GetStats()
	//}

	//out := o.String() + "\n" + nfstats

	return resp
}

func cliCaptureClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := clearAllCaptures()
	if err != nil {
		resp.Error = err.Error()
	}

	return resp

}

// cliCapturePcap manages the CLI for starting and stopping captures to pcap.
func cliCapturePcap(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	if c.BoolArgs["delete"] {
		// Stop a capture
		err = clearCapture("pcap", c.StringArgs["id"])
	} else if c.StringArgs["bridge"] != "" {
		// Capture bridge -> pcap
		err = startBridgeCapturePcap(c.StringArgs["bridge"], c.StringArgs["filename"])
	} else if c.StringArgs["vm"] != "" {
		// Capture VM:interface -> pcap
		var iface int
		iface, err = strconv.Atoi(c.StringArgs["interface"])
		if err != nil {
			err = fmt.Errorf("invalid interface: `%v`", c.StringArgs["interface"])
		} else {
			err = startCapturePcap(c.StringArgs["vm"], iface, c.StringArgs["filename"])
		}
	} else {
		// List captures
		resp.Header = []string{"ID", "Bridge", "VM/interface", "Path"}

		resp.Tabular = [][]string{}
		for _, v := range captureEntries {
			if v.Type == "pcap" {
				iface := fmt.Sprintf("%v/%v", v.VM, v.Interface)
				row := []string{
					strconv.Itoa(v.ID),
					v.Bridge,
					iface,
					v.Path,
				}
				resp.Tabular = append(resp.Tabular, row)
			}
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

// cliCaptureNetflow manages the CLI for starting and stopping captures to netflow.
func cliCaptureNetflow(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	if c.BoolArgs["delete"] {
		// Stop a capture
		err = clearCapture("netflow", c.StringArgs["id"])
	} else if c.BoolArgs["timeout"] {
		// Set or get the netflow timeout
		timeout := c.StringArgs["timeout"]
		val, err := strconv.Atoi(timeout)
		if timeout != "" {
			resp.Response = strconv.Itoa(captureNFTimeout)
		} else if err != nil {
			resp.Error = fmt.Sprintf("invalid timeout parameter: `%v`", timeout)
		} else {
			captureNFTimeout = val
			captureUpdateNFTimeouts()
		}
	} else if c.BoolArgs["file"] {
		// Capture -> netflow (file)
		err = startCaptureNetflowFile(
			c.StringArgs["bridge"],
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
		err = startCaptureNetflowSocket(
			c.StringArgs["bridge"],
			transport,
			c.StringArgs["hostname:port"],
			c.BoolArgs["ascii"],
		)
	} else {
		captureLock.Lock()
		defer captureLock.Unlock()

		// List captures
		resp.Header = []string{"ID", "Bridge", "Path", "Mode", "Compress"}

		for _, v := range captureEntries {
			if v.Type == "netflow" {
				row := []string{
					strconv.Itoa(v.ID),
					v.Bridge,
					v.Path,
					v.Mode,
					strconv.FormatBool(v.Compress),
				}
				resp.Tabular = append(resp.Tabular, row)
			}
		}

		// TODO: netflow stats?

	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
