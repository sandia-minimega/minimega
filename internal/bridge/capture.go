// Copyright 2016-2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// for tests
var openCaptureSource = openSource

// disableIPv6 stops the host from sending IPv6 router solicitations and DAD
// frames on a capture tap. Tests replace it.
var disableIPv6 = func(tap string) error {
	p := filepath.Join("/proc/sys/net/ipv6/conf", tap, "disable_ipv6")

	err := os.WriteFile(p, []byte("1"), 0644)
	if errors.Is(err, os.ErrNotExist) {
		// IPv6 is not enabled on this host
		return nil
	}

	return err
}

// createCaptureTap adds a host tap with no VLAN tag and tags. mirror creates
// the mirror to tap with the given ovs-vsctl commands, which add tap, in the
// same transaction: the output port of a mirror only receives mirrored
// frames, so tap never receives the flooded frames of the bridge. Assumes
// bridgeLock is held.
func (b *Bridge) createCaptureTap(tap string, tags MirrorTags, mirror func(port []string) error) error {
	log.Info("creating capture tap on bridge: %v %v", b.Name, tap)

	b.reapTaps()

	if _, ok := b.taps[tap]; ok {
		return fmt.Errorf("tap already on bridge")
	}

	port := append([]string{"add-port", b.Name, tap}, tags.args()...)
	port = append(port, "--", "set", "Interface", tap, "type=internal")

	if err := mirror(port); err != nil {
		return err
	}

	b.forgetEdgePort(tap)
	b.taps[tap] = &Tap{
		Name:   tap,
		Bridge: b.Name,
		Host:   true,
	}

	err := disableIPv6(tap)
	if err == nil {
		err = upInterface(tap, true)
	}

	if err != nil {
		b.removeCaptureTap(tap)
		return err
	}

	return nil
}

// removeCaptureTap removes a capture tap and its mirror after a failed start.
// Assumes bridgeLock is held.
func (b *Bridge) removeCaptureTap(tap string) {
	if b.mirrors[tap] {
		if err := b.destroyMirror(tap); err != nil {
			log.Error("unable to remove mirror of %v: %v", tap, err)
		}
	}

	if err := b.destroyTap(tap); err != nil {
		log.Error("zombie tap -- %v %v", tap, err)
	}

	b.reapTaps()
}

// startCapture opens tap, creates the file and starts the engine. mirror is
// set when tap is a capture tap with a mirror. Assumes bridgeLock is held.
func (b *Bridge) startCapture(tap string, mirror bool, fname string, c CaptureConfig) (int, error) {
	return b.startCaptureSink(tap, mirror, c, func() (*fileSink, error) {
		return createFileSink(fname, tap, c)
	})
}

// startCaptureSink is startCapture with a sink from newSink. Assumes
// bridgeLock is held.
func (b *Bridge) startCaptureSink(tap string, mirror bool, c CaptureConfig, newSink func() (*fileSink, error)) (int, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}

	if c.SnapLen == 0 {
		c.SnapLen = DefaultSnapLen
	}

	src, err := openCaptureSource(tap, c)
	if err != nil {
		return 0, err
	}

	sink, err := newSink()
	if err != nil {
		src.Close()
		return 0, err
	}

	id := *b.captureID
	*b.captureID++

	e := newEngine(src, sink, c.MaxSize)
	if !mirror {
		e.recOpts.Inbound = c.Direction == DirectionIn
		e.recOpts.Outbound = c.Direction == DirectionOut
	}
	ca := &capture{
		id:     id,
		tap:    tap,
		mirror: mirror,
		config: c,
		start:  time.Now(),
		engine: e,
	}
	e.onEnd = func() { b.captureEnded(ca) }

	b.captures[id] = ca

	log.Info("capture %v on %v to %v", id, tap, sink.path)

	e.start()

	return id, nil
}

// release removes the mirror and the capture tap of c, once. Assumes
// bridgeLock is held.
func (b *Bridge) release(c *capture) {
	if c.released || !c.mirror {
		c.released = true
		return
	}

	c.released = true

	if b.mirrors[c.tap] {
		if err := b.destroyMirror(c.tap); err != nil {
			log.Error("stop capture %v %v: %v", c.tap, c.id, err)
		}
	}

	if _, ok := b.taps[c.tap]; ok {
		if err := b.destroyTap(c.tap); err != nil {
			log.Error("stop capture %v %v: %v", c.tap, c.id, err)
		}
	}
}

// captureEnded releases the resources of a capture that ended on its own. The
// capture stays in the list so that its state and error are visible.
func (b *Bridge) captureEnded(c *capture) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	state, err := c.engine.status()
	log.Info("capture %v on %v ended: %v %v", c.id, c.tap, state, err)

	if b.destroyed() {
		return
	}

	b.release(c)
	b.reapTaps()
}

// stopCapturesLocked stops all the captures and waits for them, also the
// captures that StopCapture stops. It tells all of them to stop before it
// waits, so that they drain at the same time. Assumes bridgeLock is held, so
// it is only for Bridge.destroy.
func (b *Bridge) stopCapturesLocked() {
	for _, c := range b.captures {
		c.engine.fail("bridge destroyed")
	}

	for id, c := range b.captures {
		<-c.engine.done

		b.release(c)
		delete(b.captures, id)
	}

	// StopCapture does not release them after destroy
	for _, c := range b.stopping {
		<-c.engine.done

		b.release(c)
	}
}

