// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// maxKBLine is the longest kb file line, in bytes. A base64 template can be
// much longer than the 64 KB default of bufio.Scanner.
const maxKBLine = 16 << 20

// errStopped tells the playback goroutine that the playback stopped.
var errStopped = errors.New("playback stopped")

type playback struct {
	*Conn // embed

	ID string // ID to identify playback

	start time.Time // start for when the playback started
	clock clock

	// out sends events to the writer goroutine. Nothing closes it: each
	// send also selects on done.
	out chan Event

	// wake tells the playback goroutine that the control state changed.
	wake chan struct{}

	done        chan struct{}    // closed by Stop
	ended       chan struct{}    // closed when the connection is closed
	screenshots chan *image.RGBA // latest screenshot of the VM

	sync.Mutex               // guards below
	depth      int           // how nested we are in LoadFiles
	duration   time.Duration // total playback duration
	e          string        // current event
	state      Control       // playback state, only Play or Pause
	step       bool          // a Step is pending
	pending    []any         // injected LoadFile and WaitForIt events
	closed     bool          // set after playback closed
	file       *os.File      // file that we are reading
}

// newPlayback creates a new playback with given id.
func newPlayback(id, rhost string) (*playback, error) {
	conn, err := Dial(rhost)
	if err != nil {
		return nil, err
	}

	return &playback{
		ID:          id,
		Conn:        conn,
		clock:       realClock{},
		out:         make(chan Event),
		wake:        make(chan struct{}, 1),
		done:        make(chan struct{}),
		ended:       make(chan struct{}),
		screenshots: make(chan *image.RGBA, 1),
		state:       Play,
	}, nil
}

func (p *playback) Closed() bool {
	p.Lock()
	defer p.Unlock()

	return p.closed
}

func (p *playback) Info() []string {
	p.Lock()
	defer p.Unlock()

	if p.closed {
		return nil
	}

	res := []string{
		p.ID,
		"playback kb",
	}

	if p.state == Pause {
		res = append(res, "PAUSED")
	} else {
		res = append(res, fmt.Sprintf("%v remaining", p.duration))
	}

	if p.file != nil {
		res = append(res, p.file.Name())
	} else {
		res = append(res, "N/A")
	}

	return res
}

// Start opens filename and starts the playback. It does not close the
// connection on an error.
func (p *playback) Start(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Keep the encodings from the handshake: Raw and DesktopSize, no cursor.

	p.Lock()
	p.start = time.Now()
	p.state = Play
	p.Unlock()

	go p.writeEvents()
	go func() {
		if err := p.playFile(f); err != nil && !errors.Is(err, errStopped) {
			log.Error("playback %v failed: %v", p.ID, err)
		}

		p.Stop()
	}()
	go p.readScreen()

	return nil
}

// readScreen reads the messages of the server. It keeps the latest
// screenshot for waitForIt, and it stops the playback when the connection
// fails.
func (p *playback) readScreen() {
	for {
		msg, err := p.Conn.ReadMessage()
		if err != nil {
			if !p.Closed() { // if already closed, don't care about error. likely eof
				log.Error("server to playback error: %v", err)
			}
			p.Stop()
			return
		}

		if msg, ok := msg.(*FramebufferUpdate); ok {
			for _, rect := range msg.Rectangles {
				// ignore non-image
				if rect.RGBA != nil {
					p.putScreenshot(rect.RGBA)
				}
			}
		}
	}
}

// putScreenshot replaces the screenshot in p.screenshots with img. Only
// readScreen sends on p.screenshots.
func (p *playback) putScreenshot(img *image.RGBA) {
	select {
	case <-p.screenshots:
	default:
	}

	p.screenshots <- img
}

// poke wakes the playback goroutine. It does not block.
func (p *playback) poke() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *playback) Step() error {
	p.Lock()
	if p.state != Play || p.closed {
		p.Unlock()
		return errors.New("playback not stepable")
	}
	p.step = true
	p.Unlock()

	p.poke()
	return nil
}

func (p *playback) Pause() error {
	p.Lock()
	if p.state != Play || p.closed {
		p.Unlock()
		return errors.New("playback not pauseable")
	}
	p.state = Pause
	p.Unlock()

	p.poke()
	return nil
}

func (p *playback) Continue() error {
	p.Lock()
	if p.state != Pause || p.closed {
		p.Unlock()
		return errors.New("playback not playable")
	}
	p.state = Play
	p.Unlock()

	p.poke()
	return nil
}

