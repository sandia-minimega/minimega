// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/internal/nscapture"
	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	nsCaptureStopWait    = 15 * time.Second
	nsCaptureDestroyWait = 5 * time.Second
	nsCaptureTick        = 50 * time.Millisecond
	nsCaptureAckPeriod   = 100 * time.Millisecond
	nsCaptureReconcile   = 2 * time.Second
	nsCaptureStatsCheck  = 10 * time.Second
	nsCaptureMaxSnapLen  = 65535
	nsCaptureInboxSize   = 1024
)

// nsCaptureHeader is the header of the capture pcap namespace listing.
var nsCaptureHeader = []string{
	"id", "host", "bridge", "state", "ports", "packets", "bytes", "kdrops", "pdrops", "qdrops", "lost", "late",
	"lag", "skew", "size", "path", "vlans", "filter", "start", "error",
}

var (
	// nsCaptureInbox holds batches for the head, nsCaptureControls holds acks
	// and stops for members. The mesh mux never blocks on them.
	nsCaptureInbox    = make(chan *nscapture.Batch, 1024)
	nsCaptureControls = make(chan nscapture.Control, 1024)

	nsCaptureDropped atomic.Uint64
	nsCaptureDropLog atomic.Int64

	nsHeads   = &headRegistry{m: map[string]map[int]*headCapture{}, next: map[string]int{}}
	nsMembers = &memberRegistry{m: map[memberKey]*memberCapture{}}
)

func init() {
	go nsCaptureDispatcher()
	go nsCaptureControlLoop()
}

// nsCaptureDeliver queues a mesh message for the capture code without
// blocking. It reports whether m was a capture message.
func nsCaptureDeliver(m *meshage.Message) bool {
	switch body := m.Body.(type) {
	case nscapture.Batch:
		select {
		case nsCaptureInbox <- &body:
		default:
			nsCaptureDrop("batch")
		}
	case nscapture.Control:
		select {
		case nsCaptureControls <- body:
		default:
			nsCaptureDrop("control")
		}
	default:
		return false
	}

	return true
}

// nsCaptureDrop counts a dropped message and logs at most once per 5 s.
func nsCaptureDrop(what string) {
	n := nsCaptureDropped.Add(1)

	now := time.Now().UnixNano()
	if last := nsCaptureDropLog.Load(); now-last > int64(5*time.Second) && nsCaptureDropLog.CompareAndSwap(last, now) {
		log.Warn("namespace capture: dropped a %v message, %v in total", what, n)
	}
}

// nsSender sends capture messages over the mesh, or in process to this host.
type nsSender struct{}

func (nsSender) Send(host string, body interface{}) error {
	if host == hostname {
		switch v := body.(type) {
		case nscapture.Batch:
			nsCaptureInbox <- &v
		case nscapture.Control:
			nsCaptureControls <- v
		}
		return nil
	}

	if meshageNode == nil {
		return errors.New("meshage is not running")
	}

	recipients, err := meshageNode.Set([]string{host}, body)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no route to %v", host)
	}

	return nil
}

// nsCaptureOptions are the options of a namespace capture.
type nsCaptureOptions struct {
	vlans     []string // as given
	vlanIDs   []int
	bridges   []string
	vms       []string
	filter    string
	snaplen   uint32
	format    string
	attribute string
	names     bool
	window    time.Duration
	maxSize   int64
	head      string
	id        int
	token     uint64
}

var nsCaptureInternal = map[string]bool{"head": true, "id": true, "token": true}

// nsFilterOK reports whether f is printable ASCII without a quote or a
// backslash, so that it passes the mesh as one quoted item.
func nsFilterOK(f string) bool {
	for _, r := range f {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return false
		}
	}

	return true
}

