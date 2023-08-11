// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/sandia-minimega/minimega/v2/internal/gonetflow"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
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
			"capture <pcap,> vm <vm name> <interface index> <filename>",
			"capture <pcap,> <delete,> vm <vm name>",
		},
		Call:    wrapVMTargetCLI(cliCaptureVM),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
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
			"capture <netflow,> <delete,> bridge <bridge>",
			"capture <netflow,> <timeout,> [timeout in seconds]",
			"capture <pcap,> bridge <bridge> <filename>",
			"capture <pcap,> <delete,> bridge <bridge>",
		},
		Call: wrapSimpleCLI(cliCapture),
		Suggest: wrapSuggest(func(ns *Namespace, val, prefix string) []string {
			if val == "bridge" {
				return cliBridgeSuggest(ns, prefix)
			}
			return nil
		}),
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

	return unreachable()
}

func cliCaptureConfig(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["snaplen"] {
		if v, ok := c.StringArgs["size"]; ok {
			i, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return err
			}

			ns.captures.SnapLen = uint32(i)
			return nil
		}

		resp.Response = strconv.FormatUint(uint64(ns.captures.SnapLen), 10)
		return nil
	} else if c.BoolArgs["filter"] {
		if v, ok := c.StringArgs["bpf"]; ok {
			// TODO: check syntax?
			ns.captures.Filter = v
			return nil
		}

		resp.Response = ns.captures.Filter
		return nil
	} else if c.BoolArgs["mode"] {
		if c.BoolArgs["raw"] {
			ns.captures.Mode = gonetflow.RAW
			return nil
		} else if c.BoolArgs["ascii"] {
			ns.captures.Mode = gonetflow.ASCII
			return nil
		}

		resp.Response = ns.captures.Mode.String()
		return nil
	} else if c.BoolArgs["gzip"] {
		if c.BoolArgs["true"] || c.BoolArgs["false"] {
			ns.captures.Compress = c.BoolArgs["true"]
			return nil
		}

		resp.Response = strconv.FormatBool(ns.captures.Compress)
		return nil
	}

	return unreachable()
}

func cliCaptureList(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"bridge",
		"type",
		"interface",
		"mode",
		"compress",
		"path",
	}

	resp.Tabular = [][]string{}

	for _, v := range ns.captures.m {
		var row []string

		switch v := v.(type) {
		case *pcapVMCapture:
			row = []string{
				v.Bridge,
				v.Type(),
				fmt.Sprintf("%v:%v", v.VM.GetName(), v.Interface),
				"", "",
				v.Path,
			}
		case *pcapBridgeCapture:
			row = []string{
				v.Bridge,
				v.Type(),
				"", "", "",
				v.Path,
			}
		case *netflowCapture:
			row = []string{
				v.Bridge,
				v.Type(),
				"",
				v.Mode.String(),
				strconv.FormatBool(v.Compress),
				v.Path,
			}
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliCaptureVM(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["vm"]
	iface := c.StringArgs["interface"]
	fname := c.StringArgs["filename"]

	// stopping capture for one or all VMs
	if c.BoolArgs["delete"] {
		return ns.captures.StopVM(name)
	}

	// capture VM:interface -> pcap
	num, err := strconv.Atoi(iface)
	if err != nil {
		return fmt.Errorf("invalid interface: `%v`", iface)
	}

	vm := ns.FindVM(name)
	if vm == nil {
		return vmNotFound(name)
	}

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(fname) {
		// TODO: should we capture to the VM directory instead?
		fname = filepath.Join(*f_iomBase, fname)
	}

	return ns.captures.CaptureVM(vm, num, fname)
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

	return ns.captures.CaptureBridge(b, fname)
}

// cliCaptureNetflow manages the CLI for starting and stopping captures to netflow.
func cliCaptureNetflow(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]

	switch {
	case c.BoolArgs["bridge"]:
		// start capture to file to socket
		if fname, ok := c.StringArgs["filename"]; ok {

			// Ensure that relative paths are always relative to /files/
			if !filepath.IsAbs(fname) {
				fname = filepath.Join(*f_iomBase, fname)
			}

			return ns.captures.CaptureNetflowFile(b, fname)
		}

		// Capture -> netflow (socket)
		transport := "tcp"
		if c.BoolArgs["udp"] {
			transport = "udp"
		}

		host := c.StringArgs["hostname:port"]

		return ns.captures.CaptureNetflowSocket(b, transport, host)
	case c.BoolArgs["delete"]:
		// delete capture
		return ns.captures.StopBridge(c.StringArgs["bridge"], "netflow")
	case c.BoolArgs["timeout"]:
		// Set or get the netflow timeout
		if v, ok := c.StringArgs["timeout"]; ok {
			i, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return err
			}

			captureNFTimeout = int(i)

			updateNetflowTimeouts()

			return nil
		}

		resp.Response = strconv.Itoa(captureNFTimeout)
		return nil
	default:
		return unreachable()
	}
}
