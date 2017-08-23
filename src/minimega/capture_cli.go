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
		},
		Call: wrapBroadcastCLI(cliCaptureList),
	},
	{ // capture config
		HelpShort: "configure captures",
		Patterns: []string{
			"capture <pcap,> <snaplen,> [size]",
			"capture <pcap,> <filter,> [bpf]",
			"capture <netflow,> <mode,> [raw,ascii]",
			"capture <netflow,> <gzip,> [true,false]",
		},
		Call: wrapBroadcastCLI(cliCaptureConfig),
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
caution. See notes below.

Capture experiment data including netflow and PCAP. Netflow capture obtains
netflow data from any local openvswitch switch, and can write to file, another
socket, or both. Netflow data can be written out in raw or ascii format, and
file output can be compressed on the fly. Multiple netflow writers can be
configured. There are several APIs to configure new netflow captures:

	capture netflow mode [raw,ascii]
	capture netflow gzip [true,false]
	capture netflow timeout [timeout]

PCAP capture can be from a bridge or VM interface. To set the snaplen or filter
for new PCAP captures, use:

	capture pcap snaplen <size>
	capture pcap filter <bpf>

Examples:

	# Capture netflow for mega_bridge to foo.netflow
	capture netflow bridge mega_bridge foo.netflow

	# Capture all bridge foo traffic to foo.pcap
	capture pcap bridge foo foo.pcap

	# Capture the 0-th interface for VM foo to foo.pcap
	capture pcap vm foo 0 foo.pcap

When run without arguments, capture prints all running captures. To stop a
capture, use the delete commands:

	capture netflow delete bridge <bridge>
	capture pcap delete bridge <bridge>
	capture pcap delete vm <name>

To stop all captures of a particular kind, replace <bridge> or <vm> with "all".
If a VM has multiple interfaces and there are multiple captures running,
calling "capture pcap delete vm <name>" stops all the captures for that VM. To
stop all captures of all types, use "clear capture".

Notes with namespaces:
 * Capturing traffic directly from the bridge (as PCAP or netflow) is not
   recommended if different namespaces share the same bridge. If this is the
   case, the captured traffic would contain data from across namespaces.
 * Due to the way Open vSwitch implements netflow, there can be only one
   netflow object per bridge. This means that the netflow timeout is shared
   across namespaces. Additionally, note that the API is also not
   bridge-specific.

Due to the above intricacies, the following commands only run on the local
minimega instance:

	capture <netflow,> <bridge,> <bridge> <filename>
	capture <netflow,> <bridge,> <bridge> <tcp,udp> <hostname:port>
	capture <netflow,> <delete,> bridge <name>
	capture <netflow,> <timeout,> [timeout in seconds]
	capture <pcap,> bridge <bridge> <filename>
	capture <pcap,> <delete,> bridge <name>

`,
		Patterns: []string{
			"capture <netflow,> <bridge,> <bridge> <filename>",
			"capture <netflow,> <bridge,> <bridge> <tcp,udp> <hostname:port>",
			"capture <netflow,> <delete,> bridge <name>",
			"capture <netflow,> <timeout,> [timeout in seconds]",

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

func cliCaptureConfig(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["snaplen"] {
		if v, ok := c.StringArgs["size"]; ok {
			i, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return err
			}

			captureConfig.SnapLen = i
			return nil
		}

		resp.Response = strconv.FormatUint(captureConfig.SnapLen, 10)
		return nil
	} else if c.BoolArgs["filter"] {
		if v, ok := c.StringArgs["bpf"]; ok {
			// TODO: check syntax?
			captureConfig.Filter = v
			return nil
		}

		resp.Response = captureConfig.Filter
		return nil
	} else if c.BoolArgs["mode"] {
		if c.BoolArgs["raw"] {
			captureConfig.Mode = "raw"
			return nil
		} else if c.BoolArgs["ascii"] {
			captureConfig.Mode = "ascii"
			return nil
		}

		resp.Response = captureConfig.Mode
		return nil
	} else if c.BoolArgs["gzip"] {
		if c.BoolArgs["true"] || c.BoolArgs["false"] {
			captureConfig.Compress = c.BoolArgs["true"]
			return nil
		}

		resp.Response = strconv.FormatBool(captureConfig.Compress)
		return nil
	}

	return errors.New("unreachable")
}

func cliCaptureList(c *minicli.Command, resp *minicli.Response) error {
	namespace := GetNamespaceName()

	resp.Header = []string{"Bridge"}

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

	if namespace == "" {
		resp.Header = append(resp.Header, "namespace")
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
	} else if c.StringArgs["filename"] != "" {
		// Capture -> netflow (file)
		return startCaptureNetflowFile(
			c.StringArgs["bridge"],
			c.StringArgs["filename"],
			captureConfig.Mode == "ascii",
			captureConfig.Compress,
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
			captureConfig.Mode == "ascii",
		)
	} else if c.BoolArgs["timeout"] {
		if v, ok := c.StringArgs["timeout"]; ok {
			i, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return err
			}

			captureNFTimeout = int(i)

			captureUpdateNFTimeouts()

			return nil
		}

		resp.Response = strconv.Itoa(captureNFTimeout)
		return nil
	}

	return errors.New("unreachable")
}