// Stop stops the playback. The writer goroutine closes the connection after
// it writes the event that it has.
func (p *playback) Stop() error {
	p.Lock()
	defer p.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.done)
	log.Info("Finished playback on %v", p.ID)

	return nil
}

func (p *playback) Inject(cmd string) error {
	e, err := parseEvent(cmd)
	if err != nil {
		return err
	}

	return p.InjectEvent(e)
}

// InjectEvent sends an Event at once. It queues a LoadFile or WaitForIt event
// for the playback goroutine, which runs it when the playback is not paused.
func (p *playback) InjectEvent(e any) error {
	if event, ok := e.(Event); ok {
		if p.Closed() {
			return errors.New("playback has already stopped")
		}
		if !p.send(event) {
			return errors.New("playback has already stopped")
		}
		return nil
	}

	switch e := e.(type) {
	case *LoadFileEvent:
		p.Lock()
		name := e.File
		if !filepath.IsAbs(name) && p.file != nil {
			name = filepath.Join(filepath.Dir(p.file.Name()), name)
		}
		p.Unlock()

		if _, err := os.Stat(name); err != nil {
			return err
		}
		e = &LoadFileEvent{File: name}

		return p.queue(e)
	case *WaitForItEvent:
		return p.queue(e)
	default:
		return fmt.Errorf("unknown event: %v", e)
	}
}

func (p *playback) queue(e any) error {
	p.Lock()
	if p.closed {
		p.Unlock()
		return errors.New("playback has already stopped")
	}
	p.pending = append(p.pending, e)
	p.Unlock()

	p.poke()
	return nil
}

// send gives e to the writer goroutine. It returns false if the playback
// stopped.
func (p *playback) send(e Event) bool {
	select {
	case p.out <- e:
		return true
	case <-p.done:
		return false
	}
}

func (p *playback) GetStep() (string, error) {
	p.Lock()
	defer p.Unlock()

	if p.closed {
		return "", errors.New("playback has already stopped")
	}

	return p.e, nil
}

// newKBScanner returns a scanner for the lines of a kb file.
func newKBScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), maxKBLine)
	return s
}

