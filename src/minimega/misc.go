// minimega
//
// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	log "minilog"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
)

// generate a random ipv4 mac address and return as a string
func randomMac() string {
	b := make([]byte, 5)
	rand.Read(b)
	mac := fmt.Sprintf("00:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4])
	log.Info("generated mac: %v", mac)
	return mac
}

func hostid(s string) (string, int) {
	k := strings.Split(s, ":")
	if len(k) != 2 {
		log.Error("hostid cannot split host vmid pair: %v", k)
		return "", -1
	}
	val, err := strconv.Atoi(k[1])
	if err != nil {
		log.Errorln(err)
		return "", -1
	}
	return k[0], val
}

func cliDebug(c cliCommand) cliResponse {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Go Version:\t%v\n", runtime.Version)
	fmt.Fprintf(w, "Goroutines:\t%v\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "CGO calls:\t%v\n", runtime.NumCgoCall())
	w.Flush()

	return cliResponse{
		Response: o.String(),
	}
}
