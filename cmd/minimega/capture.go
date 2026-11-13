// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/gonetflow"
	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// captureHeader is the header of the capture listing. path stays last because
// some tools read it by position.
var captureHeader = []string{
	"bridge",
	"type",
	"interface",
	"mode",
	"compress",
	"id",
	"state",
	"format",
	"snaplen",
	"filter",
	"packets",
	"bytes",
	"kdrops",
	"size",
	"start",
	"error",
	"path",
}

type capture interface {
	Type() string
	Stop() error
	// Row returns the listing row of the capture with the given ID.
	Row(id int) []string
}

type netflowConfig struct {
	gonetflow.Mode // embed

	Compress bool
}

type pcapCapture struct {
	// Bridge that is doing the capture
	Bridge string

	// ID returned by bridge
	ID int

	// Path where PCAP is being written
	Path string
}

type pcapVMCapture struct {
	pcapCapture // embed

	VM        VM
	Interface int
}

type pcapBridgeCapture struct {
	pcapCapture // embed
}

type netflowCapture struct {
	netflowConfig // embed

	// Bridge that is doing the capture
	Bridge string

	// Path where netflow is being written (may be host:port)
	Path string
}

type captures struct {
	m map[int]capture

	counter *Counter // counter simply for keys in m, not used otherwise

	bridge.CaptureConfig // embed config for new pcap captures
	netflowConfig        // embed config for new netflow captures
}

// Active timeout for connections in seconds. Due to a limitation with Open
// vSwitch, this has to be shared across namespaces.
var captureNFTimeout = 10

// lookupBridge for tests
var lookupBridge = func(name string) (*bridge.Bridge, bool) {
	return bridges.Lookup(name)
}

func (c *pcapCapture) Type() string {
	return "pcap"
}

// Stop stops the capture. A missing bridge or capture means that the capture
// already ended, so Stop does not create the bridge again.
func (c *pcapCapture) Stop() error {
	br, ok := lookupBridge(c.Bridge)
	if !ok {
		return nil
	}

	if err := br.StopCapture(c.ID); err != nil {
		log.Debug("capture %v on %v already ended: %v", c.ID, c.Bridge, err)
	}

	return nil
}

// info returns the state of the capture from its bridge.
func (c *pcapCapture) info() bridge.CaptureInfo {
	if br, ok := lookupBridge(c.Bridge); ok {
		if info, ok := br.CaptureInfo(c.ID); ok {
			return info
		}
	}

	return bridge.CaptureInfo{
		Path:  c.Path,
		State: bridge.CaptureFailed,
		Error: fmt.Sprintf("bridge %v is gone", c.Bridge),
	}
}

func pcapRow(id int, br, iface string, info bridge.CaptureInfo) []string {
	var snaplen, start, packets, bytes, kdrops, size string

	if info.Config.SnapLen != 0 {
		snaplen = strconv.FormatUint(uint64(info.Config.SnapLen), 10)
	}
	if !info.Start.IsZero() {
		start = info.Start.UTC().Format(time.RFC3339)
		packets = strconv.FormatUint(info.Packets, 10)
		bytes = strconv.FormatUint(info.Bytes, 10)
		kdrops = strconv.FormatUint(info.KernelDrops, 10)
		size = strconv.FormatInt(info.Size, 10)
	}

	return []string{
		br,
		"pcap",
		iface,
		"", "",
		strconv.Itoa(id),
		string(info.State),
		info.Format,
		snaplen,
		info.Config.Filter,
		packets,
		bytes,
		kdrops,
		size,
		start,
		info.Error,
		info.Path,
	}
}

func (c *pcapVMCapture) Row(id int) []string {
	iface := fmt.Sprintf("%v:%v", c.VM.GetName(), c.Interface)

	return pcapRow(id, c.Bridge, iface, c.info())
}

func (c *pcapBridgeCapture) Row(id int) []string {
	return pcapRow(id, c.Bridge, "", c.info())
}

func (c *netflowCapture) Type() string {
	return "netflow"
}

