// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/ziutek/telnet"
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
	log.Debug("Logging into PDU")
	time.Sleep(500 * time.Millisecond)
	err := p.readTelnet("Username: ")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s\r\n", p.username)
	err = p.writeTelnet(cmd, true)
	if err != nil {
		return err
	}
	err = p.readTelnet("Password: ")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("%s\r\n", p.password)
	err = p.writeTelnet(cmd, false)
	if err != nil {
		return err
	}
	return nil
}

// Convenience function, log out.
func (p ServerTechPDU) logout() error {
	// send a blank line to make sure we get a prompt
	log.Debug("Logging out of PDU")
	err := p.writeTelnet("\r\n", true)
	if err != nil {
		return err
	}
	err = p.readTelnet(prompt)
	if err != nil {
		return err
	}
	err = p.writeTelnet("exit\r\n", true)
	if err != nil {
		return err
	}

	log.Debug("Trying to Close Connection")
	p.c.Close()
	//check if connection is closed
	err = p.readTelnet(prompt)
	log.Debug(err.Error())
	log.Debug("Connection Closed")
	return nil
}

// Log in, say "on <port>" for every port
// the user specified, and log out.
func (p ServerTechPDU) On(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		err := p.readTelnet(prompt)
		if err != nil {
			return err
		}
		err = p.writeTelnet(fmt.Sprintf("on %s\r\n", port), true)
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
		log.Debug("Attempting to power off %v", port)
		err := p.readTelnet(prompt)
		if err != nil {
			return err
		}
		err = p.writeTelnet(fmt.Sprintf("off %s\r\n", port), true)
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
		log.Debug("Attempting to power cylce %v", port)
		err := p.readTelnet(prompt)
		if err != nil {
			return err
		}
		err = p.writeTelnet(fmt.Sprintf("reboot %s\r\n", port), true)
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

func (p ServerTechPDU) Status(ports map[string]string) error {
	p.login()
	log.Debug("Attempting to get status")
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

func (p ServerTechPDU) Temp() error {
	//noop
	return nil
}

func (p ServerTechPDU) Info() error {
	//noop
	return nil
}

func (p ServerTechPDU) readTelnet(token string) error {
	dl := time.Now().Add((time.Second * 5))
	p.c.SetReadDeadline(dl)
	read, err := p.c.ReadUntil(token)
	if len(read) != 0 {
		log.Debug("contents from telnet: %v", string(read))
	}
	if err != nil {
		log.Error("Read Timedout: waiting for %v", token)
		p.c.Close()
		return err
	}
	return nil
}

func (p ServerTechPDU) writeTelnet(token string, tolog bool) error {
	if tolog {
		log.Debug("Attempting to write: %v", token)
	}
	dl := time.Now().Add((time.Second * 5))
	p.c.SetWriteDeadline(dl)
	_, err := p.c.Write([]byte(token))
	if err != nil {
		log.Error("Write Timedout")
		p.c.Close()
		return err
	}
	return nil
}
