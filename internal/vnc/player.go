// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// ErrPlaybackNotFound is the error of a Player operation on an id that has
// no playback.
var ErrPlaybackNotFound = errors.New("kb playback not found")

// errPasteQueueFull is the error of a paste to a VM that has
// maxQueuedPastes pastes that wait.
var errPasteQueueFull = errors.New("paste queue full")

// pasteKeyDelay is the delay before each key event of a paste. A character
// is two key events, or four with Shift. Without a delay, a large paste can
// overrun the input buffer of the guest.
const pasteKeyDelay = 10 * time.Millisecond

const (
	// maxPasteChars is the most characters that one paste can have.
	maxPasteChars = 65536

	// maxQueuedPastes is the most pastes to one VM that can wait for the
	// paste that types now.
	maxQueuedPastes = 8
)

// Player keeps track of active playbacks.
type Player struct {
	base   string // directory for relative paths of injected LoadFile events
	tmpDir string // directory for paste files; empty means os.TempDir

	mu sync.Mutex // guards below

	m      map[string]*playback
	pastes map[string]*pasteQueue
}

// pasteQueue holds the pastes to one VM that wait for the paste that types
// now.
type pasteQueue struct {
	jobs      []pasteJob
	cancelled bool
}

type pasteJob struct {
	rhost, file string
}

// NewPlayer returns a Player. Relative paths of injected LoadFile events are
// in base.
func NewPlayer(base string) *Player {
	return &Player{
		base:   base,
		m:      make(map[string]*playback),
		pastes: make(map[string]*pasteQueue),
	}
}

// Stop stops the playback of id, and drops the pastes to id that wait.
func (p *Player) Stop(id string) error {
	p.mu.Lock()
	p.cancelPastes(id)
	p.mu.Unlock()

	return p.apply(id, func(p *playback) error {
		return p.Stop()
	})
}

func (p *Player) Pause(id string) error {
	return p.apply(id, func(p *playback) error {
		return p.Pause()
	})
}

func (p *Player) Continue(id string) error {
	return p.apply(id, func(p *playback) error {
		return p.Continue()
	})
}

func (p *Player) Step(id string) error {
	return p.apply(id, func(p *playback) error {
		return p.Step()
	})
}

func (p *Player) GetStep(id string) (string, error) {
	var res string

	err := p.apply(id, func(p *playback) (err error) {
		res, err = p.GetStep()
		return
	})

	return res, err
}

func (p *Player) apply(id string, fn func(*playback) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// clear out any old playbacks
	p.reap()

	if p := p.m[id]; p != nil {
		return fn(p)
	}

	return fmt.Errorf("%w for %v", ErrPlaybackNotFound, id)
}

// Clear stops all playbacks and drops all pastes that wait.
func (p *Player) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id := range p.pastes {
		p.cancelPastes(id)
	}

	for k, pb := range p.m {
		log.Debug("stopping kb playback for %v", k)
		if err := pb.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(p.m, k)
	}
}

// reap deletes the playbacks that stopped. LOCK: Assumes p.mu is held.
func (p *Player) reap() {
	for k, pb := range p.m {
		if pb.Closed() {
			delete(p.m, k)
		}
	}
}

// Creates a new VNC connection, the initial playback reader, and starts the
// vnc playback
func (p *Player) Playback(id, rhost, filename string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.playback(id, rhost, filename)
	return err
}

// playback starts a playback of filename and registers it. It registers
// nothing if the start fails. LOCK: Assumes p.mu is held.
func (p *Player) playback(id, rhost, filename string) (*playback, error) {
	// clear out any old playbacks
	p.reap()

	// Is this playback already running?
	if _, ok := p.m[id]; ok {
		return nil, fmt.Errorf("kb playback %v already playing", id)
	}

	pb, err := newPlayback(id, rhost)
	if err != nil {
		return nil, err
	}

	if err := pb.Start(filename); err != nil {
		pb.Conn.Close()
		return nil, err
	}

	p.m[id] = pb

	return pb, nil
}

// Paste types text on id as key events. It types printable ASCII, Tab, and
// Enter, and skips other characters with one warning. Pastes to one id are
// typed in order. Paste returns the error of the first start, and it logs
// the errors of queued pastes. It does not log the text. A paste can have
// at most maxPasteChars characters, and at most maxQueuedPastes pastes to one
// id can wait.
func (p *Player) Paste(id, rhost, text string) error {
	n := utf8.RuneCountInString(text)
	log.Debug("vnc paste to %v: paste of %d characters", id, n)

	if n > maxPasteChars {
		return fmt.Errorf("paste has %d characters, limit is %d", n, maxPasteChars)
	}

	if err := p.checkQueue(id); err != nil {
		return err
	}

	file, typed, skipped, err := writePasteFile(p.tmpDir, text)
	if err != nil {
		return err
	}

	if skipped > 0 {
		log.Warn("vnc paste to %v: skipped %d characters that are not printable ASCII, Tab, or Enter", id, skipped)
	}

	if typed == 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if q := p.pastes[id]; q != nil {
		if len(q.jobs) >= maxQueuedPastes {
			removePasteFile(file)
			return errPasteQueueFull
		}

		q.jobs = append(q.jobs, pasteJob{rhost: rhost, file: file})
		return nil
	}

	pb, err := p.playback(id, rhost, file)
	if err != nil {
		removePasteFile(file)
		return err
	}

	q := &pasteQueue{}
	p.pastes[id] = q
	go p.typePastes(id, q, pb, file)

	return nil
}