// loadFile plays the named file. A relative name is in the directory of
// parent.
func (p *playback) loadFile(parent *os.File, filename string) error {
	if !filepath.IsAbs(filename) && parent != nil {
		// Our file is in the same directory as the parent
		filename = filepath.Join(filepath.Dir(parent.Name()), filename)
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	return p.playFile(f)
}

// playFile plays the events of f, and then closes f.
func (p *playback) playFile(f *os.File) error {
	defer f.Close()
	log.Info("Start playback of %v on %v", f.Name(), p.ID)

	// record that we're reading a new file and update the remaining duration
	p.addDuration(getDuration(f))

	old, err := p.setFile(f)
	if err != nil {
		return err
	}
	defer p.resetFile(old)

	scanner := newKBScanner(f)

	for scanner.Scan() {
		// Parse the event
		s := strings.SplitN(scanner.Text(), ":", 2)

		// Skip malformed commands and blank lines
		if len(s) != 2 {
			log.Debug("malformed vnc command: %s", scanner.Text())
			continue
		}

		// Ignore comments
		if strings.HasPrefix(s[0], "#") {
			log.Info("playback: %s", scanner.Text())
			continue
		}

		res, err := parseEvent(s[1])
		if err != nil {
			log.Error("invalid vnc message: `%s`", s[1])
			continue
		}

		// Set the current event context
		p.setStep(scanner.Text())

		duration, err := parseDuration(s[0])
		if err != nil {
			log.Errorln(err)
			continue
		}

		if err := p.wait(f, duration); err != nil {
			return err
		}

		if err := p.run(f, res); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %v: %w", f.Name(), err)
	}

	return nil
}

// run runs one event of the file f.
func (p *playback) run(f *os.File, e any) error {
	switch e := e.(type) {
	case Event:
		if !p.send(e) {
			return errStopped
		}
	case *LoadFileEvent:
		return p.loadFile(f, e.File)
	case *WaitForItEvent:
		return p.runWaitForIt(e)
	}

	return nil
}

// wait waits for d before the next event of the file f. It runs the
// injected events, holds while the playback is paused, and ends at once on
// a Step.
func (p *playback) wait(f *os.File, d time.Duration) error {
	for {
		step, err := p.control(f)
		if err != nil {
			return err
		}
		if step {
			p.addDuration(-d)
			return nil
		}
		if d <= 0 {
			return nil
		}

		start := p.clock.Now()

		select {
		case <-p.clock.After(d):
			p.addDuration(-d)
			return nil
		case <-p.wake:
			waited := min(p.clock.Now().Sub(start), d)
			p.addDuration(-waited)

			// don't need to wait as long next time
			d -= waited
		case <-p.done:
			return errStopped
		}
	}
}

// control holds while the playback is paused and runs the injected events.
// It returns true if a Step is pending.
func (p *playback) control(f *os.File) (bool, error) {
	for {
		p.Lock()
		if p.closed {
			p.Unlock()
			return false, errStopped
		}

		if p.state == Pause {
			p.Unlock()

			select {
			case <-p.wake:
				continue
			case <-p.done:
				return false, errStopped
			}
		}

		if len(p.pending) == 0 {
			step := p.step
			p.step = false
			p.Unlock()

			return step, nil
		}

		e := p.pending[0]
		p.pending = p.pending[1:]
		p.Unlock()

		if err := p.run(f, e); err != nil {
			return false, err
		}
	}
}

// runWaitForIt waits for the template of e. For a click, it sends a press and
// then a release on the center of the template.
func (p *playback) runWaitForIt(e *WaitForItEvent) error {
	// TODO: what to do for duration?
	press, err := p.waitForIt(e)
	if err != nil || !e.Click {
		return err
	}

	release := *press
	release.ButtonMask = 0

	if !p.send(press) || !p.send(&release) {
		return errStopped
	}

	return nil
}

// waitForIt waits until the template image is displayed. If it is detected
// within the timeout, returns a PointerEvent to click on the center of the
// template image.
func (p *playback) waitForIt(e *WaitForItEvent) (*PointerEvent, error) {
	log.Info("playback %v, wait for %v, timeout = %v", p.ID, e.Source, e.Timeout)

	deadline := time.After(e.Timeout)

	for {
		// drop a screenshot from before the request
		select {
		case <-p.screenshots:
		default:
		}

		width, height := p.Conn.size()
		fb := &FramebufferUpdateRequest{
			Width:  width,
			Height: height,
		}

		// request an updated screenshot
		if err := fb.Write(p.Conn); err != nil {
			return nil, err
		}

		start := time.Now()

		select {
		case screenshot := <-p.screenshots:
			log.Info("playback %v got screenshot after %v", p.ID, time.Since(start))

			if e := matchTemplate(screenshot, e.Template); e != nil {
				return e, nil
			}
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for %v", e.Source)
		case <-p.done:
			return nil, errStopped
		}

		// sleep and try again
		select {
		case <-time.After(time.Second):
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for %v", e.Source)
		case <-p.done:
			return nil, errStopped
		}
	}
}

func (p *playback) setFile(f *os.File) (old *os.File, err error) {
	p.Lock()
	defer p.Unlock()

	if p.depth >= 100 {
		return nil, errors.New("too many recursive LoadFiles")
	}

	p.depth += 1
	if p.depth > 10 {
		log.Warn("recursive LoadFiles detected in vnc playback")
	}

	old, p.file = p.file, f
	return
}

func (p *playback) resetFile(old *os.File) {
	p.Lock()
	defer p.Unlock()

	p.depth -= 1
	p.file = old
}

func (p *playback) setStep(s string) {
	p.Lock()
	defer p.Unlock()

	p.e = s
}

func (p *playback) addDuration(d time.Duration) {
	p.Lock()
	defer p.Unlock()

	p.duration += d
}

// writeEvents reads events from the out channel and writes them to the vnc
// connection. It closes the connection when the playback stops.
func (p *playback) writeEvents() {
	defer close(p.ended)
	defer p.Conn.Close()

	var logged bool
	for {
		select {
		case <-p.done:
			return
		case e := <-p.out:
			if pe, ok := e.(*kbPointerEvent); ok && pe.dropped() != 0 && !logged {
				log.Warn("playback %v drops pointer mask bits %#x: the connection does not support extended mouse buttons", p.ID, pe.dropped())
				logged = true
			}

			if err := e.Write(p.Conn); err != nil {
				if !p.Closed() {
					log.Error("unable to write vnc event: %v", err)
				}
				p.Stop()
				return
			}
		}
	}
}

// clock gives the time to the playback wait. Tests replace it.
type clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