func parseNSCaptureOptions(ns *Namespace, opts []string, internal bool) (nsCaptureOptions, error) {
	o := nsCaptureOptions{
		filter:    ns.captures.Filter,
		snaplen:   ns.captures.SnapLen,
		format:    ns.captures.Format,
		attribute: nscapture.AttributePort,
		names:     true,
		window:    nscapture.DefaultWindow,
	}
	if o.snaplen == 0 || o.snaplen > nsCaptureMaxSnapLen {
		o.snaplen = bridge.DefaultSnapLen
	}

	seen := map[string]bool{}

	for _, opt := range opts {
		k, v, ok := strings.Cut(opt, "=")
		if !ok || k == "" {
			return o, fmt.Errorf("invalid option %q: use key=value", opt)
		}
		if seen[k] {
			return o, fmt.Errorf("option %v is given more than once", k)
		}
		seen[k] = true

		if nsCaptureInternal[k] && !internal {
			return o, fmt.Errorf("internal option: %v", k)
		}

		var err error

		switch k {
		case "ports":
			if v != bridge.PortsEdge {
				return o, errors.New("only ports=edge is supported")
			}
		case "vlans":
			for _, s := range strings.Split(v, ",") {
				if s != "" {
					o.vlans = append(o.vlans, s)
				}
			}
			o.vlanIDs, err = parseCaptureVLANs(ns.Name, v)
		case "bridges":
			for _, s := range strings.Split(v, ",") {
				if s != "" {
					o.bridges = append(o.bridges, s)
				}
			}
		case "vms":
			for _, s := range strings.Split(v, ",") {
				if s != "" {
					o.vms = append(o.vms, s)
				}
			}
		case "filter":
			if !nsFilterOK(v) {
				return o, errors.New("filter must be printable ASCII without \" or \\")
			}
			o.filter = v
		case "snaplen":
			var n uint64
			n, err = strconv.ParseUint(v, 10, 32)
			if err != nil || n < bridge.MinSnapLen || n > nsCaptureMaxSnapLen {
				return o, fmt.Errorf("snaplen must be from %v to %v", bridge.MinSnapLen, nsCaptureMaxSnapLen)
			}
			o.snaplen = uint32(n)
		case "format":
			err = bridge.ValidateFormat(v)
			o.format = v
		case "attribute":
			if v != nscapture.AttributePort && v != nscapture.AttributeStream {
				return o, errors.New("attribute must be port or stream")
			}
			o.attribute = v
		case "names":
			o.names, err = strconv.ParseBool(v)
		case "window":
			o.window, err = time.ParseDuration(v)
			if err == nil && (o.window < nscapture.MinWindow || o.window > nscapture.MaxWindow) {
				err = fmt.Errorf("window must be from %v to %v", nscapture.MinWindow, nscapture.MaxWindow)
			}
		case "maxsize":
			o.maxSize, err = bridge.ParseSize(v)
		case "head":
			o.head = v
		case "id":
			o.id, err = strconv.Atoi(v)
		case "token":
			o.token, err = strconv.ParseUint(v, 10, 64)
		default:
			return o, fmt.Errorf("unknown option: %v", k)
		}

		if err != nil {
			return o, fmt.Errorf("invalid option %v: %v", k, err)
		}
	}

	if o.format != pcapfile.FormatPcapng && (seen["attribute"] || seen["names"]) {
		return o, errors.New("attribute= and names= need format=pcapng")
	}

	if err := bridge.ValidateFilter(o.filter, o.snaplen); err != nil {
		return o, err
	}

	if internal && (o.head == "" || !seen["id"] || !seen["token"]) {
		return o, errors.New("missing internal option head=, id= or token=")
	}

	return o, nil
}

// memberArgs returns the options for the members.
func (o nsCaptureOptions) memberArgs() []string {
	args := []string{
		"snaplen=" + strconv.FormatUint(uint64(o.snaplen), 10),
		"head=" + o.head,
		"id=" + strconv.Itoa(o.id),
		"token=" + strconv.FormatUint(o.token, 10),
	}

	if o.format != "" {
		args = append(args, "format="+o.format)
	}
	if o.format == pcapfile.FormatPcapng {
		args = append(args, "attribute="+o.attribute)
	}
	if o.filter != "" {
		args = append(args, "filter="+o.filter)
	}

	if len(o.vlanIDs) > 0 {
		var vs []string
		for _, v := range o.vlanIDs {
			vs = append(vs, strconv.Itoa(v))
		}
		args = append(args, "vlans="+strings.Join(vs, ","))
	}
	if len(o.bridges) > 0 {
		args = append(args, "bridges="+strings.Join(o.bridges, ","))
	}
	if len(o.vms) > 0 {
		args = append(args, "vms="+strings.Join(o.vms, ","))
	}

	return args
}

