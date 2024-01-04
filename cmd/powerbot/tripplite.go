// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"

	"github.com/ziutek/telnet"
)

// Implementation of a specific Tripp Lite PDU.
// This should hopefully work for any Tripp Lite device with an
// up-to-date SNMPWEBCARD interface
type TrippLitePDU struct {
	//	e *expect.Expecter
	username string
	password string
	c        *telnet.Conn
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewTrippLitePDU(host, port, username, password string) (PDU, error) {
	var tp TrippLitePDU
	conn, err := telnet.Dial("tcp", host+":"+port)
	if err != nil {
		return tp, err
	}
	tp.c = conn
	tp.username = username
	tp.password = password
	return tp, err
}

// Convenience function, log in.
func (p TrippLitePDU) login() error {
	// wait for login prompt
	_, err := p.c.ReadUntil("login: ")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s\r\n", p.username)
	_, err = p.c.Write([]byte(cmd))
	if err != nil {
		return err
	}
	_, err = p.c.ReadUntil("Password: ")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("%s\r\n", p.password)
	_, err = p.c.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

// Convenience function, log out.
func (p TrippLitePDU) logout() error {
	// send a blank line to make sure we get a prompt
	_, err := p.c.Write([]byte("\r\n"))
	if err != nil {
		return err
	}
	_, err = p.c.ReadUntil("$> ")
	if err != nil {
		return err
	}
	_, err = p.c.Write([]byte("exit\r\n"))
	if err != nil {
		return err
	}
	return nil
}

// Log in, say "loadctl on -o <port> --force" for every port
// the user specified, and log out.
func (p TrippLitePDU) On(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl on -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

// Log in, say "loadctl off -o <port> --force" for every port
// the user specified, and log out.
func (p TrippLitePDU) Off(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl off -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

// Log in, say "loadctl cycle -o <port> --force" for every port
// the user specified, and log out.
func (p TrippLitePDU) Cycle(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl cycle -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

// This isn't done yet because I don't want to parse that crap yet.
// Should be pretty easy really, call "loadctl status -o" and then
// look for the lines corresponding to the listed nodes. Print only
// those lines.
func (p TrippLitePDU) Status(ports map[string]string) error {
	fmt.Println("not yet implemented")
	return nil
	// doesn't work right
	/*
		p.login()
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte("loadctl status -o\r\n"))
		if err != nil {
			return err
		}
		result, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		fmt.Println(string(result))
		p.logout()
		return nil
	*/
}

func (p TrippLitePDU) Temp() error {
	//noop
	return nil
}

func (p TrippLitePDU) Info() error {
	//noop
	return nil
}
