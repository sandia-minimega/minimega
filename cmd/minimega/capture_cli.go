// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
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
			"capture <pcap,> <format,> [pcap or pcapng]",
			"capture <netflow,> <mode,> [raw,ascii]",
			"capture <netflow,> <gzip,> [true,false]",
		},
		Call: wrapBroadcastCLI(cliCaptureConfig),
	},
	{ // capture for VM
		HelpShort: "capture experiment data for a VM",
		Patterns: []string{
			"capture <pcap,> vm <vm name> <interface index> <filename> [option]...",
			"capture <pcap,> <delete,> vm <vm name> [interface index]",
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

PCAP capture can be from a bridge or VM interface. By default, PCAP files use
the classic pcap format with nanosecond timestamps. To set the snaplen, filter
or file format for new PCAP captures in the namespace, use:

	capture pcap snaplen <size>
	capture pcap filter <bpf>
	capture pcap format <pcap or pcapng>

A pcapng file also records the interface, its description, the filter, the
snaplen, the drop counts and the known VM addresses.

The snaplen must be from 14 to 262144. minimega compiles the filter to check
it. Give an empty filter, "", to remove the filter.

You can give options as key=value items after the file name. The options
override the namespace settings for one capture:

	snaplen=<size>      bytes kept for each packet, 14 to 262144
	filter=<bpf>        BPF filter for this capture
	maxsize=<size>      stop the capture as full when the file reaches this
	                    size, in bytes, or with a K, M or G suffix
	format=<format>     file format: pcap or pcapng
	ports=<ports>       bridge captures only: all (the default) mirrors all
	                    frames on the bridge. edge mirrors only the frames that
	                    the bridge receives from the VM, container and host taps
	                    of the active namespace, also taps that start later.
	vlans=<vlans>       bridge captures only: limit the mirror to these VLANs,
	                    as a comma-separated list of aliases or IDs
	vms=<vms>           bridge captures with ports=edge only: limit the mirror
	                    to the taps of these VMs, as a comma-separated list

With ports=edge, a bridge capture records each frame once, where it enters the
bridge. It does not record frames from trunks, tunnels or other ports that
minimega did not make.
	direction=<dir>     VM captures only: in, out or both (the default). in
	                    captures the frames that the host receives on the tap,
	                    which are the frames that the VM sends.

Put a filter that contains spaces in quotes, for example filter="tcp port 80".

minimega makes the directory of the file. A relative file name is relative to
the -filepath directory. minimega refuses a file that another running capture
writes, except for files that are not regular files, such as /dev/null.

Examples:

	# Capture netflow for mega_bridge to foo.netflow
	capture netflow bridge mega_bridge foo.netflow

	# Capture all bridge foo traffic to foo.pcap
	capture pcap bridge foo foo.pcap

	# Capture the 0-th interface for VM foo to foo.pcap
	capture pcap vm foo 0 foo.pcap

	# Capture only the TCP port 80 frames that VM foo sends, at most 1 GiB
	capture pcap vm foo 0 foo.pcap filter="tcp port 80" direction=in maxsize=1G

When run without arguments, capture prints all captures. Each capture has an
ID and a state. The states are running, failed and full. A failed capture shows
its error, for example when its VM is flushed. minimega closes the file of a
failed or full capture, and keeps the capture in the list until you delete it.
The list also shows the format, snaplen, filter, packets, bytes, kernel drops
(kdrops), file size and start time of each PCAP capture. The path is always
the last column. To stop a capture, use the delete commands:

	capture netflow delete bridge <bridge>
	capture pcap delete bridge <bridge>
	capture pcap delete vm <name>
	capture pcap delete vm <name> <interface index>
	capture pcap delete id <id>

To stop all captures of a particular kind, replace <bridge> or <vm> with "all".
If a VM has multiple interfaces and there are multiple captures running,
calling "capture pcap delete vm <name>" stops all the captures for that VM. To
stop only the capture for a single interface, specify the interface index,
e.g. "capture pcap delete vm <name> 0" stops the capture for interface 0
without affecting captures on other interfaces of the same VM. A delete that
matches no capture returns an error. Capture IDs are local to each host. To
stop all captures of all types, use "clear capture". To stop all captures of
one type, use "clear capture pcap" or "clear capture netflow".

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
	capture <pcap,> bridge <bridge> <filename> [option]...
	capture <pcap,> <delete,> bridge <name>
	capture <pcap,> <delete,> id <id>
`,
		Patterns: []string{
			"capture <netflow,> <bridge,> <bridge> <filename>",
			"capture <netflow,> <bridge,> <bridge> <tcp,udp> <hostname:port>",
			"capture <netflow,> <delete,> bridge <bridge>",
			"capture <netflow,> <timeout,> [timeout in seconds]",
			"capture <pcap,> bridge <bridge> <filename> [option]...",
			"capture <pcap,> <delete,> bridge <bridge>",
			"capture <pcap,> <delete,> id <id>",
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
Resets state for captures across the namespace. With netflow or pcap, stops
only the captures of that type. See "help capture" for more information.`,
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
				return fmt.Errorf("invalid snaplen %q", v)
			}
			if err := bridge.ValidateSnapLen(uint32(i)); err != nil {
				return err
			}

			ns.captures.SnapLen = uint32(i)
			return nil
		}

		resp.Response = strconv.FormatUint(uint64(ns.captures.SnapLen), 10)
		return nil
	} else if c.BoolArgs["filter"] {
		if v, ok := c.StringArgs["bpf"]; ok {
			snaplen := ns.captures.SnapLen
			if snaplen == 0 {
				snaplen = bridge.DefaultSnapLen
			}
			if err := bridge.ValidateFilter(v, snaplen); err != nil {
				return err
			}

			ns.captures.Filter = v
			return nil
		}

		resp.Response = ns.captures.Filter
		return nil
	} else if c.BoolArgs["format"] {
		if v, ok := c.StringArgs["pcap"]; ok {
			if err := bridge.ValidateFormat(v); err != nil {
				return err
			}

			ns.captures.Format = v
			return nil
		}

		resp.Response = ns.captures.Format
		if resp.Response == "" {
			resp.Response = "pcap"
		}
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
	resp.Header = captureHeader
	resp.Tabular = ns.captures.Rows()

	return nil
}

