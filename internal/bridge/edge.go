// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Port selections for CaptureConfig.Ports.
const (
	PortsAll  = "all"
	PortsEdge = "edge"
)

// reconcileInterval is how often an edge mirror is compared with the taps.
const reconcileInterval = 2 * time.Second

// EdgeSelector selects the edge ports of a namespace on a bridge: the ports of
// the VM, container and host taps that minimega made for the namespace.
// Tunnels, trunks and ports that minimega did not make are never edge ports.
type EdgeSelector struct {
	Namespace string
	VLANs     []int    // empty means all VLANs
	VMs       []string // empty means all VMs and host taps
}

func (s EdgeSelector) matchVLAN(vlan int) bool {
	if len(s.VLANs) == 0 {
		return true
	}

	for _, v := range s.VLANs {
		if v == vlan {
			return true
		}
	}

	return false
}

func (s EdgeSelector) matchOwner(owner string) bool {
	if len(s.VMs) == 0 {
		return true
	}

	for _, v := range s.VMs {
		if v == owner && owner != "" {
			return true
		}
	}

	return false
}

// edgeCapture is one edge mirror on a bridge.
type edgeCapture struct {
	sel    EdgeSelector
	mirror string          // mirror name
	ports  map[string]bool // OVS ports in select_src_port
	done   chan struct{}
}

// SetTapOwner records the namespace and the VM (empty for a host tap) that own
// tap, and adds tap to the matching edge mirrors.
func (b *Bridge) SetTapOwner(tap, namespace, vm string) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	t, ok := b.taps[tap]
	if !ok {
		return
	}

	t.Namespace = namespace
	t.Owner = vm

	b.edgeHook()
}

// edgePorts returns the edge ports that s selects, sorted. A bond member gives
// the bond port name once. Assumes bridgeLock is held.
func (b *Bridge) edgePorts(s EdgeSelector) []string {
	set := map[string]bool{}

	for _, t := range b.taps {
		switch {
		case t.Defunct, t.Namespace == "", t.Namespace != s.Namespace:
		case b.mirrors[t.Name]:
		case !s.matchVLAN(t.VLAN), !s.matchOwner(t.Owner):
		case t.Bond != "":
			set[t.Bond] = true
		default:
			set[t.Name] = true
		}
	}

	var res []string
	for k := range set {
		res = append(res, k)
	}
	sort.Strings(res)

	return res
}

// ovsAddEdgeMirror creates a mirror on bridge that sends the frames that the
// bridge receives from ports to dst. Assumes bridgeLock is held.
func ovsAddEdgeMirror(bridge, dst string, ports []string, vlans []int, tags MirrorTags) error {
	args := []string{"--", "--id=@out", "get", "Port", dst}

	var refs []string
	for i, p := range ports {
		id := "@s" + strconv.Itoa(i)
		refs = append(refs, id)
		args = append(args, "--", "--id="+id, "get", "Port", p)
	}

	args = append(args, "--", "--id=@m", "create", "Mirror", "name=mirror-"+dst, "output_port=@out")

	if len(refs) > 0 {
		args = append(args, "select_src_port="+strings.Join(refs, ","))
	}

	if len(vlans) > 0 {
		var vs []string
		for _, v := range vlans {
			vs = append(vs, strconv.Itoa(v))
		}
		args = append(args, "select_vlan="+strings.Join(vs, ","))
	}

	args = append(args, tags.args()...)
	args = append(args, "--", "add", "Bridge", bridge, "mirrors", "@m")

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("add edge mirror failed: %v", err)
	}

	return nil
}

// ovsMirrorPort adds port to, or removes it from, the select_src_port of
// mirror.
func ovsMirrorPort(mirror, port string, add bool) error {
	op := "remove"
	if add {
		op = "add"
	}

	args := []string{"--", "--id=@p", "get", "Port", port, "--", op, "Mirror", mirror, "select_src_port", "@p"}

	_, err := ovsCmdWrapper(args)
	return err
}

// createEdgeMirror creates the edge mirror for the capture tap dst and starts
// its reconcile loop. Assumes bridgeLock is held.
func (b *Bridge) createEdgeMirror(dst string, sel EdgeSelector, tags MirrorTags) error {
	if b.mirrors[dst] {
		return fmt.Errorf("destination tap is already a mirror")
	}

	if err := checkMirrorBudget(b.Name); err != nil {
		return err
	}

	ports := b.edgePorts(sel)

	if err := ovsAddEdgeMirror(b.Name, dst, ports, sel.VLANs, tags); err != nil {
		return err
	}

	ec := &edgeCapture{
		sel:    sel,
		mirror: "mirror-" + dst,
		ports:  map[string]bool{},
		done:   make(chan struct{}),
	}
	for _, p := range ports {
		ec.ports[p] = true
	}

	b.mirrors[dst] = true
	b.edgeCaptures[dst] = ec

	go b.reconcileLoop(ec)

	return nil
}

// destroyEdgeMirror stops the reconcile loop of the edge mirror of dst.
// Assumes bridgeLock is held.
func (b *Bridge) forgetEdgeMirror(dst string) {
	if ec, ok := b.edgeCaptures[dst]; ok {
		close(ec.done)
		delete(b.edgeCaptures, dst)
	}
}

// edgeHook applies the port changes of every edge mirror. It only runs
// ovs-vsctl, never calls back into minimega, and logs its errors, which the
// reconcile loop repairs. Assumes bridgeLock is held.
func (b *Bridge) edgeHook() {
	for _, ec := range b.edgeCaptures {
		b.reconcile(ec)
	}
}

// reconcile makes the select_src_port of ec match its selector. Assumes
// bridgeLock is held.
func (b *Bridge) reconcile(ec *edgeCapture) {
	want := map[string]bool{}
	for _, p := range b.edgePorts(ec.sel) {
		want[p] = true

		if !ec.ports[p] {
			if err := ovsMirrorPort(ec.mirror, p, true); err != nil {
				log.Warn("unable to add %v to %v: %v", p, ec.mirror, err)
				continue
			}
			ec.ports[p] = true
		}
	}

	for p := range ec.ports {
		if want[p] {
			continue
		}

		// a deleted port leaves the mirror on its own; ignore errors for it
		if err := ovsMirrorPort(ec.mirror, p, false); err != nil {
			log.Debug("unable to remove %v from %v: %v", p, ec.mirror, err)
		}
		delete(ec.ports, p)
	}
}

func (b *Bridge) reconcileLoop(ec *edgeCapture) {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()

	for {
		select {
		case <-ec.done:
			return
		case <-t.C:
		}

		bridgeLock.Lock()
		if !b.destroyed() {
			select {
			case <-ec.done:
			default:
				b.reconcile(ec)
			}
		}
		bridgeLock.Unlock()
	}
}

// EdgePorts returns the edge ports that s selects on the bridge.
func (b *Bridge) EdgePorts(s EdgeSelector) []string {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.edgePorts(s)
}

// EdgeTaps returns copies of the taps that s selects, bond members included,
// sorted by name.
func (b *Bridge) EdgeTaps(s EdgeSelector) []Tap {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	var res []Tap

	for _, t := range b.taps {
		switch {
		case t.Defunct, t.Namespace == "", t.Namespace != s.Namespace:
		case b.mirrors[t.Name]:
		case !s.matchVLAN(t.VLAN), !s.matchOwner(t.Owner):
		default:
			v := *t
			v.qos = nil
			v.stats = nil
			res = append(res, v)
		}
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })

	return res
}
