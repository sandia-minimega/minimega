// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"gonetflow"
	"io"
	"os"
)

var (
	f_gunzip = flag.Bool("gunzip", false, "gunzip input file(s)")
)

func readRecord(r io.Reader) (*gonetflow.Record, error) {
	b := make([]byte, gonetflow.NETFLOW_RECORD_LEN)

	_, err := io.ReadAtLeast(r, b, gonetflow.NETFLOW_RECORD_LEN)
	if err != nil {
		return nil, err
	}

	return gonetflow.DecodeRecord(b), nil
}

func readPacket(r io.Reader) (*gonetflow.Packet, error) {
	b := make([]byte, gonetflow.NETFLOW_HEADER_LEN)

	_, err := io.ReadAtLeast(r, b, gonetflow.NETFLOW_HEADER_LEN)
	if err != nil {
		return nil, err
	}

	p := &gonetflow.Packet{
		Header: gonetflow.DecodeHeader(b),
	}

	for i := 0; i < p.Header.Count; i++ {
		v, err := readRecord(r)
		if err != nil {
			return nil, err
		}

		p.Records = append(p.Records, v)
	}

	return p, nil
}

func cat(fname string) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f

	if *f_gunzip {
		r2, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer r2.Close()
		r = r2
	}

	for {
		p, err := readPacket(r)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		fmt.Printf("%#v", p)
	}

	return nil
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Printf("USAGE: %v [OPTIONS] FILE...\n", os.Args[0])
		os.Exit(1)
	}

	for _, v := range flag.Args() {
		if err := cat(v); err != nil {
			fmt.Printf("unable to read %v: %v\n", v, err)
			os.Exit(1)
		}
	}
}
