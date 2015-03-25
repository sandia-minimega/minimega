// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"telnet"
)

var (
	prompt = "Switched CDU: "
)

// Implementation of a specific Server Tech PDU.
// This should hopefully work for any Server Tech device with an
// up-to-date firmware interface
type ServerTechPDU struct {
	username string
	password string
	c        *telnet.Conn
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewServerTechPDU(host, port, username, password string) (PDU, error) {
	var tp ServerTechPDU
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
func (p ServerTechPDU) login() error {
	// wait for login prompt
	_, err := p.c.ReadUntil("Username: ")
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
func (p ServerTechPDU) logout() error {
	// send a blank line to make sure we get a prompt
	_, err := p.c.Write([]byte("\r\n"))
	if err != nil {
		return err
	}
	_, err = p.c.ReadUntil(prompt)
	if err != nil {
		return err
	}
	_, err = p.c.Write([]byte("exit\r\n"))
	if err != nil {
		return err
	}
	return nil
}

// Log in, say "on <port>" for every port
// the user specified, and log out.
func (p ServerTechPDU) On(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil(prompt)
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("on %s\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

// Log in, say "off <port>" for every port
// the user specified, and log out.
func (p ServerTechPDU) Off(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil(prompt)
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("off %s\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

// Log in, say "reboot <port>" for every port
// the user specified, and log out.
func (p ServerTechPDU) Cycle(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil(prompt)
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("reboot %s\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

func (p ServerTechPDU) Status(ports map[string]string) error {
	p.login()
	_, err := p.c.ReadUntil(prompt)
	if err != nil {
		return err
	}
	_, err = p.c.Write([]byte("status\r\n"))
	if err != nil {
		return err
	}
	result, err := p.c.ReadUntil(prompt)
	scanner := bufio.NewScanner(bytes.NewReader(result))
	var output []string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		outlet := fields[0]
		for p, o := range ports {
			if o == outlet {
				output = append(output, fmt.Sprintf("%s: %s", p, fields[2]))
			}
		}
	}
	sort.Sort(ByNumber(output))
	for _, s := range output {
		fmt.Println(s)
	}
	if err != nil {
		return err
	}
	p.logout()
	return nil
}
