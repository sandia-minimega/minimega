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
	Close Control = iota
	Pause
	Play
	Loadf
	Step
)

type vncSignal struct {
	kind Control
	data string
}

type vncControl struct {
	in      chan Event   // Receives parsed events from playFile()
	out     chan Event   // Sends events to the vnc connection in playEvents()
	control chan Control // Used to play, pause, and stop playbacks
}

// Encapsulates the active playback file
type PlaybackReader struct {
	scanner *bufio.Scanner
	file    *os.File
}

type vncKBPlayback struct {
	// Embedded
	*vncClient
	*vncControl
	sync.Mutex

	paused   time.Time
	duration time.Duration // Total playback duration

	prs []*PlaybackReader
	err error

	signal chan vncSignal

	// Current event
	e string

	state Control
}

// Playback's control loop. Listens for sends on both the control
// channel and the event channel. A pause on the control channel will cause the
// goroutine to block until it receives a resume. A close will teardown the
// running playback. Otherwise, events received on in are sent to the out
// channel.
func NewVncControl(in chan Event) *vncControl {
	c := &vncControl{
		in:      in,
		out:     make(chan Event),
		control: make(chan Control),
	}

	go func() {
		defer close(c.out)
		defer close(c.control)

		for {
			select {
			case v := <-c.control:
				if v == Close {
					return
				}
				// Pause, block until resumed or closed
				v = <-c.control
				if v == Close {
					return
				}
			// Receive events and send to out
			case e := <-c.in:
				if e != nil {
					c.out <- e
				}
			}
		}
	}()
	return c
}

// playEvents runs in a goroutine and writes events read from the out channel
// to the vnc connection. The control channel will close the out channel when
// the playback stops. This goroutine is also responsible for closing the vnc
// connection once the playback is terminated.
func (v *vncKBPlayback) playEvents() {
	for {
		e, more := <-v.out
		if more {
			v.err = e.Write(v.Conn)
			if v.err != nil {
				log.Warn(v.err.Error())
			}
		} else {
			v.vncClient.Stop()
			return
		}
	}
}

func NewVncKbPlayback(c *vncClient, pr *PlaybackReader) *vncKBPlayback {
	kbp := &vncKBPlayback{
		vncClient:  c,
		duration:   getDuration(pr.file.Name()),
		vncControl: NewVncControl(make(chan (Event))),
		prs:        []*PlaybackReader{pr},
		signal:     make(chan vncSignal),
		state:      Play,
	}
	return kbp
}

// Creates a new VNC connection, the initial playback reader, and starts the
// vnc playback
func (v *vncPlayer) PlaybackKB(vm *KvmVM, filename string) error {
	v.Lock()
	defer v.Unlock()

	return v.playbackKB(vm, filename)
}

func (v *vncPlayer) playbackKB(vm *KvmVM, filename string) error {
	// Is this playback already running?
	if _, ok := v.m[vm.Name]; ok {
		return fmt.Errorf("kb playback %v already playing", vm.Name)
	}

	c, err := NewVNCClient(vm)
	if err != nil {
		return err
	}

	c.file, err = os.Open(filename)
	if err != nil {
		return err
	}

	c.Conn, err = vnc.Dial(c.Rhost)
	if err != nil {
		return err
	}

	pr := &PlaybackReader{
		file:    c.file,
		scanner: bufio.NewScanner(c.file),
	}

	p := NewVncKbPlayback(c, pr)

	v.m[c.ID] = p

	go p.Play()
	return nil
}

