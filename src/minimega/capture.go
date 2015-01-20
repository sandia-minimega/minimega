// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"gonetflow"
	"gopcap"
	"minicli"
	log "minilog"
	"strconv"
	"strings"
	"sync"
)

type capture struct {
	ID        int
	Type      string
	Bridge    string
	VM        int
	Interface int
	Path      string
	Mode      string
	Compress  bool
	tap       string
	pcap      *gopcap.Pcap
}

var (
	captureEntries   map[int]*capture
	captureIDCount   chan int
	captureLock      sync.Mutex
	captureNFTimeout int
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

	minimega$ capture netflow mega_bridge file foo.netflow ascii gzip

You can change the active flow timeout with:

	minimega$ capture netflow mega_bridge timeout <timeout>

With <timeout> in seconds.

To capture pcap on bridge 'foo' to file 'foo.pcap':

	minimega$ capture pcap bridge foo foo.pcap

To capture pcap on VM 'foo' to file 'foo.pcap', using the 2nd interface on that
VM:

	minimega$ capture pcap vm foo 0 foo.pcap`,
		Patterns: []string{
			"capture",
			"clear capture [netflow,pcap]",

			"capture <netflow,>",
			"capture <netflow,> <timeout,> [timeout]",
			"capture <netflow,> <bridge>",
			"capture <netflow,> <bridge> <file,> <filename>",
			"capture <netflow,> <bridge> <file,> <filename> <raw,ascii> [gzip]",
			"capture <netflow,> <bridge> <socket,> <tcp,udp> <hostname:port> <raw,ascii>",
			"capture <netflow,> <delete,> <id or *>",

			"capture <pcap,>",
			"capture <pcap,> bridge <bridge> <filename>",
			"capture <pcap,> vm <vm id or name> <interface index> <filename>",
			"capture <pcap,> <delete,> <id or *>",
		},
		Call: wrapSimpleCLI(cliCapture),
	},
}

func init() {
	registerHandlers("capture", captureCLIHandlers)

	captureNFTimeout = 10
	captureEntries = make(map[int]*capture)
	captureIDCount = make(chan int)
	count := 0
	go func() {
		for {
			captureIDCount <- count
			count++
		}
	}()
}

func cliCapture(c *minicli.Command) *minicli.Response {
	if isClearCommand(c) {
		resp := &minicli.Response{Host: hostname}
		if err := clearAllCaptures(); err != nil {
			resp.Error = err.Error()
		}
		return resp
	} else if c.BoolArgs["netflow"] {
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

func clearAllCaptures() (err error) {
	err = clearCapture("netflow", "*")
	if err == nil {
		err = clearCapture("pcap", "*")
	}

	return
}

func clearCapture(captureType, id string) (err error) {
	captureLock.Lock()
	defer captureLock.Unlock()

	defer func() {
		// check if we need to remove the nf object
		if err != nil && captureType == "netflow" {
			err = cleanupNetflow()
		}
	}()

	if id == "*" {
		for _, v := range captureEntries {
			if v.Type == "pcap" && captureType == "pcap" {
				return stopPcapCapture(v)
			} else if v.Type == "netflow" && captureType == "netflow" {
				return stopNetflowCapture(v)
			}
		}
	} else {
		val, err := strconv.Atoi(id)
		if err != nil {
			return err
		}

		entry, ok := captureEntries[val]
		if !ok {
			return fmt.Errorf("entry %v does not exist", val)
		}

		if entry.Type != captureType {
			return fmt.Errorf("invalid id/capture type, `%s` != `%s`", entry.Type, captureType)
		} else if entry.Type == "pcap" {
			return stopPcapCapture(captureEntries[val])
		} else if entry.Type == "netflow" {
			return stopNetflowCapture(captureEntries[val])
		}
	}

	return nil
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

// startCapturePcap starts a new capture for a specified interface on a VM,
// writing the packets to the specified filename in PCAP format.
func startCapturePcap(vm string, iface int, filename string) error {
	// get the vm
	v := vms.getVM(vm)
	if v == nil {
		return fmt.Errorf("no such vm %v", vm)
	}

	if len(v.taps) <= iface {
		return fmt.Errorf("no such interface %v", iface)
	}

	tap := v.taps[iface]

	// attempt to start pcap on the bridge
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        <-captureIDCount,
		Type:      "pcap",
		VM:        v.Id,
		Interface: iface,
		Path:      filename,
		pcap:      p,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// startBridgeCapturePcap starts a new capture for all the traffic on the
// specified bridge, writing all packets to the specified filename in PCAP
// format.
func startBridgeCapturePcap(bridge, filename string) error {
	// create the bridge if necessary
	b, err := getBridge(bridge)
	if err != nil {
		return err
	}

	tap, err := b.CreateBridgeMirror()
	if err != nil {
		return err
	}

	// attempt to start pcap on the mirrored tap
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        <-captureIDCount,
		Type:      "pcap",
		Bridge:    bridge,
		VM:        -1,
		Interface: -1,
		Path:      filename,
		pcap:      p,
		tap:       tap,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// startCaptureNetflowFile starts a new netflow recorder for all the traffic on
// the specified bridge, writing the netflow records to the specified filename.
// Flags control whether the format is raw or ascii, and whether the logs will
// be compressed.
func startCaptureNetflowFile(bridge, filename string, ascii, compress bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	modeStr := "raw"
	if ascii {
		mode = gonetflow.ASCII
		modeStr = "ascii"
	}

	err = nf.NewFileWriter(filename, mode, compress)
	if err != nil {
		return err
	}

	ce := &capture{
		ID:       <-captureIDCount,
		Type:     "netflow",
		Bridge:   bridge,
		Path:     filename,
		Mode:     modeStr,
		Compress: compress,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// startCaptureNetflowSocket starts a new netflow recorder for all the traffic
// on the specified bridge, writing the netflow record across the network to a
// remote host. The ascii flag controls whether the record format is raw or
// ascii.
func startCaptureNetflowSocket(bridge, transport, host string, ascii bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	modeStr := "raw"
	if ascii {
		mode = gonetflow.ASCII
		modeStr = "ascii"
	}

	err = nf.NewSocketWriter(transport, host, mode)
	if err != nil {
		return err
	}

	ce := &capture{
		ID:     <-captureIDCount,
		Type:   "netflow",
		Bridge: bridge,
		Path:   fmt.Sprintf("%v:%v", transport, host),
		Mode:   modeStr,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// stopPcapCapture stops the specified pcap capture. Assumes that captureLock
// has been acquired by the caller.
func stopPcapCapture(entry *capture) error {
	if entry.Type != "pcap" {
		return errors.New("called stop pcap capture on capture of wrong type")
	}

	delete(captureEntries, entry.ID)

	if entry.pcap == nil {
		return fmt.Errorf("capture %v has no valid pcap interface", entry.ID)
	}
	entry.pcap.Close()

	if entry.tap != "" && entry.Bridge != "" {
		b, err := getBridge(entry.Bridge)
		if err != nil {
			return err
		}

		return b.DeleteBridgeMirror(entry.tap)
	}

	return nil
}

// stopNetflowCapture stops the specified netflow capture. Assumes that
// captureLock has been acquired by the caller.
func stopNetflowCapture(entry *capture) error {
	if entry.Type != "netflow" {
		return errors.New("called stop netflow capture on capture of wrong type")
	}

	delete(captureEntries, entry.ID)

	// get the netflow object associated with this bridge
	nf, err := getNetflowFromBridge(entry.Bridge)
	if err != nil {
		return err
	}

	return nf.RemoveWriter(entry.Path)
}

// cleanupNetflow destroys any netflow objects that are not currently
// capturing. This should be invoked after calling stopNetflowCapture.
func cleanupNetflow() error {
	b := enumerateBridges()
	for _, v := range b {
		empty := true
		for _, n := range captureEntries {
			if n.Bridge == v {
				empty = false
				break
			}
		}

		if !empty {
			continue
		}

		b, err := getBridge(v)
		if err != nil {
			return err
		}

		err = b.DestroyNetflow()
		if err != nil {
			if !strings.Contains(err.Error(), "has no netflow object") {
				return err
			}
		}
	}

	return nil
}

// getOrCreateNetflow wraps calls to getBridge and getNetflowFromBridge,
// creating each, if needed.
func getOrCreateNetflow(bridge string) (*gonetflow.Netflow, error) {
	// create the bridge if necessary
	b, err := getBridge(bridge)
	if err != nil {
		return nil, err
	}

	nf, err := getNetflowFromBridge(bridge)
	if err != nil && !strings.Contains(err.Error(), "has no netflow object") {
		return nil, err
	}

	if nf == nil {
		// create a new netflow object
		nf, err = b.NewNetflow(captureNFTimeout)
	}

	return nf, err
}

func captureUpdateNFTimeouts() {
	b := enumerateBridges()
	for _, v := range b {
		br, err := getBridge(v)
		if err != nil {
			log.Error("could not get bridge: %v", err)
			continue
		}
		_, err = getNetflowFromBridge(v)
		if err != nil {
			if !strings.Contains(err.Error(), "has no netflow object") {
				log.Error("get netflow object from bridge: %v", err)
			}
			continue
		}
		err = br.UpdateNFTimeout(captureNFTimeout)
		if err != nil {
			log.Error("update netflow timeout: %v", err)
		}
	}
}
