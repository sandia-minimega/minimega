package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"vnc"
)

var (
	vncKBPlaying     map[string]*vncKBPlayback
	vncKBPlayingLock sync.RWMutex
)

type Event interface {
	Write(w io.Writer) error
}

// Playback control
type Control int

const (
	Close Control = iota
	Pause
	Play
)

type Chan struct {
	in      chan Event   // Receives parsed events from playFile()
	out     chan Event   // Sends events to the vnc connection in playEvents()
	control chan Control // Used to play, pause, and stop playbacks
}

// Encapsulates the active playback file
type PlaybackReader struct {
	scanner *bufio.Scanner
	file    *os.File
}

func init() {
	vncKBPlaying = make(map[string]*vncKBPlayback)
}

type vncKBPlayback struct {
	// Embedded
	*vncClient
	*Chan
	sync.Mutex

	paused   time.Time
	duration time.Duration // Total playback duration

	prs []*PlaybackReader
	err error

	step chan bool

	// Current event
	e string

	state Control
}

// Playback's control loop. Listens for sends on both the control
// channel and the event channel. A pause on the control channel will cause the
// goroutine to block until it receives a resume. A close will teardown the
// running playback. Otherwise, events received on in are sent to the out
// channel.
func NewChan(in chan Event) *Chan {
	c := &Chan{
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
				c.out <- e
			}
		}
	}()

	return c
}

// playEvents runs in a goroutine and writes events read from the out channel
// to the vnc connection. The control channel will close the out channel when
// the playback stops.
func (v *vncKBPlayback) playEvents() {
	for {
		e, more := <-v.out
		if more {
			v.err = e.Write(v.Conn)
		} else {
			return
		}
	}
}

func NewVncKbPlayback(c *vncClient, pr *PlaybackReader) *vncKBPlayback {
	kbp := &vncKBPlayback{
		vncClient: c,
		duration:  getDuration(pr.file.Name()),
		Chan:      NewChan(make(chan (Event))),
		prs:       []*PlaybackReader{pr},
		step:      make(chan bool),
		state:     Play,
	}
	return kbp
}

// Creates a new VNC connection, the initial playback reader, and starts the
// vnc playback
func vncPlaybackKB(vm *KvmVM, filename string) error {
	vncKBPlayingLock.Lock()
	defer vncKBPlayingLock.Unlock()

	// Is this playback already running?
	id := fmt.Sprintf("%v:%v", vm.Namespace, vm.Name)
	if _, ok := vncKBPlaying[id]; ok {
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
	log.Warn("dialing %v", c.Rhost)
	c.Conn, err = vnc.Dial(c.Rhost)
	if err != nil {
		return err
	}

	pr := &PlaybackReader{
		file:    c.file,
		scanner: bufio.NewScanner(c.file),
	}

	p := NewVncKbPlayback(c, pr)

	vncKBPlaying[c.ID] = p

	log.Warn("added %v to playbacks", c.ID)
	go p.Play()
	return nil
}

// Reads the vnc playback file and sends parsed events on the playback's in
// channel. Because we support embedding LoadFile events there can be multiple
// playbackReaders in the maps prs. When a LoadFile is encountered during the
// playback, the loadFile function is called and the playback continues at the
// outerLoop label.
func (v *vncKBPlayback) playFile() {
	v.start = time.Now()
	defer close(v.in)

outerLoop:
	for {
		for _, pr := range v.prs {
			// Update the file we are playing
			v.file = pr.file

			for pr.scanner.Scan() && v.err == nil {
				// Parse the event
				s := strings.SplitN(pr.scanner.Text(), ":", 2)
				res, err := v.parseEvent(s[1])
				if err != nil {
					log.Error("invalid vnc message: `%s`", s[1])
					continue
				}

				// Ignore comments
				if s[0] == "#" {
					log.Info("vncplayback: %s", s)
					continue
				}

				// Set the current event context
				v.e = pr.scanner.Text()

				duration, err := time.ParseDuration(s[0] + "ns")
				if err != nil {
					log.Errorln(err)
					continue
				}

				// Wait for the computed duration
				wait := time.After(duration)
				select {
				case <-v.done:
					return
				case <-wait:
				case <-v.step:
					// TODO fix time
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
					// LoadFileEvent
					err = v.loadFile(res.(string))
					if err != nil {
						log.Error(err.Error())
					} else {
						continue outerLoop
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

func (v *vncKBPlayback) parseEvent(cmd string) (interface{}, error) {
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
	case v.step <- true:
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

	v.vncClient.Stop()

	log.Warn("deleting kbpb %v", v.ID)
	delete(vncKBPlaying, v.ID)

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

	e, err := v.parseEvent(cmd)
	if err != nil {
		return err
	}

	if event, ok := e.(Event); ok {
		v.out <- event
	} else {
		return errors.New("playback only supports injecting keyboard and mouse commands")
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