func (v *vncPlayer) Inject(vm *KvmVM, s string) error {
	v.Lock()
	defer v.Unlock()

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

// Reads the vnc playback file and sends parsed events on the playback's in
// channel. Because we support embedding LoadFile events there can be multiple
// playbackReaders in the maps prs. When a LoadFile is encountered during the
// playback, the loadFile function is called and the playback continues at the
// outerLoop label.
func (v *vncKBPlayback) playFile() {
	v.start = time.Now()
	defer close(v.in)
	defer close(v.signal)

fileLoop:
	for {
		for _, pr := range v.prs {
			// Update the file we are playing
			v.file = pr.file

			for pr.scanner.Scan() && v.err == nil {
				// Parse the event
				s := strings.SplitN(pr.scanner.Text(), ":", 2)

				// Skip malformed commands and blank lines
				if len(s) != 2 {
					log.Debug("malformed vnc command: %s", pr.scanner.Text())
					continue
				}

				// Ignore comments
				if s[0] == "#" {
					log.Info("vncplayback: %s", s)
					continue
				}

				res, err := parseEvent(s[1])
				if err != nil {
					log.Error("invalid vnc message: `%s`", s[1])
					continue
				}

				// Set the current event context
				v.e = pr.scanner.Text()

				duration, err := time.ParseDuration(s[0] + "ns")
				if err != nil {
					log.Errorln(err)
					continue
				}

				wait := time.After(duration)
				t := time.Now()

				select {
				case <-v.done:
					return
				case sig := <-v.signal:
					// Injected LoadFile event
					if sig.kind == Loadf {
						err = v.loadFile(sig.data)
						if err != nil {
							log.Error(err.Error())
						} else {
							continue fileLoop
						}
					}
					// Step to the next event
					if sig.kind == Step {
						v.duration -= duration - time.Since(t)
					}
				// Wait for the duration
				case <-wait:
				}

				switch event := res.(type) {
				case Event:
					// Vnc event
					select {
					case <-v.done:
						return
					case v.in <- event:
					}
				case string:
					// Embedded LoadFile event
					err = v.loadFile(res.(string))
					if err != nil {
						log.Error(err.Error())
					} else {
						continue fileLoop
					}
				}
			}
		}
		break
	}
	// Playback finished, stop ourselves
	go v.Stop()
	<-v.done
}

func (v *vncKBPlayback) loadFile(f string) error {
	v.Lock()
	defer v.Unlock()
	if v.state == Close {
		return nil
	}

	if !filepath.IsAbs(f) {
		// Our file is in the same directory as the parent
		f = filepath.Join(filepath.Dir(v.file.Name()), f)
	}

	var err error
	pr := &PlaybackReader{}

	// Load the new file
	pr.file, err = os.Open(f)
	if err != nil {
		return fmt.Errorf("Couldn't load VNC playback file %v: %v", f, err)
	} else {
		v.duration += getDuration(f)
		pr.scanner = bufio.NewScanner(pr.file)
	}

	v.prs = append([]*PlaybackReader{pr}, v.prs...)

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

func (v *vncKBPlayback) Play() {
	v.Lock()
	defer v.Unlock()

	err := (&vnc.SetEncodings{
		Encodings: []int32{vnc.CursorPseudoEncoding},
	}).Write(v.Conn)

	if err != nil {
		log.Error("unable to set encodings: %v", err)
		return
	}

	v.state = Play
	go v.playEvents()
	go v.playFile()
}

func (v *vncKBPlayback) Step() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Play {
		return errors.New("kbplayback currently paused, use continue to resume")
	}

	select {
	case v.signal <- vncSignal{kind: Step, data: ""}:
	default:
	}
	return nil
}

func (v *vncKBPlayback) Pause() error {
	v.Lock()
	defer v.Unlock()

	if v.state == Pause {
		return errors.New("kbplayback already paused")
	}

	v.paused = time.Now()
	v.state = Pause
	v.control <- Pause
	return nil
}

func (v *vncKBPlayback) Continue() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Pause {
		return errors.New("kbplayback already running")
	}

	// Adjust start time for the time spent paused
	v.duration += time.Since(v.paused)

	v.state = Play
	v.control <- Play
	return nil
}

func (v *vncKBPlayback) Stop() error {
	v.Lock()
	defer v.Unlock()

	if v.state == Close {
		return errors.New("kbplayback already stopped")
	}

	v.state = Close
	v.control <- Close

	close(v.done)

	// Cleanup any open playback readers
	for _, pr := range v.prs {
		pr.file.Close()
	}
	return nil
}

func (v *vncKBPlayback) Inject(cmd string) error {
	v.Lock()
	defer v.Unlock()

	if v.state == Close {
		return errors.New("kbplayback already stopped")
	}

	e, err := parseEvent(cmd)
	if err != nil {
		return err
	}

	if event, ok := e.(Event); ok {
		v.out <- event
	} else {
		v.signal <- vncSignal{kind: Loadf, data: e.(string)}
	}

	return nil
}

func (v *vncKBPlayback) GetStep() (string, error) {
	v.Lock()
	defer v.Unlock()

	if v.state == Close {
		return "", errors.New("kbplayback already stopped")
	}
	return v.e, nil
}

func (v *vncKBPlayback) timeRemaining() string {
	elapsed := time.Since(v.start)
	return (v.duration - elapsed).String()
}

// Returns the duration of a given kbrecording file
func getDuration(filename string) time.Duration {
	d := 0

	f, _ := os.Open(filename)
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

	duration, err := time.ParseDuration(strconv.Itoa(d) + "ns")
	if err != nil {
		log.Errorln(err)
		return 0
	}

	return duration
}
