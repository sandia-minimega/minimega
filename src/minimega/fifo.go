// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// A fifo for bytes.

package main

import (
)

type byteFifo struct {
	data []byte
	limit int // max size
}

func NewByteFifo(limit int) *byteFifo {
	b := make([]byte, 0)
	return &byteFifo{data: b, limit: limit}
}

func (bf *byteFifo) Read(b []byte) (n int, err error) {
	n = copy(b, bf.data)
	return
}

func (bf *byteFifo) Write(p []byte) (n int, err error) {
	bf.data = append(bf.data, p...)
	if len(bf.data) > bf.limit {
		bf.data = bf.data[len(bf.data)-bf.limit-1:]
	}
	n = len(p)
	return
}
