// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"fmt"
	"strings"

	log "minilog"
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

	if len(cmds) == 1 {
		cmds[0].Original = input.String()
		return cmds[0]
	} else if len(cmds) > 1 {
		log.Warn("ambiguous command, found %v possibilities", len(cmds))
	}

	return nil
}

// help finds all handlers with a given prefix
//
// TODO: remove duplicates
func (p *patternTrie) help(input inputItems) []*Handler {
	var res []*Handler

	if len(input) == 0 {
		if p.Handler != nil {
			res = append(res, p.Handler)
		}

		for _, p2 := range p.Children {
			res = append(res, p2.help(input)...)
		}

		return res
	}

	for k, p2 := range p.Children {
		switch k.Type {
		case literalItem, choiceItem:
			// must match literal for current item
			if !strings.HasPrefix(k.Value, input[0].Value) {
				continue
			}

			res = append(res, p2.help(input[1:])...)
		case stringItem, stringItem | optionalItem:
			// ignore the item itself
			res = append(res, p2.help(input[1:])...)
		case listItem, listItem | optionalItem, commandItem, commandItem | optionalItem:
			if p.Handler == nil {
				log.Warn("found list item without handler... odd")
				continue
			}

			res = append(res, p.Handler)
		default:
			log.Warn("found unknown pattern item: %v", k.Type)
		}
	}

	return res
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