// quoteArg quotes an argument for a command string when it needs quotes.
func quoteArg(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\"'\\#") {
		return strconv.Quote(s)
	}

	return s
}

// ---------------------------------------------------------------- head

type headRegistry struct {
	mu   sync.Mutex
	m    map[string]map[int]*headCapture
	next map[string]int
}

type headCapture struct {
	ns    string
	id    int
	token uint64
	opts  nsCaptureOptions
	path  string
	start time.Time

	file    *os.File
	release func()
	merger  *nscapture.Merger

	inbox chan *nscapture.Batch
	stop  chan struct{}
	done  chan struct{}

	stopOnce sync.Once

	mu      sync.Mutex
	hosts   []string
	state   string
	err     string
	streams []nscapture.StreamStatus
	sum     nscapture.StreamStatus
	size    int64
	acks    map[string]nscapture.Control
}

func (r *headRegistry) add(c *headCapture) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.m[c.ns] == nil {
		r.m[c.ns] = map[int]*headCapture{}
	}
	r.m[c.ns][c.id] = c
}

func (r *headRegistry) nextID(ns string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next[ns]++
	return r.next[ns]
}

func (r *headRegistry) get(ns string, id int) *headCapture {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.m[ns][id]
}

func (r *headRegistry) remove(ns string, id int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.m[ns], id)
}

// list returns the captures of ns, sorted by ID.
func (r *headRegistry) list(ns string) []*headCapture {
	r.mu.Lock()
	defer r.mu.Unlock()

	var res []*headCapture
	for _, c := range r.m[ns] {
		res = append(res, c)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].id < res[j].id })

	return res
}

// nsCaptureDispatcher gives each batch to its capture, or answers that the
// head does not know the capture.
func nsCaptureDispatcher() {
	for b := range nsCaptureInbox {
		c := nsHeads.get(b.Namespace, b.ID)
		if c == nil || b.Head != hostname || b.Token != c.token {
			go nsSender{}.Send(b.Host, nscapture.Control{
				Op: nscapture.OpAck, Namespace: b.Namespace, Head: b.Head, ID: b.ID, Token: b.Token,
				Bridge: b.Bridge, Unknown: true,
			})
			continue
		}

		b.SentAt = time.Now().UnixNano() // reuse as the receive time

		select {
		case c.inbox <- b:
		default:
			nsCaptureDrop("batch")
		}
	}
}

func (c *headCapture) setState(state, err string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case nscapture.StateStopped, nscapture.StateFailed, nscapture.StateFull:
		return
	}

	c.state = state
	if err != "" && c.err == "" {
		c.err = err
	}
}

func (c *headCapture) getHosts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.hosts...)
}

// run merges the batches until the capture stops, then closes the file.
func (c *headCapture) run() {
	defer close(c.done)

	tick := time.NewTicker(nsCaptureTick)
	defer tick.Stop()

	ackTick := time.NewTicker(nsCaptureAckPeriod)
	defer ackTick.Stop()

	var stopping bool
	var stopDeadline time.Time

	for {
		select {
		case b := <-c.inbox:
			recv := b.SentAt
			seq, ok := c.merger.Add(b)
			if ok && b.Bridge != "" {
				c.mu.Lock()
				c.acks[b.Host+"/"+b.Bridge] = nscapture.Control{
					Op: nscapture.OpAck, Namespace: c.ns, Head: hostname, ID: c.id, Token: c.token,
					Bridge: b.Bridge, Seq: seq, HeadRecv: recv,
				}
				c.mu.Unlock()
			}
		case <-tick.C:
			c.merger.Tick()
		case <-ackTick.C:
			c.sendAcks()
		case <-c.stop:
			if !stopping {
				stopping = true
				stopDeadline = time.Now().Add(nsCaptureStopWait)
				c.sendStop()
			}
		}

		if !stopping && (c.merger.Full() || c.merger.Err() != nil) {
			if c.merger.Full() {
				c.setState(nscapture.StateFull, fmt.Sprintf("file reached maxsize %v", c.opts.maxSize))
			} else {
				c.setState(nscapture.StateFailed, c.merger.Err().Error())
			}
			stopping = true
			stopDeadline = time.Now().Add(nsCaptureStopWait)
			c.sendStop()
		}

		c.snapshot()

		if stopping && (c.merger.Done(c.getHosts()) || time.Now().After(stopDeadline)) {
			break
		}
	}

	// take the batches that arrived with the last ones
	for {
		select {
		case b := <-c.inbox:
			c.merger.Add(b)
			continue
		default:
		}
		break
	}

	err := c.merger.Finish(c.getHosts())
	if err2 := c.file.Sync(); err == nil {
		err = err2
	}
	if err2 := c.file.Close(); err == nil {
		err = err2
	}
	c.release()

	if err != nil {
		c.setState(nscapture.StateFailed, err.Error())
	}
	c.setState(nscapture.StateStopped, "")
	c.snapshot()

	log.Info("namespace capture %v of %v ended", c.id, c.ns)
}