// typePastes waits for the paste pb to end, and then types the queued
// pastes in order.
func (p *Player) typePastes(id string, q *pasteQueue, pb *playback, file string) {
	for {
		if pb != nil {
			// wait for the connection to close, so that the key events of
			// two pastes do not mix
			<-pb.ended
		}
		removePasteFile(file)

		p.mu.Lock()
		if q.cancelled || len(q.jobs) == 0 {
			if p.pastes[id] == q {
				delete(p.pastes, id)
			}
			p.mu.Unlock()
			return
		}

		job := q.jobs[0]
		q.jobs = q.jobs[1:]

		var err error
		pb, err = p.playback(id, job.rhost, job.file)
		p.mu.Unlock()

		if err != nil {
			log.Warn("unable to paste to vm %v: %v", id, err)
		}
		file = job.file
	}
}

// cancelPastes drops the pastes to id that wait, and removes their files.
// LOCK: Assumes p.mu is held.
func (p *Player) cancelPastes(id string) {
	q := p.pastes[id]
	if q == nil {
		return
	}

	for _, job := range q.jobs {
		removePasteFile(job.file)
	}

	q.jobs = nil
	q.cancelled = true
	delete(p.pastes, id)
}

// checkQueue returns errPasteQueueFull when maxQueuedPastes pastes to id
// wait.
func (p *Player) checkQueue(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if q := p.pastes[id]; q != nil && len(q.jobs) >= maxQueuedPastes {
		return errPasteQueueFull
	}

	return nil
}

// writePasteLines writes the kb lines that type text to w. It returns the
// count of characters that the lines type and the count that they skip. CR
// LF and CR are each one Enter.
func writePasteLines(w io.Writer, text string) (int, int, error) {
	bw := bufio.NewWriter(w)
	delay := pasteKeyDelay.Nanoseconds()

	event := func(down bool, key string) {
		fmt.Fprintf(bw, "%d:"+keyEventFmt+"\n", delay, down, key)
	}

	var typed, skipped int

	for i, c := range text {
		switch {
		case c == '\r':
			c = '\n'
		case c == '\n' && i > 0 && text[i-1] == '\r':
			continue
		}

		key, ok := pasteKey(c)
		if !ok {
			skipped++
			continue
		}
		typed++

		shift := requiresShift(key)
		if shift {
			event(true, "Shift_L")
		}
		event(true, key)
		event(false, key)
		if shift {
			event(false, "Shift_L")
		}
	}

	return typed, skipped, bw.Flush()
}

// writePasteFile writes the kb lines that type text to a new file in dir.
// It returns the file name and the counts of writePasteLines. When no
// character is typed or on an error, it removes the file and returns no
// name.
func writePasteFile(dir, text string) (string, int, int, error) {
	f, err := os.CreateTemp(dir, "mm_vnc_")
	if err != nil {
		return "", 0, 0, err
	}

	typed, skipped, err := writePasteLines(f, text)
	if err2 := f.Close(); err == nil {
		err = err2
	}
	if err != nil || typed == 0 {
		removePasteFile(f.Name())
		return "", typed, skipped, err
	}

	return f.Name(), typed, skipped, nil
}

func removePasteFile(name string) {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Warn("unable to remove paste file %v: %v", name, err)
	}
}

// Inject sends one event to id. An Event goes through the running playback,
// or else through a new connection. A LoadFile with no running playback
// starts a playback. A relative LoadFile path is in the base directory.
func (p *Player) Inject(id, rhost, s string) error {
	// check to see that we have a valid event
	e, err := parseEvent(s)
	if err != nil {
		return err
	}

	if e, ok := e.(*LoadFileEvent); ok && !filepath.IsAbs(e.File) && p.base != "" {
		e.File = filepath.Join(p.base, e.File)
	}

	p.mu.Lock()

	// clear out any old playbacks
	p.reap()

	pb := p.m[id]
	if pb == nil {
		if e, ok := e.(*LoadFileEvent); ok {
			// This is an injected LoadFile event without a running playback.
			// This is equivalent to starting a new vnc playback.
			_, err := p.playback(id, rhost, e.File)
			p.mu.Unlock()
			return err
		}
	}

	p.mu.Unlock()

	// inject to existing playback
	if pb != nil {
		return pb.InjectEvent(e)
	}

	if e, ok := e.(Event); ok {
		// VNC keyboard or mouse event
		conn, err := Dial(rhost)
		if err != nil {
			return err
		}
		defer conn.Close()

		return e.Write(conn)
	}

	switch e := e.(type) {
	case *WaitForItEvent:
		return fmt.Errorf("unhandled inject event for non-running playback: %T", e)
	default:
		return fmt.Errorf("unhandled inject event: %T", e)
	}
}

func (p *Player) Info() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// clear out any old playbacks
	p.reap()

	res := [][]string{}

	for _, v := range p.m {
		if info := v.Info(); info != nil {
			res = append(res, info)
		}
	}

	return res
}