func (c *netflowCapture) Row(id int) []string {
	row := make([]string, len(captureHeader))

	row[0] = c.Bridge
	row[1] = c.Type()
	row[3] = c.Mode.String()
	row[4] = strconv.FormatBool(c.Compress)
	row[5] = strconv.Itoa(id)
	row[6] = string(bridge.CaptureRunning)
	row[len(row)-1] = c.Path

	return row
}

func (c *netflowCapture) Stop() error {
	br, ok := lookupBridge(c.Bridge)
	if !ok {
		return nil
	}

	// get the netflow object associated with this bridge
	nf, err := br.GetNetflow()
	if err != nil {
		return err
	}

	if err := nf.RemoveWriter(c.Path); err != nil {
		return err
	}

	// we were the last writer -- clean up the netflow object
	if !nf.HasWriter() {
		return br.DestroyNetflow()
	}

	return nil
}

// Rows returns the listing rows of all captures, sorted by ID.
func (c *captures) Rows() [][]string {
	ids := make([]int, 0, len(c.m))
	for id := range c.m {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	res := [][]string{}
	for _, id := range ids {
		res = append(res, c.m[id].Row(id))
	}

	return res
}

// CaptureVM starts a new capture for a specified interface on a VM, writing
// the packets to the specified file in PCAP format.
func (c *captures) CaptureVM(vm VM, iface int, fname string, config bridge.CaptureConfig) (int, error) {
	nic, err := vm.GetNetwork(iface)
	if err != nil {
		return 0, err
	}

	if nic.VLAN == DisconnectedVLAN || nic.Bridge == "" {
		return 0, fmt.Errorf("interface %v of %v is disconnected", iface, vm.GetName())
	}

	br, err := getBridge(nic.Bridge)
	if err != nil {
		return 0, err
	}

	config.Description = fmt.Sprintf("%v interface %v, VLAN %v", vm.GetName(), iface, nic.VLAN)
	if nic.Alias != "" {
		config.Description = fmt.Sprintf("%v interface %v, VLAN %v (%v)", vm.GetName(), iface, nic.Alias, nic.VLAN)
	}
	config.Comment = fmt.Sprintf("minimega capture of VM %v in namespace %v", vm.GetName(), vm.GetNamespace())

	for _, ip := range []string{nic.IP4, nic.IP6} {
		if ip != "" {
			config.Names = append(config.Names, pcapfile.Name{Addr: ip, Names: []string{vm.GetName()}})
		}
	}

	id, err := br.CaptureTap(nic.Tap, fname, config)
	if err != nil {
		return 0, err
	}

	return c.addCapture(&pcapVMCapture{
		pcapCapture: pcapCapture{
			Bridge: nic.Bridge,
			Path:   fname,
			ID:     id,
		},
		VM:        vm,
		Interface: iface,
	}), nil
}

// CaptureBridge starts a new capture for all the traffic on the specified
// bridge, writing all packets to the specified filename in PCAP format.
func (c *captures) CaptureBridge(b, fname string, config bridge.CaptureConfig) (int, error) {
	br, err := getBridge(b)
	if err != nil {
		return 0, err
	}

	id, err := br.Capture(fname, config)
	if err != nil {
		return 0, err
	}

	return c.addCapture(&pcapBridgeCapture{
		pcapCapture: pcapCapture{
			Bridge: br.Name,
			Path:   fname,
			ID:     id,
		},
	}), nil
}

// CaptureNetflowFile starts a new netflow recorder for all the traffic on the
// specified bridge, writing the netflow records to the specified filename.
func (c *captures) CaptureNetflowFile(bridge, fname string) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	if err := nf.NewFileWriter(fname, c.Mode, c.Compress); err != nil {
		return err
	}

	c.addCapture(&netflowCapture{
		netflowConfig: c.netflowConfig,
		Bridge:        bridge,
		Path:          fname,
	})

	return nil
}

// CaptureNetflowSocket starts a new netflow recorder for all the traffic on
// the specified bridge, writing the netflow record across the network to a
// remote host.
func (c *captures) CaptureNetflowSocket(bridge, transport, host string) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	if err := nf.NewSocketWriter(transport, host, c.Mode); err != nil {
		return err
	}

	c.addCapture(&netflowCapture{
		netflowConfig: c.netflowConfig,
		Bridge:        bridge,
		Path:          fmt.Sprintf("%v:%v", transport, host),
	})

	return nil
}