// sendAcks sends the latest ack of each stream, in a goroutine so that a slow
// mesh never blocks the merger.
func (c *headCapture) sendAcks() {
	window := nscapture.DefaultAckLimit
	if len(c.inbox) > nsCaptureInboxSize/2 || c.merger.HeapBytes() > nscapture.MemoryCap*3/4 {
		window = nscapture.SlowAckLimit
	}

	c.mu.Lock()
	acks := c.acks
	c.acks = map[string]nscapture.Control{}
	c.mu.Unlock()

	if len(acks) == 0 {
		return
	}

	go func() {
		for key, a := range acks {
			a.Window = window
			a.HeadSend = time.Now().UnixNano()
			host, _, _ := strings.Cut(key, "/")
			if err := (nsSender{}).Send(host, a); err != nil {
				log.Debug("namespace capture %v: ack to %v: %v", c.id, host, err)
			}
		}
	}()
}

// sendStop sends OpStop to every host that accepted the start.
func (c *headCapture) sendStop() {
	for _, host := range c.getHosts() {
		go func(host string) {
			ctrl := nscapture.Control{Op: nscapture.OpStop, Namespace: c.ns, Head: hostname, ID: c.id, Token: c.token}
			if err := (nsSender{}).Send(host, ctrl); err != nil {
				log.Warn("namespace capture %v: stop to %v: %v", c.id, host, err)
			}
		}(host)
	}
}

func (c *headCapture) snapshot() {
	streams, sum := c.merger.Status()
	size := c.merger.Size()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.streams, c.sum, c.size = streams, sum, size

	if c.state == nscapture.StateRunning {
		for _, s := range streams {
			if s.State == nscapture.StateStalled {
				c.state = nscapture.StateStalled
			}
		}
	} else if c.state == nscapture.StateStalled {
		stalled := false
		for _, s := range streams {
			stalled = stalled || s.State == nscapture.StateStalled
		}
		if !stalled {
			c.state = nscapture.StateRunning
		}
	}
}

// requestStop tells the capture to stop and waits at most wait.
func (c *headCapture) requestStop(wait time.Duration) {
	c.setState(nscapture.StateStopping, "")
	c.stopOnce.Do(func() { close(c.stop) })

	select {
	case <-c.done:
	case <-time.After(wait):
		log.Warn("namespace capture %v of %v did not stop in %v", c.id, c.ns, wait)
	}
}

func durString(d time.Duration, round time.Duration) string {
	return d.Round(round).String()
}

// rows returns the listing rows of the capture: the summary, then the streams.
func (c *headCapture) rows() [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	u := func(v uint64) string { return strconv.FormatUint(v, 10) }
	id := strconv.Itoa(c.id)

	res := [][]string{{
		id, "", "", c.state, strconv.Itoa(c.sum.Ports), u(c.sum.Packets), u(c.sum.Bytes), u(c.sum.KDrops),
		u(c.sum.PDrops), u(c.sum.QDrops), u(c.sum.Lost), u(c.sum.Late), durString(c.sum.Lag, time.Millisecond), "",
		strconv.FormatInt(c.size, 10), c.path, strings.Join(c.opts.vlans, ","), c.opts.filter,
		c.start.UTC().Format(time.RFC3339), c.err,
	}}

	for _, s := range c.streams {
		res = append(res, []string{
			id, s.Host, s.Bridge, s.State, strconv.Itoa(s.Ports), u(s.Packets), u(s.Bytes), u(s.KDrops),
			u(s.PDrops), u(s.QDrops), u(s.Lost), u(s.Late), durString(s.Lag, time.Millisecond),
			durString(s.Skew, time.Microsecond), "", "", "", "", "", s.Error,
		})
	}

	return res
}

