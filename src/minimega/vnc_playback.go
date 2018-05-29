package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"vnc"
)

type vncPlayer struct {
	m map[string]*vncKBPlayback

	sync.RWMutex // embed
}

type Event interface {
	Write(w io.Writer) error
}

// VNC playback control
type Control int

const (
	Play Control = iota
	Pause
	Step
	LoadFile
)

type vncSignal struct {
	kind Control
	data string
}

type vncKBPlayback struct {
	// Embedded
	*vncClient

	out    chan Event
	signal chan vncSignal

	sync.Mutex               // guards below
	depth      int           // how nested we are in LoadFiles
	duration   time.Duration // total playback duration
	e          string        // current event
	state      Control       // playback state, only Play or Pause
	closed     bool          // set after playback closed
}

// writeEvents reads events from the out channel and write them to the vnc
// connection. Closes the connection when it drains the channel.
func (v *vncKBPlayback) writeEvents() {
	defer v.Conn.Close()

	for e := range v.out {
		if err := e.Write(v.Conn); err != nil {
			log.Error("unable to write vnc event: %v", err)
			break
		}
	}

	// stop ourselves in a separate goroutine to avoid a deadlock
	go v.Stop()

	for range v.out {
		// drain the channel
	}
}

func NewVncKbPlayback(c *vncClient) *vncKBPlayback {
	return &vncKBPlayback{
		vncClient: c,
		out:       make(chan Event),
		signal:    make(chan vncSignal),
		state:     Play,
	}
}

// Creates a new VNC connection, the initial playback reader, and starts the
// vnc playback
func (v *vncPlayer) PlaybackKB(vm *KvmVM, filename string) error {
	v.Lock()
	defer v.Unlock()

	// clear out any old playbacks
	v.reap()

	return v.playbackKB(vm, filename)
}

func (v *vncPlayer) playbackKB(vm *KvmVM, filename string) error {
	// Is this playback already running?
	if _, ok := v.m[vm.Name]; ok {
		return fmt.Errorf("kb playback %v already playing", vm.Name)
	}

	c, err := DialVNC(vm)
	if err != nil {
		return err
	}

	p := NewVncKbPlayback(c)
	v.m[c.ID] = p

	return p.Start(filename)
}

func (v *vncPlayer) Inject(vm *KvmVM, s string) error {
	v.Lock()
	defer v.Unlock()

	// clear out any old playbacks
	v.reap()

	if p := v.m[vm.Name]; p != nil {
		return p.Inject(s)
	}

	e, err := parseEvent(s)
	if err != nil {
		return err
	}

	if event, ok := e.(Event); ok {
		// VNC keyboard or mouse event
		return vncInject(vm, event)
	}

	// This is an injected LoadFile event without a running playback. This is
	// equivalent to starting a new vnc playback.
	return v.playbackKB(vm, e.(string))
}

// Clear stops all playbacks
func (v *vncPlayer) Clear() {
	v.Lock()
	defer v.Unlock()

	for k, p := range v.m {
		log.Debug("stopping kb playback for %v", k)
		if err := p.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(v.m, k)
	}
}

func (v *vncPlayer) reap() {
	for k, p := range v.m {
		if p.Closed() {
			delete(v.m, k)
		}
	}
}

func (v *vncKBPlayback) playFile(parent *os.File, filename string) error {
	if !filepath.IsAbs(filename) && parent != nil {
		// Our file is in the same directory as the parent
		filename = filepath.Join(filepath.Dir(parent.Name()), filename)
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// record that we're reading a new file and update the remaining duration
	v.addDuration(getDuration(f))

	old, err := v.setFile(f)
	if err != nil {
		return err
	}
	defer func() {
		v.resetFile(old)
	}()

	scanner := bufio.NewScanner(f)

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
			log.Info("vncplayback: %s", scanner.Text())
			continue
		}

		res, err := parseEvent(s[1])
		if err != nil {
			log.Error("invalid vnc message: `%s`", s[1])
			continue
		}

		// Set the current event context
		v.setStep(scanner.Text())

		duration, err := time.ParseDuration(s[0] + "ns")
		if err != nil {
			log.Errorln(err)
			continue
		}

		for {
			start := time.Now()

			select {
			case <-time.After(duration):
				v.addDuration(-duration)

				goto Event
			case sig, ok := <-v.signal:
				if !ok {
					// signal channel closed -- bail
					log.Info("abort playback of %v due to signal", f.Name())
					return nil
				}

				waited := start.Sub(time.Now())
				v.addDuration(-waited)

				// don't need to wait as long next time
				duration -= waited

				switch sig.kind {
				case Pause:
					sig, ok := <-v.signal
					if !ok {
						// signal channel closed -- bail
						log.Info("abort playback of %v due to signal", f.Name())
						return nil
					}

					switch sig.kind {
					case Play:
						// do nothing except keep playing
					default:
						log.Error("unexpected signal: %v", sig)
					}
				case LoadFile:
					if err := v.playFile(f, sig.data); err != nil {
						return err
					}
				case Step:
					// decrease by the remaining
					v.addDuration(-duration)

					goto Event
				default:
					log.Error("unexpected signal: %v", sig)
				}
			}
		}

		// waited so process the event
	Event:
		switch event := res.(type) {
		case Event:
			v.out <- event
		case string:
			if err := v.playFile(f, event); err != nil {
				return err
			}
		}
	}

	return nil
}