func cliCaptureVM(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["vm"]
	iface := c.StringArgs["interface"]
	fname := c.StringArgs["filename"]

	vmExists := func(s string) bool { return ns.FindVM(s) != nil }

	// stopping capture for one or all VMs, optionally scoped to a single
	// interface index
	if c.BoolArgs["delete"] {
		if iface != "" {
			num, err := strconv.Atoi(iface)
			if err != nil {
				return fmt.Errorf("invalid interface: `%v`", iface)
			}

			return ns.captures.StopVM(name, &num, vmExists)
		}

		return ns.captures.StopVM(name, nil, vmExists)
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

	if err := checkCaptureFilename(fname); err != nil {
		return err
	}

	config, err := parseCaptureOptions(captureKindVM, ns.Name, ns.captures.CaptureConfig, c.ListArgs["option"])
	if err != nil {
		return err
	}

	_, err = captureVM(&ns.captures, vm, num, capturePath(fname), config)
	return err
}

func cliCaptureClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["pcap"]:
		return ns.captures.StopType("pcap")
	case c.BoolArgs["netflow"]:
		return ns.captures.StopType("netflow")
	}

	return ns.captures.StopAll()
}

// cliCapturePcap manages the CLI for starting and stopping captures to pcap.
func cliCapturePcap(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]
	fname := c.StringArgs["filename"]

	if c.BoolArgs["delete"] {
		if v, ok := c.StringArgs["id"]; ok {
			id, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid capture ID: `%v`", v)
			}

			return ns.captures.StopID(id)
		}

		return ns.captures.StopBridge(b, "pcap")
	}

	if err := checkCaptureFilename(fname); err != nil {
		return err
	}

	config, err := parseCaptureOptions(captureKindBridge, ns.Name, ns.captures.CaptureConfig, c.ListArgs["option"])
	if err != nil {
		return err
	}

	_, err = captureBridge(&ns.captures, b, capturePath(fname), config)
	return err
}

// cliCaptureNetflow manages the CLI for starting and stopping captures to netflow.
func cliCaptureNetflow(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]

	switch {
	case c.BoolArgs["bridge"]:
		// start capture to file to socket
		if fname, ok := c.StringArgs["filename"]; ok {
			return captureNetflowFile(&ns.captures, b, capturePath(fname))
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

// capturePath returns where a capture to fname is written: relative paths are
// relative to the files directory.
func capturePath(fname string) string {
	if filepath.IsAbs(fname) {
		return fname
	}

	return filepath.Join(*f_iomBase, fname)
}

// The handlers start captures through these so tests can observe the path
// without a real bridge.
var (
	captureVM          = (*captures).CaptureVM
	captureBridge      = (*captures).CaptureBridge
	captureNetflowFile = (*captures).CaptureNetflowFile
)
