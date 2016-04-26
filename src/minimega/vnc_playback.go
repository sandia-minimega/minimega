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

var vncKBPlaying map[string]*vncKBPlayback

type Event interface {
	Write(w io.Writer) error
}

type Control int

const (
	Close Control = iota
	Pause
	Play
)

type Chan struct {
	in, out chan Event
	control chan Control
}

func init() {
	vncKBPlaying = make(map[string]*vncKBPlayback)
}

type vncKBPlayback struct {
	// Embedded
	*vncClient
	*Chan
	sync.Mutex

	duration time.Duration
	paused   time.Time

	scanner *bufio.Scanner
	err     error

	step chan bool

	state Control
}

func NewChan(in chan Event) *Chan {
	out := make(chan Event)
	control := make(chan Control)

	c := &Chan{
		in:      in,
		out:     out,
		control: control,
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
				// Block until resumed or closed
				v = <-c.control
				if v == Close {
					return
				}
			// Receive events on e
			case e := <-c.in:
				c.out <- e
			}
		}
	}()

	return c
}

func NewVncKbPlayback(c *vncClient) *vncKBPlayback {
	d := duration(c.file.Name())
	in := make(chan Event)
	ch := NewChan(in)

	kbp := &vncKBPlayback{
		vncClient: c,
		Chan:      ch,
		duration:  d,
		scanner:   bufio.NewScanner(c.file),
		step:      make(chan bool),
		state:     Play,
	}

	return kbp
}

func vncPlaybackKB(host, vm, filename string) error {
	c, err := NewVNCClient(host, vm)
	if err != nil {
		return err
	}

	// is this rhost already being recorded?
	if _, ok := vncKBPlaying[c.Rhost]; ok {
		return fmt.Errorf("kb playback for %v %v already running", host, vm)
	}

	c.file, err = os.Open(filename)
	if err != nil {
		return err
	}

	c.Conn, err = vnc.Dial(c.Rhost)
	if err != nil {
		return err
	}

	p := NewVncKbPlayback(c)

	vncKBPlaying[c.Rhost] = p

	go p.Play()

	return nil
}

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

func (v *vncKBPlayback) playFile() {
	v.start = time.Now()

	for v.scanner.Scan() && v.err == nil {
		s := strings.SplitN(v.scanner.Text(), ":", 2)

		duration, err := time.ParseDuration(s[0] + "ns")
		if err != nil {
			log.Errorln(err)
			continue
		}

		wait := time.After(duration)
		select {
		case <-v.done:
			return
		case <-wait:
		case <-v.step:
			// TODO fix time
		}

		if res, err := vnc.ParseKeyEvent(s[1]); err == nil {
			select {
			case v.in <- res:
			case <-v.done:
				return
			}
		} else if res, err := vnc.ParsePointerEvent(s[1]); err == nil {
			select {
			case v.in <- res:
			case <-v.done:
				return
			}
		} else if file, err := ParseLoadFileEvent(s[1]); err == nil {
			if !filepath.IsAbs(file) {
				// Our file is in the same directory as the parent
				file = filepath.Join(filepath.Dir(v.file.Name()), file)
			}
			// Save the file we were working from
			oldfile := v.file
			// Load the new file
			v.file, err = os.Open(file)
			if err != nil {
				log.Error("Couldn't load VNC playback file %v: %v", file, err)
				v.err = err
			} else {
				r := NewVncKbPlayback(v.vncClient)
				// We will wait until this file has played fully.
				r.playFile()
			}
			v.file = oldfile
		} else {
			log.Error("invalid vnc message: `%s`", s[1])
		}
	}

	// Playback finished, stop ourselves
	go v.Stop()
	<-v.done
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
	err := (&vnc.SetEncodings{
		Encodings: []int32{vnc.CursorPseudoEncoding},
	}).Write(v.Conn)

	if err != nil {
		log.Error("unable to set encodings: %v", err)
		return
	}

	v.Lock()
	defer v.Unlock()

	v.state = Play
	go v.playEvents()
	go v.playFile()
}

func (v *vncKBPlayback) Step() error {
	v.Lock()
	defer v.Unlock()

	if v.state != Play {
		return errors.New("kbplayback currently paused, use continue")
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
	log.Info("kbplayback stopping")
	if v.state == Close {
		return errors.New("kbplayback already stopped")
	}

	v.state = Close

	// Wait for playFile before we close the in channel
	v.control <- Close
	v.done <- true
	close(v.in)

	delete(vncKBPlaying, v.Rhost)
	return nil
}

func (v *vncKBPlayback) timeRemaining() string {
	elapsed := time.Since(v.start)
	return (v.duration - elapsed).String()
}
