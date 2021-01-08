// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
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

// TODO: This is what readPacket and readRecord *should* be. When we release
// 2.3, we should kill the grotesque hacks that we are using currently.

/*
// readPacket reads a gonetflow.Packet from the Reader by first decoding a
// header and then reading the correct number of Records.
func readPacket(r io.Reader) (*gonetflow.Packet, error) {
	b := make([]byte, gonetflow.NETFLOW_HEADER_LEN)

	_, err := io.ReadAtLeast(r, b, gonetflow.NETFLOW_HEADER_LEN)
	if err != nil {
		return nil, err
	}

	p := &gonetflow.Packet{
		Header: gonetflow.DecodeHeader(b),
	}

	for i := 0; i < p.Header.Count-1; i++ {
		v, err := readRecord(r)
		if err != nil {
			return nil, err
		}

		p.Records = append(p.Records, v)
	}

	return p, nil
}

// readRecord reads a single gonetflow.Record from the Reader.
func readRecord(r io.Reader) (*gonetflow.Record, error) {
	b := make([]byte, gonetflow.NETFLOW_RECORD_LEN)

	_, err := io.ReadAtLeast(r, b, gonetflow.NETFLOW_RECORD_LEN)
	if err != nil {
		return nil, err
	}

	return gonetflow.DecodeRecord(b), nil
}
*/

// Copied from io for bufio.Reader
func ReadAtLeast(r *bufio.Reader, buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, io.ErrShortBuffer
	}
	for n < min && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	if n >= min {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

// Before we merged PR #559, there was an off-by-one error in minimega that
// cause it to omit the last byte when writing out netflow packets so we only
// read LEN-1. It is up to the caller to read the last byte, if necessary.
func readRecordXXX(r *bufio.Reader) (*gonetflow.Record, error) {
	b := make([]byte, gonetflow.NETFLOW_RECORD_LEN-1)

	_, err := ReadAtLeast(r, b, gonetflow.NETFLOW_RECORD_LEN-1)
	if err != nil {
		return nil, err
	}

	return gonetflow.DecodeRecord(b), nil
}

func readPacketXXX(r *bufio.Reader) (*gonetflow.Packet, error) {
	b := make([]byte, gonetflow.NETFLOW_HEADER_LEN)

	_, err := ReadAtLeast(r, b, gonetflow.NETFLOW_HEADER_LEN)
	if err != nil {
		return nil, err
	}

	p := &gonetflow.Packet{
		Header: gonetflow.DecodeHeader(b),
	}

	for {
		v, err := readRecordXXX(r)
		if err != nil {
			return nil, err
		}

		p.Records = append(p.Records, v)

		// Since we can't trust the Count, we will just read until we peek and see
		// what looks like a new header
		peek, err := r.Peek(3)
		if err == io.EOF {
			return p, nil
		} else if err != nil {
			return nil, err
		}

		// Only looking for netflow v5 headers
		if peek[0] == 0 && peek[1] == 5 {
			return p, nil
		}

		// Handle the off-by-one case
		if peek[1] == 0 && peek[2] == 5 {
			r.ReadByte()
			return p, nil
		}

		// consume byte of padding
		r.ReadByte()
	}
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

	r2 := bufio.NewReader(r)
	for {
		p, err := readPacketXXX(r2)
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
