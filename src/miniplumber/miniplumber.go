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
	"meshage"
)

const (
	ALL = iota
	RR
	RND
)

type Plumber struct {
	node       *meshage.Node         // meshage node to use for distributed environments
	Messages   chan *meshage.Message // incoming messages from meshage
	localPipes map[string]*Pipe
}

type Pipe struct {
	Pipe string
	Mode int
}

type Message struct {
}

// New returns a new Plumber object over meshage node n
func New(n *meshage.Node) *Plumber {
	p := &Plumber{
		node:       n,
		Messages:   make(chan *meshage.Message, 1024),
		localPipes: make(map[string]*Pipe),
	}

	// go p.handleMessages()

	return p
}

func (p *Plumber) Plumb(pipeline ...string) error {
	return nil
}

func (p *Plumber) Mode(pipe string, mode int) {}

func (p *Plumber) Delete(pipe string) error {
	return nil
}

func (p *Plumber) DeleteAll() error {
	return nil
}

func (p *Plumber) Pipes() ([]*Pipe, error) {
	return nil, nil
}

func (p *Plumber) NewReader(pipe string) (chan string, error) {
	return nil, nil
}

func (p *Plumber) Write(pipe string, value string) error {
	return nil
}