// nsCaptureVersionGate refuses a start when a mesh node does not know the
// namespace capture: an old node closes its mesh connection on the new
// message types.
func nsCaptureVersionGate() error {
	if meshageNode == nil || len(meshageNode.BroadcastRecipients()) == 0 {
		return nil
	}

	cmd := minicli.MustCompile("mesh send all capture pcap namespace")
	cmd.SetRecord(false)

	var bad []string
	for resps := range runCommands(cmd) {
		for _, resp := range resps {
			if resp.Error != "" {
				bad = append(bad, resp.Host)
			}
		}
	}

	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("mesh nodes %v do not support namespace captures; minimega 3.4.0 or later must run on every node, relays included", strings.Join(bad, ", "))
	}

	return nil
}

// cliNSCaptureStart starts a namespace capture on the head.
func cliNSCaptureStart(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	fname := c.StringArgs["filename"]
	if err := checkCaptureFilename(fname); err != nil {
		return err
	}

	o, err := parseNSCaptureOptions(ns, c.ListArgs["option"], false)
	if err != nil {
		return err
	}

	if err := nsCaptureVersionGate(); err != nil {
		return err
	}

	o.head = hostname
	o.id = nsHeads.nextID(ns.Name)
	o.token = rand.Uint64()

	hc, err := newHeadCapture(ns.Name, ns.hostSlice(), o, capturePath(fname), c.Original)
	if err != nil {
		return err
	}

	if err := nsCaptureFanOut(ns, hc, hc.hosts); err != nil {
		hc.setState(nscapture.StateFailed, err.Error())
		hc.requestStop(nsCaptureStopWait)
		return err
	}

	hc.setState(nscapture.StateRunning, "")

	resp.Response = strconv.Itoa(hc.id)
	return nil
}

// newHeadCapture creates the file of a namespace capture, registers the
// capture and starts its merger.
func newHeadCapture(ns string, hosts []string, o nsCaptureOptions, path, original string) (*headCapture, error) {
	release, err := bridge.ClaimCapturePath(path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		release()
		return nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		release()
		return nil, err
	}

	comment := fmt.Sprintf("minimega namespace capture %v of %v; vlans %v; filter %q; command: %v",
		o.id, ns, strings.Join(o.vlans, ","), o.filter, original)

	w, err := bridge.NewCaptureWriter(f, o.format, o.snaplen, comment)
	if err == nil {
		err = w.Flush()
	}
	if err != nil {
		f.Close()
		release()
		return nil, err
	}

	hc := &headCapture{
		ns:      ns,
		id:      o.id,
		token:   o.token,
		opts:    o,
		path:    path,
		start:   time.Now(),
		file:    f,
		release: release,
		inbox:   make(chan *nscapture.Batch, nsCaptureInboxSize),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		hosts:   append([]string(nil), hosts...),
		state:   nscapture.StateStarting,
		acks:    map[string]nscapture.Control{},
	}
	sort.Strings(hc.hosts)

	hc.merger = nscapture.NewMerger(nscapture.MergerConfig{
		Namespace: ns,
		Window:    o.window,
		SnapLen:   o.snaplen,
		MaxSize:   o.maxSize,
		Attribute: o.attribute,
		Names:     o.names,
		Token:     o.token,
		Filter:    o.filter,
	}, w, nil)

	nsHeads.add(hc)
	go hc.run()

	return hc, nil
}

