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
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// for tests
var openCaptureSource = openSource

// disableIPv6 stops the host from sending IPv6 router solicitations and DAD
// frames on a capture tap.
func disableIPv6(tap string) error {
	p := filepath.Join("/proc/sys/net/ipv6/conf", tap, "disable_ipv6")

	err := os.WriteFile(p, []byte("1"), 0644)
	if errors.Is(err, os.ErrNotExist) {
		// IPv6 is not enabled on this host
		return nil
	}

	return err
}

// createCaptureTap adds a host tap with no VLAN tag that only receives mirrored
// frames. Assumes bridgeLock is held.
func (b *Bridge) createCaptureTap(tap string, tags MirrorTags) error {
	log.Info("creating capture tap on bridge: %v %v", b.Name, tap)

	b.reapTaps()

	if _, ok := b.taps[tap]; ok {
		return fmt.Errorf("tap already on bridge")
	}

	if err := b.addTap(tap, "", 0, true); err != nil {
		return err
	}

	err := tagCapturePort(tap, tags)
	if err == nil {
		err = disableIPv6(tap)
	}
	if err == nil {
		err = upInterface(tap, true)
	}

	if err != nil {
		if err := b.destroyTap(tap); err != nil {
			log.Error("zombie tap -- %v %v", tap, err)
		}
		return err
	}

	return nil
}

// startCapture opens tap, creates the file and starts the engine. When mirror
// is set, it creates the mirror after it opens tap so that the first frames
// are not lost. Assumes bridgeLock is held.
func (b *Bridge) startCapture(tap string, mirror func() error, fname string, c CaptureConfig) (int, error) {
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

	sink, err := createFileSink(fname, tap, c)
	if err != nil {
		src.Close()
		return 0, err
	}

	if mirror != nil {
		if err := mirror(); err != nil {
			src.Close()
			sink.close()
			return 0, err
		}
	}

	id := *b.captureID
	*b.captureID++

	e := newEngine(src, sink, c.MaxSize)
	if mirror == nil {
		e.recOpts.Inbound = c.Direction == DirectionIn
		e.recOpts.Outbound = c.Direction == DirectionOut
	}
	ca := &capture{
		id:     id,
		tap:    tap,
		mirror: mirror != nil,
		config: c,
		start:  time.Now(),
		engine: e,
	}
	e.onEnd = func() { b.captureEnded(ca) }

	b.captures[id] = ca

	log.Info("capture %v on %v to %v", id, tap, fname)

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

// stopCaptureLocked stops a capture and waits for it. Assumes bridgeLock is
// held, so it is only for Bridge.destroy.
func (b *Bridge) stopCaptureLocked(c *capture) {
	if c.mirror && b.mirrors[c.tap] {
		if err := b.destroyMirror(c.tap); err != nil {
			log.Error("stop capture %v %v: %v", c.tap, c.id, err)
		}
	}

	c.engine.fail("bridge destroyed")
	<-c.engine.done

	b.release(c)
	delete(b.captures, c.id)
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
	if err := b.createCaptureTap(tap, MirrorTags{Namespace: c.Namespace, Kind: MirrorKindBridge}); err != nil {
		return 0, err
	}

	tags := MirrorTags{Namespace: c.Namespace, Kind: MirrorKindBridge}
	mirror := func() error {
		if c.Ports == PortsEdge {
			return b.createEdgeMirror(tap, EdgeSelector{Namespace: c.Namespace, VLANs: c.VLANs, VMs: c.VMs}, tags)
		}
		return b.createMirrorVLANs("", tap, c.VLANs, tags)
	}

	id, err := b.startCapture(tap, mirror, fname, c)
	if err != nil {
		if err := b.destroyTap(tap); err != nil {
			log.Error("zombie tap -- %v %v", tap, err)
		}
		b.reapTaps()

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

	return b.startCapture(tap, nil, fname, c)
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

	// remove the mirror first, so that no new frames arrive
	if c.mirror && !c.released && b.mirrors[c.tap] {
		if err := b.destroyMirror(c.tap); err != nil {
			log.Error("stop capture %v %v: %v", c.tap, id, err)
		}
	}

	bridgeLock.Unlock()

	log.Info("stopping capture: %v %v", c.tap, id)
	c.engine.Stop()

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

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
