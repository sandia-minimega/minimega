// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"io"
	"log"
	"ssh"
	"strconv"
	"strings"
)

func isIPv6(ip string) bool {
	d := strings.Split(ip, ":")
	if len(d) > 8 || len(d) < 2 {
		return false
	}

	// if there are zero or one empty groups, and all the others are <= 16 bit hex, we're good.
	// a special case is a leading ::, as in ::1, which will generate two empty groups.
	empty := false
	for i, v := range d {
		if v == "" && i == 0 {
			continue
		}
		if v == "" && !empty {
			empty = true
			continue
		}
		if v == "" {
			return false
		}
		// check for dotted quad
		if len(d) <= 6 && i == len(d)-1 && isIPv4(v) {
			return true
		}
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 65535 {
			return false
		}
	}

	return true
}

type sshConn struct {
	Config  *ssh.ClientConfig
	Client  *ssh.ClientConn
	Session *ssh.Session
	Host    string
	Stdin   io.Writer
	Stdout  io.Reader
}

type sshPassword string

func (p sshPassword) Password(user string) (string, error) {
	return string(p), nil
}

func sshClientConnect(host, port, user, password string) (*sshConn, error) {
	sc := &sshConn{}
	sc.Config = &ssh.ClientConfig{
		User: user,
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthPassword(sshPassword(password)),
		},
	}

	// url notation requires leading and trailing [] on ipv6 addresses
	dHost := host
	if isIPv6(dHost) {
		dHost = "[" + dHost + "]"
	}

	var err error
	sc.Client, err = ssh.Dial("tcp", dHost+":"+port, sc.Config)
	if err != nil {
		log.Print(err)
		return nil, err
	}

	sc.Session, err = sc.Client.NewSession()
	if err != nil {
		log.Print(err)
		return nil, err
	}

	//	sc.Session.Stdout = &sc.StdoutBuf
	sc.Stdout, err = sc.Session.StdoutPipe()
	if err != nil {
		log.Print(err)
		return nil, err
	}
	sc.Stdin, err = sc.Session.StdinPipe()
	if err != nil {
		log.Print(err)
		return nil, err
	}

	if err := sc.Session.Shell(); err != nil {
		log.Print(err)
		return nil, err
	}

	sc.Host = host

	return sc, nil
}

func isIPv4(ip string) bool {
	d := strings.Split(ip, ".")
	if len(d) != 4 {
		return false
	}

	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 255 {
			return false
		}
	}

	return true
}
