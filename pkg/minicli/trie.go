// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"fmt"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type patternTrieKey struct {
	Type  itemType
	Value string
}

// patternTrie indexes the patterns from all the handlers to make command
// compilation faster.
type patternTrie struct {
	Children map[patternTrieKey]*patternTrie
	Handler  *Handler
}

// Add a new handler to the patternTrie. Returns an error if there is a
// conflict.
func (p *patternTrie) Add(h *Handler) error {
	for _, v := range h.PatternItems {
		if err := p.add(v, h); err != nil {
			return err
		}
	}

	return nil
}

// Add a pattern to the patternTrie. Returns an error if there is a conflict.
func (p *patternTrie) add(v []PatternItem, h *Handler) error {
	// ran out of items
	if len(v) == 0 {
		return p.setHandler(h)
	}

	// when we search for handlers, we don't want to have to deal with optional
	// things so set the handler at this level and the next level
	if v[0].IsOptional() {
		if err := p.setHandler(h); err != nil {
			return err
		}
	}

	var keys []string

	switch {
	case v[0].IsLiteral():
		keys = append(keys, v[0].Text)
	case v[0].IsChoice():
		// explode all the options into literals
		for _, opt := range v[0].Options {
			keys = append(keys, opt)
		}
	default:
		keys = append(keys, v[0].Key)
	}

	for _, k := range keys {
		key := patternTrieKey{
			Type:  v[0].Type,
			Value: k,
		}

		if _, ok := p.Children[key]; !ok {
			p.Children[key] = &patternTrie{
				Children: make(map[patternTrieKey]*patternTrie),
			}
		}

		if err := p.Children[key].add(v[1:], h); err != nil {
			return err
		}
	}

	return nil
}

func (p *patternTrie) setHandler(h *Handler) error {
	if p.Handler != nil {
		return fmt.Errorf("conflicting handlers: %v and %v", p.Handler, h)
	}

	p.Handler = h
	return nil
}

// compile an input into a Command.
func (p *patternTrie) compile(input inputItems) *Command {
	if len(input) == 0 {
		// reached the end of the input... return a new command if there is a
		// handler at this depth
		if p.Handler != nil {
			return newCommand(p.Handler.Call)
		}

		return nil
	}

	var cmds []*Command

	for k, p2 := range p.Children {
		var c *Command

		// ignore optional flag since we have input to consume
		switch k.Type & ^optionalItem {
		case literalItem:
			// must match literal for current item
			if !strings.HasPrefix(k.Value, input[0].Value) {
				continue
			}

			c = p2.compile(input[1:])
			if c != nil {
				c.exact = c.exact && len(k.Value) == len(input[0].Value)
			}
		case stringItem:
			// current item becomes StringArg if the remainder compiles
			c = p2.compile(input[1:])
			if c != nil {
				c.StringArgs[k.Value] = input[0].Value
			}
		case choiceItem:
			// must match literal for current item
			if !strings.HasPrefix(k.Value, input[0].Value) {
				continue
			}

			// current item becomes BoolArgs if the remainder compiles
			c = p2.compile(input[1:])
			if c != nil {
				c.BoolArgs[k.Value] = true
				c.exact = c.exact && len(k.Value) == len(input[0].Value)
			}
		case listItem:
			if p2.Handler == nil {
				log.Warn("found list item without handler... odd")
				continue
			}

			// remaining items become ListArgs
			c = newCommand(p2.Handler.Call)
			c.ListArgs[k.Value] = make([]string, len(input))
			for i, v := range input {
				c.ListArgs[k.Value][i] = v.Value
			}
		case commandItem:
			if p2.Handler == nil {
				log.Warn("found command item without handler... odd")
				continue
			}

			// remaining items are compiled as a nested command
			c = newCommand(p2.Handler.Call)
			if c.Subcommand = trie.compile(input); c.Subcommand == nil {
				c = nil
			}
		default:
			log.Warn("found unknown pattern item: %v", k.Type)
		}

		if c != nil {
			cmds = append(cmds, c)
		}
	}

	var res *Command
	var exact int
	for _, cmd := range cmds {
		if cmd.exact {
			exact += 1
			// optimistic
			res = cmd
		}
	}

	if len(cmds) == 0 {
		return nil
	} else if len(cmds) == 1 {
		res = cmds[0]
	} else if exact == 1 {
		// have multiple ambiguous commands but already found the one exact
		// match with optimism
	} else if exact > 1 {
		// this shouldn't happen -- patterns should be distinct
		log.Error("found multiple exact matches, please report")
		return nil
	} else {
		// multiple ambiguous and no exact
		log.Warn("ambiguous command, found %v possibilities", len(cmds))
		return nil
	}

	flagsLock.Lock()
	defer flagsLock.Unlock()

	res.Record = defaultFlags.Record
	res.Preprocess = defaultFlags.Preprocess
	res.Original = input.String()
	return res
}

// help finds all handlers with a given prefix
func (p *patternTrie) help(input inputItems) []*Handler {
	handlers := map[*Handler]bool{}
	add := func(vals ...*Handler) {
		for _, v := range vals {
			handlers[v] = true
		}
	}
	flatten := func() []*Handler {
		var res []*Handler
		for h := range handlers {
			res = append(res, h)
		}

		return res
	}

	if len(input) == 0 {
		if p.Handler != nil {
			add(p.Handler)
		}

		for _, p2 := range p.Children {
			add(p2.help(input)...)
		}

		return flatten()
	}

	for k, p2 := range p.Children {
		switch k.Type {
		case literalItem, choiceItem:
			// must match literal for current item
			if !strings.HasPrefix(k.Value, input[0].Value) {
				continue
			}

			add(p2.help(input[1:])...)
		case stringItem, stringItem | optionalItem:
			// ignore the item itself
			add(p2.help(input[1:])...)
		case listItem, listItem | optionalItem, commandItem, commandItem | optionalItem:
			if p.Handler == nil {
				log.Warn("found list item without handler... odd")
				continue
			}

			add(p.Handler)
		default:
			log.Warn("found unknown pattern item: %v", k.Type)
		}
	}

	return flatten()
}

func (p *patternTrie) dump(depth int) {
	indent := strings.Repeat("  ", depth)
	if p.Handler != nil {
		log.Info("%vHandler: %v", indent, p.Handler.HelpShort)
	}
	log.Info("%vChildren:", indent)
	for k, v := range p.Children {
		log.Info("%v Pattern: %v", indent, k)
		v.dump(depth + 1)
	}
}