// nsCaptureFanOut starts the members of hc on hosts. When a host fails, it
// returns the errors; the caller rolls back with OpStop.
func nsCaptureFanOut(ns *Namespace, hc *headCapture, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}

	args := []string{"capture", "pcap", "namespace", quoteArg(hc.path)}
	for _, a := range hc.opts.memberArgs() {
		args = append(args, quoteArg(a))
	}
	original := strings.Join(args, " ")

	var cmds []*minicli.Command
	var peers []string

	for _, host := range hosts {
		if host == hostname {
			cmds = append(cmds, minicli.MustCompile(original))
		} else {
			peers = append(peers, strconv.Quote(host))
		}
	}
	if len(peers) > 0 {
		cmds = append(cmds, minicli.MustCompilef("mesh send %v namespace %q %v", strings.Join(peers, ","), ns.Name, original))
	}

	for _, cmd := range cmds {
		cmd.SetSource(ns.Name)
		cmd.SetNamespace(ns.Name)
		cmd.SetRecord(false)
	}

	var errs []string
	for resps := range runCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				errs = append(errs, fmt.Sprintf("%v: %v", resp.Host, resp.Error))
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// nsCaptureAddHosts starts the running captures of ns on hosts that ns
// add-hosts added.
func nsCaptureAddHosts(ns *Namespace, hosts []string) {
	for _, hc := range nsHeads.list(ns.Name) {
		hc.mu.Lock()
		running := hc.state == nscapture.StateRunning || hc.state == nscapture.StateStalled
		var add []string
		for _, h := range hosts {
			var known bool
			for _, h2 := range hc.hosts {
				known = known || h == h2
			}
			if !known {
				add = append(add, h)
			}
		}
		if running {
			hc.hosts = append(hc.hosts, add...)
		}
		hc.mu.Unlock()

		if running && len(add) > 0 {
			if err := nsCaptureFanOut(ns, hc, add); err != nil {
				log.Error("namespace capture %v: start on new hosts: %v", hc.id, err)
			}
		}
	}
}

// nsCaptureMatch returns the captures of ns that target selects: an ID, a file
// name or all.
func nsCaptureMatch(ns, target string) []*headCapture {
	var res []*headCapture

	for _, hc := range nsHeads.list(ns) {
		switch {
		case target == Wildcard,
			target == strconv.Itoa(hc.id),
			target == hc.path,
			capturePath(target) == hc.path:
			res = append(res, hc)
		}
	}

	return res
}

// nsCaptureStopHead stops the captures of ns that target selects and returns
// their final rows.
func nsCaptureStopHead(ns, target string, wait time.Duration) [][]string {
	var rows [][]string

	for _, hc := range nsCaptureMatch(ns, target) {
		hc.requestStop(wait)
		rows = append(rows, hc.rows()...)
		nsHeads.remove(ns, hc.id)
	}

	return rows
}

// nsCaptureDestroy stops the head and member captures of a namespace that is
// destroyed. It sends only OpStop and waits at most 5 s.
func nsCaptureDestroy(ns string) {
	for _, hc := range nsHeads.list(ns) {
		hc.setState(nscapture.StateStopping, "")
		hc.stopOnce.Do(func() { close(hc.stop) })
	}
	for _, hc := range nsHeads.list(ns) {
		select {
		case <-hc.done:
		case <-time.After(nsCaptureDestroyWait):
		}
		nsHeads.remove(ns, hc.id)
	}

	nsMembers.stopNamespace(ns)
}

func cliNSCapture(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["delete"]:
		resp.Header = nsCaptureHeader
		resp.Tabular = nsCaptureStopHead(ns.Name, c.StringArgs["id"], nsCaptureStopWait)
		if len(resp.Tabular) == 0 && c.StringArgs["id"] != Wildcard {
			return fmt.Errorf("no namespace capture %v", c.StringArgs["id"])
		}
		return nil
	case c.StringArgs["filename"] != "":
		if c.Source != "" {
			return cliNSCaptureMember(ns, c, resp)
		}
		return cliNSCaptureStart(ns, c, resp)
	}

	resp.Header = nsCaptureHeader
	resp.Tabular = [][]string{}
	for _, hc := range nsHeads.list(ns.Name) {
		resp.Tabular = append(resp.Tabular, hc.rows()...)
	}

	return nil
}

// ---------------------------------------------------------------- member

type memberKey struct {
	ns, head string
	id       int
}

type memberRegistry struct {
	mu sync.Mutex
	m  map[memberKey]*memberCapture
}

type memberStream struct {
	bridge  string
	capID   int
	stream  *nscapture.Stream
	ports   []nscapture.PortInfo
	started time.Time
	checked bool
}

