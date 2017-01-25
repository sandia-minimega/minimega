// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// miniplumber is a package to facilitate communication pipelines between
// registered readers and writers across a distributed meshage environment.
// miniplumber supports plumbing to external programs over stdio, similar to
// unix pipelines, supports trees of communication pipelines, and configurable
// delivery options.
package miniplumber

import (
	"fmt"
	"meshage"
	log "minilog"
	"strings"
	"sync"
)

// const (
// 	ALL = iota
// 	RR
// 	RND
// )

type Plumber struct {
	node      *meshage.Node         // meshage node to use for distributed environments
	Messages  chan *meshage.Message // incoming messages from meshage
	pipes     map[string]*Pipe
	pipelines map[string]*pipeline
}

type Pipe struct {
	Name    string
	Mode    int
	readers []chan string
}

type pipeline struct {
	name       string
	production []string
	done       chan bool
	canceler   sync.Once
}

type Message struct {
}

// New returns a new Plumber object over meshage node n
func New(n *meshage.Node) *Plumber {
	p := &Plumber{
		node:      n,
		Messages:  make(chan *meshage.Message, 1024),
		pipes:     make(map[string]*Pipe),
		pipelines: make(map[string]*pipeline),
	}

	//go p.handleMessages()

	return p
}

func (p *Plumber) Plumb(production ...string) error {
	// pipelines are unique by their string name, which may be an issue
	// down the road
	name := strings.Join(production, " ")

	log.Debug("got production: %v", name)

	if _, ok := p.pipelines[name]; ok {
		return fmt.Errorf("pipeline already exists: %v", production)
	}

	p.pipelines[name] = &pipeline{
		name:       name,
		production: production,
	}

	go p.startPipeline(p.pipelines[name])

	return nil
}

// func (p *Plumber) Mode(pipe string, mode int) {}

func (p *Plumber) PipelineDelete(production ...string) error {
	name := strings.Join(production, " ")
	if pp, ok := p.pipelines[name]; !ok {
		return fmt.Errorf("no such pipeline: %v", production)
	} else {
		pp.cancel()
		return nil
	}
}

func (p *Plumber) PipelineDeleteAll() error {
	var keys []string
	for k, _ := range p.pipelines {
		keys = append(keys, k)
	}
	for _, k := range keys {
		err := p.PipelineDelete(k)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plumber) PipeDelete(pipe string) error {
	if pp, ok := p.pipes[pipe]; ok {
		// kill all of the readers
		for _, c := range pp.readers {
			close(c)
		}
		delete(p.pipes, pipe)

		return nil
	} else {
		return fmt.Errorf("no such pipe: %v", pipe)
	}
}

func (p *Plumber) PipeDeleteAll() error {
	var keys []string
	for k, _ := range p.pipes {
		keys = append(keys, k)
	}
	for _, k := range keys {
		err := p.PipeDelete(k)
		if err != nil {
			return err
		}
	}
	return nil
}

// func (p *Plumber) Pipes() ([]*Pipe, error) {
// 	return nil, nil
// }

func (p *Plumber) NewReader(pipe string) <-chan string {
	c := make(chan string)

	if pp, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			Name:    pipe,
			readers: []chan string{c},
		}
	} else {
		pp.readers = append(pp.readers, c)
	}

	return c
}

func (p *Plumber) NewWriter(pipe string) chan<- string {
	c := make(chan string)

	if _, ok := p.pipes[pipe]; !ok {
		p.pipes[pipe] = &Pipe{
			Name: pipe,
		}
	}

	go func() {
		for v := range c {
			err := p.Write(pipe, v)
			if err != nil {
				log.Errorln(err)
				break
			}
		}
	}()

	return c
}

func (p *Plumber) Write(pipe string, value string) error {
	if pp, ok := p.pipes[pipe]; ok {
		pp.write(value)
		return nil
	}
	return fmt.Errorf("no such pipe: %v", pipe)
}

func (p *Plumber) startPipeline(pl *pipeline) {
	pl.done = make(chan bool)

	defer func() {
		<-pl.done
		delete(p.pipelines, pl.name)
	}()

	var b <-chan string
	for _, e := range pl.production {
		log.Debug("starting pipeline production element: %v", e)

		// start a process if it looks like a process, otherwise create
		// a pipe

		// looks like a named pipe
		in := p.NewReader(e)
		out := p.NewWriter(e)
		b = pl.pipe(e, in, out, b)
	}
}

func (pl *pipeline) pipe(name string, pin <-chan string, pout chan<- string, in <-chan string) <-chan string {
	out := make(chan string)

	if in != nil {
		go func() {
			defer close(pout)
			defer pl.cancel()

			for {
				select {
				case v := <-in:
					select {
					case pout <- v:
					case <-pl.done:
						break
					}
				case <-pl.done:
					break
				}
			}
			log.Debug("closing input side of production element: %v: %v", pl.name, name)
		}()
	}

	go func() {
		defer close(out)
		defer pl.cancel()

		for {
			select {
			case out <- <-pin:
			case <-pl.done:
				break
			}
		}
		log.Debug("closing output side of production element: %v: %v", pl.name, name)
	}()

	return out
}

func (pl *pipeline) cancel() {
	pl.canceler.Do(func() {
		log.Debug("closing pipeline: %v", pl.name)
		close(pl.done)
	})
}

func (p *Pipe) write(value string) {
	defer func() {
		if recover() != nil {
			log.Error("pipeline closed during write: %v", value)
		}
	}()

	for _, c := range p.readers {
		c <- value
	}
}