// addCapture generates an ID for the capture, adds it to the map and returns
// the ID.
func (c *captures) addCapture(v capture) int {
	id := c.counter.Next()
	c.m[id] = v

	return id
}

// StopAll stops all captures.
func (c *captures) StopAll() error {
	return c.stop(func(_ capture) bool {
		return true
	})
}

// StopType stops all captures of one type, pcap or netflow.
func (c *captures) StopType(typ string) error {
	return c.stop(func(v capture) bool {
		return v.Type() == typ
	})
}

// StopID stops the pcap capture with the given ID.
func (c *captures) StopID(id int) error {
	v, ok := c.m[id]
	if !ok {
		return fmt.Errorf("no capture with ID %v", id)
	}

	if v.Type() != "pcap" {
		return fmt.Errorf("capture %v is not a pcap capture", id)
	}

	return c.stop(func(v2 capture) bool {
		return v2 == v
	})
}

// StopVM stops captures for a VM (wildcard supported for VM name). If iface
// is non-nil, only the capture for that interface index is stopped; otherwise
// all captures for the VM are stopped. vmExists reports whether a VM name is
// in the namespace, for the error when nothing matched.
func (c *captures) StopVM(s string, iface *int, vmExists func(string) bool) error {
	var found bool

	err := c.stop(func(v capture) bool {
		switch v := v.(type) {
		case *pcapVMCapture:
			r := v.VM.GetName() == s || s == Wildcard
			if iface != nil {
				r = r && v.Interface == *iface
			}
			found = r || found
			return r
		}

		return false
	})

	switch {
	case err != nil || found || s == Wildcard:
		return err
	case !vmExists(s):
		return vmNotFound(s)
	case iface != nil:
		return fmt.Errorf("no capture on interface %v of VM %v", *iface, s)
	default:
		return fmt.Errorf("no capture for VM %v", s)
	}
}

// StopBridge stops capture for bridge (wildcard supported).
func (c *captures) StopBridge(s, typ string) error {
	var found bool

	err := c.stop(func(v capture) bool {
		if v.Type() != typ {
			return false
		}

		var r bool

		switch v := v.(type) {
		case *pcapBridgeCapture:
			r = v.Bridge == s || s == Wildcard
		case *netflowCapture:
			r = v.Bridge == s || s == Wildcard
		}

		found = found || r
		return r
	})

	if err == nil && !found && s != Wildcard {
		return fmt.Errorf("no %v capture on bridge %v", typ, s)
	}

	return err
}

// stop stops all captures that fn returns true for. Captures are always
// removed from the map, even if stopping them failed, so that a single bad
// capture doesn't prevent the rest from being stopped or leave a permanently
// unstoppable entry behind. Any errors encountered are aggregated.
func (c *captures) stop(fn func(capture) bool) error {
	var errs []error

	for id, v := range c.m {
		if fn(v) {
			if err := v.Stop(); err != nil {
				errs = append(errs, err)
			}

			delete(c.m, id)
		}
	}

	return errors.Join(errs...)
}

// getOrCreateNetflow wraps calls to getBridge and getNetflowFromBridge,
// creating each, if needed.
//
// Note that we cannot have a separate netflow object per namespace because OVS
// doesn't support more than one netflow object per bridge.
func getOrCreateNetflow(b string) (*gonetflow.Netflow, error) {
	br, err := getBridge(b)
	if err != nil {
		return nil, err
	}

	nf, err := br.GetNetflow()
	if err == bridge.ErrNoNetflow {
		return br.NewNetflow(captureNFTimeout)
	}

	return nf, nil
}

// updateNetflowTimeouts updates the timeouts for all netflow objects
func updateNetflowTimeouts() {
	for _, b := range bridges.Names() {
		br, err := getBridge(b)
		if err != nil {
			log.Error("could not get bridge: %v", err)
			continue
		}

		err = br.SetNetflowTimeout(captureNFTimeout)
		if err != nil && err != bridge.ErrNoNetflow {
			log.Error("unable to update netflow timeout: %v", err)
		}
	}
}