// Capture traffic from a bridge to fname. Only the first config is used, if
// there is more than one. Returns an ID which can be passed to StopCapture to
// stop the capture.
func (b *Bridge) Capture(fname string, config ...CaptureConfig) (int, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	var c CaptureConfig
	if len(config) > 0 {
		c = config[0]
	}

	// the capture tap only receives frames
	c.Direction = DirectionIn

	tap := <-b.tapChan
	tags := MirrorTags{Namespace: c.Namespace, Kind: MirrorKindBridge}
	mirror := func(port []string) error {
		if c.Ports == PortsEdge {
			return b.createEdgeMirror(tap, EdgeSelector{Namespace: c.Namespace, VLANs: c.VLANs, VMs: c.VMs}, tags, port)
		}
		return b.createMirrorVLANs("", tap, c.VLANs, tags, port)
	}

	if err := b.createCaptureTap(tap, tags, mirror); err != nil {
		return 0, err
	}

	id, err := b.startCapture(tap, true, fname, c)
	if err != nil {
		b.removeCaptureTap(tap)
		return 0, err
	}

	return id, nil
}

// CaptureTap captures traffic for the specified tap to fname. Only the first
// config is used, if there is more than one. Returns an ID which can be passed
// to StopCapture to stop the capture.
func (b *Bridge) CaptureTap(tap, fname string, config ...CaptureConfig) (int, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if t, ok := b.taps[tap]; !ok || t.Defunct {
		return 0, fmt.Errorf("unknown tap on bridge %v: %v", b.Name, tap)
	}

	var c CaptureConfig
	if len(config) > 0 {
		c = config[0]
	}

	return b.startCapture(tap, false, fname, c)
}

// StopCapture stops a capture and waits until its file is closed. It holds
// bridgeLock only to find the capture and to release its mirror and tap.
func (b *Bridge) StopCapture(id int) error {
	bridgeLock.Lock()

	c, ok := b.captures[id]
	if !ok {
		bridgeLock.Unlock()
		return fmt.Errorf("unknown capture ID: %v", id)
	}

	delete(b.captures, id)

	if b.stopping == nil {
		b.stopping = map[int]*capture{}
	}
	b.stopping[id] = c

	// keep the mirror until the engine ends: without it, the capture tap
	// gets the flooded frames of all VLANs
	bridgeLock.Unlock()

	log.Info("stopping capture: %v %v", c.tap, id)
	c.engine.Stop()

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	delete(b.stopping, id)

	if !b.destroyed() {
		b.release(c)
		b.reapTaps()
	}

	log.Info("stopped capture: %v %v", c.tap, id)

	return nil
}

// CaptureInfo returns information about the capture with the given ID.
func (b *Bridge) CaptureInfo(id int) (CaptureInfo, bool) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	c, ok := b.captures[id]
	if !ok {
		return CaptureInfo{}, false
	}

	return c.info(), true
}

// CaptureInfos returns information about all the captures on the bridge,
// sorted by ID.
func (b *Bridge) CaptureInfos() []CaptureInfo {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	var res []CaptureInfo
	for _, c := range b.captures {
		res = append(res, c.info())
	}

	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })

	return res
}

// failTapCaptures marks the captures that read tap as failed, because tap is
// removed. Assumes bridgeLock is held.
func (b *Bridge) failTapCaptures(tap string) {
	for _, c := range b.captures {
		if c.tap == tap && !c.mirror {
			c.engine.fail(fmt.Sprintf("tap %v removed", tap))
		}
	}
}

// StartEdgeStream mirrors the edge ports that sel selects to a new capture tap
// and writes the frames to w, which must already have its interface. It
// returns the capture ID, or an error when the bridge has no edge port for
// sel. Stop the stream with StopCapture.
func (b *Bridge) StartEdgeStream(sel EdgeSelector, c CaptureConfig, w pcapfile.Writer, tags MirrorTags) (int, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if len(b.edgePorts(sel)) == 0 {
		return 0, ErrNoEdgePorts
	}

	c.Direction = DirectionIn
	c.Format = w.Format()

	tap := <-b.tapChan
	mirror := func(port []string) error {
		return b.createEdgeMirror(tap, sel, tags, port)
	}

	if err := b.createCaptureTap(tap, tags, mirror); err != nil {
		return 0, err
	}

	id, err := b.startCaptureSink(tap, true, c, func() (*fileSink, error) {
		return &fileSink{w: w, path: "mesh:" + tags.Capture}, nil
	})
	if err != nil {
		b.removeCaptureTap(tap)
		return 0, err
	}

	return id, nil
}

// ErrNoEdgePorts is returned by StartEdgeStream for a bridge without edge
// ports.
var ErrNoEdgePorts = errors.New("no edge ports")

// MirrorApplied reports whether Open vSwitch applies the mirror of the
// capture with the given ID: an ignored mirror has no statistics.
func (b *Bridge) MirrorApplied(id int) (bool, error) {
	bridgeLock.Lock()
	c, ok := b.captures[id]
	bridgeLock.Unlock()

	if !ok || !c.mirror {
		return false, fmt.Errorf("unknown capture ID: %v", id)
	}

	out, err := ovsCmdWrapper([]string{"get", "Mirror", "mirror-" + c.tap, "statistics"})
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(out) != "{}", nil
}