func parseEvent(cmd string) (interface{}, error) {
	var e Event
	var res string
	var err error

	if e, err = vnc.ParseKeyEvent(cmd); err == nil {
		return e, err
	} else if e, err = vnc.ParsePointerEvent(cmd); err == nil {
		return e, err
	} else if res, err = ParseLoadFileEvent(cmd); err == nil {
		return res, err
	} else {
		return nil, errors.New("invalid event specified")
	}
}

func ParseLoadFileEvent(arg string) (string, error) {
	var filename string

	format := "LoadFile,%s"
	_, err := fmt.Sscanf(arg, format, &filename)
	if err != nil {
		return "", err
	}

	return filename, nil
}

func (v *vncKBPlayback) Closed() bool {
	v.Lock()
	defer v.Unlock()

	return v.closed
}

func (v *vncKBPlayback) Info() []string {
	v.Lock()
	defer v.Unlock()

	if v.closed {
		return nil
	}

	res := []string{
		v.VM.Name,
		"playback kb",
	}

	if v.state == Pause {
		res = append(res, "PAUSED")
	} else {
		res = append(res, fmt.Sprintf("%v remaining", v.duration))
	}

	res = append(res, v.file.Name())

	return res
}

func (v *vncKBPlayback) Start(filename string) error {
	v.Lock()
	defer v.Unlock()

	err := (&vnc.SetEncodings{
		Encodings: []int32{vnc.CursorPseudoEncoding},
	}).Write(v.Conn)

	if err != nil {
		log.Error("unable to set encodings: %v", err)
		return err
	}

	v.start = time.Now()
	v.state = Play

	go v.writeEvents()
	go func() {
		if err := v.playFile(nil, filename); err != nil {
			log.Error("playback failed: %v", err)
		}

		// finished producing -- close output so the underlying connection
		// closes (see writeEvents)
		close(v.out)

		// finished with this playback
		v.Stop()
	}()

	return nil
}

func (v *vncKBPlayback) Step() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Play || v.closed {
		return errors.New("playback not stepable")
	}

	v.signal <- vncSignal{kind: Step}

	return nil
}

func (v *vncKBPlayback) Pause() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Play || v.closed {
		return errors.New("playback not pauseable")
	}

	v.signal <- vncSignal{kind: Pause}
	v.state = Pause

	return nil
}

func (v *vncKBPlayback) Continue() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Pause || v.closed {
		return errors.New("playback not playable")
	}

	v.signal <- vncSignal{kind: Play}
	v.state = Play

	return nil
}

func (v *vncKBPlayback) Stop() error {
	v.Lock()
	defer v.Unlock()

	if v.closed {
		return errors.New("playback has already stopped")
	}

	close(v.signal)
	v.closed = true

	return nil
}

func (v *vncKBPlayback) Inject(cmd string) error {
	v.Lock()
	defer v.Unlock()

	if v.closed {
		return errors.New("playback has already stopped")
	}

	e, err := parseEvent(cmd)
	if err != nil {
		return err
	}

	if event, ok := e.(Event); ok {
		v.out <- event
	} else {
		v.signal <- vncSignal{kind: LoadFile, data: e.(string)}
	}

	return nil
}

func (v *vncKBPlayback) GetStep() (string, error) {
	v.Lock()
	defer v.Unlock()

	if v.closed {
		return "", errors.New("playback has already stopped")
	}

	return v.e, nil
}

func (v *vncKBPlayback) setFile(f *os.File) (old *os.File, err error) {
	v.Lock()
	defer v.Unlock()

	v.depth += 1
	if v.depth > 10 {
		log.Warn("recursive LoadFiles detected in vnc playback")
	}
	if v.depth > 100 {
		return nil, errors.New("too many recursive LoadFiles")
	}

	old, v.file = v.file, f
	return
}

func (v *vncKBPlayback) resetFile(old *os.File) {
	v.Lock()
	defer v.Unlock()

	v.depth -= 1
	v.file = old
}

func (v *vncKBPlayback) setStep(s string) {
	v.Lock()
	defer v.Unlock()

	v.e = s
}

func (v *vncKBPlayback) addDuration(d time.Duration) {
	v.Lock()
	defer v.Unlock()

	v.duration += d
}

// Returns the duration of a given kbrecording file
func getDuration(f *os.File) time.Duration {
	defer f.Seek(0, 0)

	d := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.SplitN(scanner.Text(), ":", 2)
		// Ignore blank and malformed lines
		if len(s) != 2 {
			log.Debug("malformed vnc statement: %s", scanner.Text())
			continue
		}

		// Ignore comments in the vnc file
		if s[0] == "#" {
			continue
		}

		i, err := strconv.Atoi(s[0])
		if err != nil {
			log.Errorln(err)
			return 0
		}
		d += i
	}

	return time.Duration(d) * time.Nanosecond
}
