// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

const DefaultSnapLen = 1600

type CaptureConfig struct {
	// Filter is a BPF string to apply to all packets. See `man pcap-filter`
	// for the syntax and semantics.
	Filter string

	// SnapLen controls how many bytes to capture for each packet. According to
	// `man pcap`, 65535 should be sufficient to capture full packets on most
	// networks. If you only need headers, you can set it much lower (i.e.
	// 200). When zero, we use DefaultSnapLen.
	SnapLen uint32
}

// stopCapture stops a capture by ID which is assumed to exist
func (b *Bridge) stopCapture(id int) {
	tap := b.captures[id].tap

	log.Info("stopping capture: %v %v", tap, id)

	atomic.StoreUint64(b.captures[id].isstopped, 1)

	if b.mirrors[tap] {
		if err := b.removeMirror(tap); err != nil {
			log.Error("stop capture %v %v: %v", tap, id, err)
		}
	}

	// wait until the capture is closed before returning
	<-b.captures[id].ack
	delete(b.captures, id)

	log.Info("stopped capture: %v %v", tap, id)
}

func (b *Bridge) captureTap(tap, fname string, config ...CaptureConfig) (int, error) {
	log.Info("capture %v to %v", tap, fname)

	// start with defaults
	c := CaptureConfig{
		SnapLen: DefaultSnapLen,
	}

	// if there are configs, only process the first one
	if len(config) > 0 {
		c2 := config[0]
		log.Debug("using config: %v", c2)

		if c2.SnapLen == 0 {
			c2.SnapLen = DefaultSnapLen
		}

		c.Filter = c2.Filter
		c.SnapLen = c2.SnapLen
	}

	handle, err := pcap.OpenLive(tap, int32(c.SnapLen), true, time.Second)
	if err != nil {
		return 0, err
	}

	if c.Filter != "" {
		if err := handle.SetBPFFilter(c.Filter); err != nil {
			handle.Close()
			return 0, err
		}
	}

	f, err := os.Create(fname)
	if err != nil {
		handle.Close()
		return 0, err
	}

	w := pcapgo.NewWriter(f)

	// write the file header
	if err := w.WriteFileHeader(c.SnapLen, layers.LinkTypeEthernet); err != nil {
		handle.Close()
		f.Close()
		return 0, err
	}

	id := len(b.captures)
	stopped := uint64(0)
	ack := make(chan bool)

	b.captures[id] = capture{
		tap:       tap,
		isstopped: &stopped,
		ack:       ack,
	}

	// start a goroutine to do the capture, runs until it encounters an error
	// and then closes the f and cleans up.
	go func() {
		defer close(ack)
		defer handle.Close()
		defer f.Close()

		var err error

		for err == nil && atomic.LoadUint64(&stopped) == 0 {
			data, ci, err2 := handle.ReadPacketData()

			if err2 == pcap.NextErrorTimeoutExpired {
				continue
			} else if err2 == nil {
				err = w.WritePacket(ci, data)
			} else {
				err = err2
			}
		}

		if err != nil && atomic.LoadUint64(&stopped) != 0 {
			log.Error("packet copier for %v: %v", tap, err)
		}

		log.Info("capture stopped: %v %v", tap, id)
	}()

	return id, nil
}

// Capture traffic from a bridge to fname. Only the first config is used, if
// there is more than one. Returns an ID which can be passed to RemoveCapture
// to stop the capture.
func (b *Bridge) Capture(fname string, config ...CaptureConfig) (int, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	tap, err := b.addMirror()
	if err != nil {
		return 0, err
	}

	id, err := b.captureTap(tap, fname, config...)
	if err != nil {
		if err := b.removeMirror(tap); err != nil {
			// Welp, we're boned
			log.Error("zombie mirror -- %v:%v %v", b.Name, tap, err)
		}

		return 0, err
	}

	return id, nil
}

// CaptureTap captures traffic for the specified tap to fname. Only the first
// config is used, if there is more than one. Returns an ID which can be passed
// to RemoveCapture to stop the capture.
func (b *Bridge) CaptureTap(tap, fname string, config ...CaptureConfig) (int, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if _, ok := b.taps[tap]; !ok {
		return 0, fmt.Errorf("unknown tap on bridge %v: %v", b.Name, tap)
	}

	return b.captureTap(tap, fname, config...)
}

func (b *Bridge) StopCapture(id int) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if _, ok := b.captures[id]; !ok {
		return fmt.Errorf("unknown capture ID: %v", id)
	}

	b.stopCapture(id)

	return nil
}