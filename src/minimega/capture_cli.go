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
			"capture <netflow,> <delete,> bridge <name>",

			"capture <pcap,> bridge <bridge> <filename>",
			"capture <pcap,> <delete,> bridge <name>",
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

func cliCapture(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["netflow"] {
		// Capture to netflow
		return cliCaptureNetflow(c, resp)
	} else if c.BoolArgs["pcap"] {
		// Capture to pcap
		return cliCapturePcap(c, resp)
	}

	return errors.New("unreachable")
}

func cliCaptureList(c *minicli.Command, resp *minicli.Response) error {
	namespace := GetNamespaceName()

	resp.Header = []string{"Bridge"}

	if !c.BoolArgs["netflow"] && !c.BoolArgs["pcap"] {
		resp.Header = append(resp.Header, "Type")
	}

	if !c.BoolArgs["netflow"] {
		resp.Header = append(resp.Header, "VM/interface")
	}
	if !c.BoolArgs["pcap"] {
		resp.Header = append(resp.Header, "Mode", "Compress")
	}

	resp.Header = append(resp.Header, "Path")

	if namespace == "" {
		resp.Header = append(resp.Header, "Namespace")
	}

	resp.Tabular = [][]string{}
	for _, v := range captureEntries {
		if !v.InNamespace(namespace) {
			continue
		}

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

		if namespace == "" {
			if v.VM != nil {
				row = append(row, v.VM.GetNamespace())
			} else {
				row = append(row, "N/A")
			}
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil

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
}

func cliCaptureVM(c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["name"]
	if c.BoolArgs["delete"] {
		// stop a capture
		return clearCapture("pcap", "vm", name)
	}

	fname := c.StringArgs["filename"]
	iface := c.StringArgs["interface"]

	// Capture VM:interface -> pcap
	num, err := strconv.Atoi(iface)
	if err != nil {
		return fmt.Errorf("invalid interface: `%v`", iface)
	}

	return startCapturePcap(name, num, fname)
}

func cliCaptureClear(c *minicli.Command, resp *minicli.Response) error {
	return clearAllCaptures()
}

// cliCapturePcap manages the CLI for starting and stopping captures to pcap.
func cliCapturePcap(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["delete"] {
		// Stop a capture
		return clearCapture("pcap", "bridge", c.StringArgs["name"])
	} else if c.StringArgs["bridge"] != "" {
		// Capture bridge -> pcap
		return startBridgeCapturePcap(c.StringArgs["bridge"], c.StringArgs["filename"])
	}

	return errors.New("unreachable")
}

// cliCaptureNetflow manages the CLI for starting and stopping captures to netflow.
func cliCaptureNetflow(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["delete"] {
		// Stop a capture
		return clearCapture("netflow", "bridge", c.StringArgs["name"])
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
			captureUpdateNFTimeouts()
		}

		return nil
	} else if c.BoolArgs["file"] {
		// Capture -> netflow (file)
		return startCaptureNetflowFile(
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

		return startCaptureNetflowSocket(
			c.StringArgs["bridge"],
			transport,
			c.StringArgs["hostname:port"],
			c.BoolArgs["ascii"],
		)
	}

	return errors.New("unreachable")
}