type memberCapture struct {
	key  memberKey
	opts nsCaptureOptions

	mu       sync.Mutex
	streams  map[string]*memberStream
	stopped  bool
	headSeen time.Time

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func (r *memberRegistry) get(k memberKey) *memberCapture {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.m[k]
}

func (r *memberRegistry) stopNamespace(ns string) {
	r.mu.Lock()
	var list []*memberCapture
	for k, m := range r.m {
		if k.ns == ns {
			list = append(list, m)
		}
	}
	r.mu.Unlock()

	for _, m := range list {
		m.requestStop("")
	}
	for _, m := range list {
		select {
		case <-m.done:
		case <-time.After(nsCaptureDestroyWait):
		}
	}
}

// cliNSCaptureMember starts the member part of a namespace capture.
func cliNSCaptureMember(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	o, err := parseNSCaptureOptions(ns, c.ListArgs["option"], true)
	if err != nil {
		return err
	}

	k := memberKey{ns: ns.Name, head: o.head, id: o.id}

	nsMembers.mu.Lock()
	if _, ok := nsMembers.m[k]; ok {
		nsMembers.mu.Unlock()
		return fmt.Errorf("capture %v of %v already runs on %v", o.id, o.head, hostname)
	}

	m := &memberCapture{
		key:      k,
		opts:     o,
		streams:  map[string]*memberStream{},
		headSeen: time.Now(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	nsMembers.m[k] = m
	nsMembers.mu.Unlock()

	if err := m.reconcile(); err != nil {
		m.requestStop(err.Error())
		<-m.done
		return err
	}

	go m.loop()

	m.mu.Lock()
	resp.Response = strconv.Itoa(len(m.streams))
	m.mu.Unlock()

	return nil
}

func (m *memberCapture) selector() bridge.EdgeSelector {
	return bridge.EdgeSelector{Namespace: m.key.ns, VLANs: m.opts.vlanIDs, VMs: m.opts.vms}
}

func (m *memberCapture) wantBridge(name string) bool {
	if len(m.opts.bridges) == 0 {
		return true
	}
	for _, b := range m.opts.bridges {
		if b == name {
			return true
		}
	}
	return false
}

func portTable(taps []bridge.Tap) []nscapture.PortInfo {
	var res []nscapture.PortInfo

	for i, t := range taps {
		kind := "vm"
		switch {
		case t.Host:
			kind = "host"
		case t.Container:
			kind = "container"
		}

		var addrs []string
		for _, a := range []string{t.IP4, t.IP6} {
			if a != "" {
				addrs = append(addrs, a)
			}
		}

		res = append(res, nscapture.PortInfo{
			Index: uint32(i + 1), Port: t.Name, MAC: t.MAC, VLAN: t.VLAN, Kind: kind, VM: t.Owner, Addrs: addrs,
		})
	}

	return res
}

func samePorts(a, b []nscapture.PortInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Port != b[i].Port || a[i].MAC != b[i].MAC || a[i].VLAN != b[i].VLAN ||
			strings.Join(a[i].Addrs, ",") != strings.Join(b[i].Addrs, ",") {
			return false
		}
	}
	return true
}

// reconcile starts streams on bridges that have edge ports and no stream, and
// updates the port tables and counters.
func (m *memberCapture) reconcile() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	sel := m.selector()
	var errs []string

	format := nscapture.FormatClassic
	if m.opts.format == pcapfile.FormatPcapng && m.opts.attribute == nscapture.AttributePort {
		format = nscapture.FormatNg
	}

	for _, name := range bridges.Names() {
		if !m.wantBridge(name) {
			continue
		}

		br, ok := bridges.Lookup(name)
		if !ok {
			continue
		}

		if s, ok := m.streams[name]; ok {
			if ports := portTable(br.EdgeTaps(sel)); !samePorts(ports, s.ports) {
				s.ports = ports
				s.stream.SetPorts(ports)
			}
			if info, ok := br.CaptureInfo(s.capID); ok {
				s.stream.SetDrops(info.KernelDrops, 0, 0)
			}
			if !s.checked && time.Since(s.started) >= nsCaptureStatsCheck {
				s.checked = true
				if applied, err := br.MirrorApplied(s.capID); err == nil && !applied {
					s.stream.Fail("Open vSwitch did not apply the mirror")
					go br.StopCapture(s.capID)
				}
			}
			continue
		}

		taps := br.EdgeTaps(sel)
		if len(taps) == 0 {
			continue
		}

		stream := nscapture.NewStream(nscapture.StreamConfig{
			Namespace: m.key.ns, Head: m.key.head, Host: hostname, Bridge: name,
			ID: m.key.id, Token: m.opts.token, Format: format,
		}, nsSender{}, nil)
		stream.OnUnknown = func() { m.requestStop("head does not know the capture") }

		ports := portTable(taps)
		stream.SetPorts(ports)

		tags := bridge.MirrorTags{
			Namespace: m.key.ns,
			Kind:      bridge.MirrorKindNamespace,
			Capture:   fmt.Sprintf("%v/%v/%v", m.key.ns, m.key.head, m.key.id),
		}
		config := bridge.CaptureConfig{Filter: m.opts.filter, SnapLen: m.opts.snaplen, Namespace: m.key.ns}

		id, err := br.StartEdgeStream(sel, config, stream, tags)
		if errors.Is(err, bridge.ErrNoEdgePorts) {
			stream.Fail("no edge ports")
			stream.Close()
			continue
		} else if err != nil {
			stream.Fail(err.Error())
			stream.Close()
			errs = append(errs, fmt.Sprintf("bridge %v: %v", name, err))
			continue
		}

		m.streams[name] = &memberStream{bridge: name, capID: id, stream: stream, ports: ports, started: time.Now()}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// headPresent reports whether the head is on the mesh.
func (m *memberCapture) headPresent() bool {
	if m.key.head == hostname || meshageNode == nil {
		return true
	}

	_, ok := meshageNode.Mesh()[m.key.head]
	return ok
}

func (m *memberCapture) loop() {
	t := time.NewTicker(nsCaptureReconcile)
	defer t.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
		}

		if m.headPresent() {
			m.headSeen = time.Now()
		} else if time.Since(m.headSeen) >= nscapture.OrphanTimeout {
			log.Warn("namespace capture %v of %v: head %v gone for %v, stopping as orphaned",
				m.key.id, m.key.ns, m.key.head, nscapture.OrphanTimeout)
			m.requestStop(nscapture.StateOrphaned)
			return
		}

		if err := m.reconcile(); err != nil {
			log.Warn("namespace capture %v of %v: %v", m.key.id, m.key.ns, err)
		}
	}
}

// requestStop stops all streams, sends the host-level final batch and removes
// the member. It does not wait.
func (m *memberCapture) requestStop(reason string) {
	m.stopOnce.Do(func() {
		close(m.stop)
		go m.shutdown(reason)
	})
}

func (m *memberCapture) shutdown(reason string) {
	defer close(m.done)

	m.mu.Lock()
	m.stopped = true
	streams := m.streams
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range streams {
		wg.Add(1)
		go func(s *memberStream) {
			defer wg.Done()

			if br, ok := bridges.Lookup(s.bridge); ok {
				br.StopCapture(s.capID)
			}
			// the engine closes the stream; close it here when the engine ended
			s.stream.Close()
		}(s)
	}
	wg.Wait()

	final := nscapture.Batch{
		Namespace: m.key.ns, Head: m.key.head, ID: m.key.id, Token: m.opts.token, Host: hostname,
		Final: true, Err: reasonErr(reason),
	}
	if err := (nsSender{}).Send(m.key.head, final); err != nil {
		log.Warn("namespace capture %v: final batch to %v: %v", m.key.id, m.key.head, err)
	}

	nsMembers.mu.Lock()
	delete(nsMembers.m, m.key)
	nsMembers.mu.Unlock()
}

func reasonErr(reason string) string {
	if reason == nscapture.StateOrphaned {
		return "orphaned: head gone"
	}
	return reason
}

// nsCaptureControlLoop gives acks and stops to the member streams.
func nsCaptureControlLoop() {
	for c := range nsCaptureControls {
		m := nsMembers.get(memberKey{ns: c.Namespace, head: c.Head, id: c.ID})
		if m == nil || m.opts.token != c.Token {
			continue
		}

		switch c.Op {
		case nscapture.OpStop:
			m.requestStop("")
		case nscapture.OpAck:
			m.mu.Lock()
			s := m.streams[c.Bridge]
			m.mu.Unlock()

			if s != nil {
				s.stream.Ack(c)
			} else if c.Unknown {
				m.requestStop("head does not know the capture")
			}
		}
	}
}

// nsCaptureStopMembers stops the member captures of ns, for clear capture.
func nsCaptureStopMembers(ns string) {
	nsMembers.stopNamespace(ns)
}
