// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"fmt"
	"os"
	"sync"
	"unicode"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Player keeps track of active playbacks.
type Player struct {
	mu sync.RWMutex // guards below

	m map[string]*playback
}

func NewPlayer() *Player {
	return &Player{
		m: make(map[string]*playback),
	}
}

func (p *Player) Stop(id string) error {
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

	return fmt.Errorf("kb playback not found for %v", id)
}

// Clear stops all playbacks
func (p *Player) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, pb := range p.m {
		log.Debug("stopping kb playback for %v", k)
		if err := pb.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(p.m, k)
	}
}

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
	_, err := p.playback(id, rhost, filename)
	return err
}

func (p *Player) PlaybackString(id, rhost, str string) error {
	f, err := os.CreateTemp("", "mm_vnc_")
	if err != nil {
		return err
	}

	for _, char := range str {
		// ascii 0x20 - 07E map directly to keysym values
		if char < 0x20 || char >= unicode.MaxASCII {
			return fmt.Errorf("unknown non-ascii character: %c", char)
		}
		keysym, err := xKeysymToString(uint32(char));
		if err != nil {
			return fmt.Errorf("character has no keysym mapping: %c", char)
		}
		f.WriteString(fmt.Sprintf("0:KeyEvent,true,%s\n", keysym))
		f.WriteString(fmt.Sprintf("0:KeyEvent,false,%s\n", keysym))

	}
	if err := f.Close(); err != nil {
		return err
	}

	pb, err := p.playback(id, rhost, f.Name())

	// remove file when playback is done
	go func() {
		<- pb.done
		if err := os.Remove(f.Name()); err != nil {
			log.Warn("Failed to delete temp file %s used for playback", f.Name())
		}
	}()

	return err
}

func (p *Player) playback(id, rhost, filename string) (*playback, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
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

	p.m[pb.ID] = pb

	return pb, pb.Start(filename)
}

func (p *Player) Inject(id, rhost, s string) error {
	// check to see that we have a valid event
	e, err := parseEvent(s)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// clear out any old playbacks
	p.reap()

	// inject to existing playback
	if p := p.m[id]; p != nil {
		return p.InjectEvent(e)
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
	case *LoadFileEvent:
		// This is an injected LoadFile event without a running playback. This is
		// equivalent to starting a new vnc playback.
		_, err := p.playback(id, rhost, e.File)
		return err
	case *WaitForItEvent:
		return fmt.Errorf("unhandled inject event for non-running playback: %T", e)
	default:
		return fmt.Errorf("unhandled inject event: %T", e)
	}
}

func (p *Player) Info() [][]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

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
