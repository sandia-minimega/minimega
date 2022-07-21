// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qemu

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

var (
	// guards below
	mu sync.Mutex

	// name -> values
	cache = map[string]map[string]bool{}
)

type parser func(io.Reader) (map[string]bool, error)

// CPUs returns a list of supported QEMU CPUs for the specified qemu and
// machine type.
func CPUs(qemu, machine string) (map[string]bool, error) {
	name := qemu + machine + "CPUs"

	cmd := []string{qemu}
	if machine != "" {
		cmd = append(cmd, "-M", machine)
	}
	cmd = append(cmd, "-cpu", "?")

	res, err := caps(name, cmd, parseCPUs)
	if err != nil && machine == "" {
		return nil, errors.New("unable to determine valid QEMU CPUs, try configuring machine first")
	} else if err != nil {
		return nil, fmt.Errorf("unable to determine valid QEMU CPUs -- %v", err)
	}

	return res, err
}

// Machines returns a list of supported QEMU machines for the specified qemu.
func Machines(qemu string) (map[string]bool, error) {
	name := qemu + "Machines"

	cmd := []string{qemu, "-M", "?"}

	res, err := caps(name, cmd, parseMachines)
	if err != nil {
		return nil, fmt.Errorf("unable to determine valid QEMU machines -- %v", err)
	}

	return res, err
}

// NICs returns a list of supported QEMU NICs for the specified qemu and
// machine type.
func NICs(qemu, machine string) (map[string]bool, error) {
	name := qemu + machine + "NICs"

	cmd := []string{qemu}
	if machine != "" {
		cmd = append(cmd, "-M", machine)
	}
	cmd = append(cmd, "-device", "?")

	res, err := caps(name, cmd, parseNICs)
	if err != nil {
		return nil, fmt.Errorf("unable to determine valid QEMU nics -- %v", err)
	}

	return res, err
}

func caps(name string, cmd []string, fn parser) (map[string]bool, error) {
	if len(cmd) == 0 {
		return nil, errors.New("not enough args")
	}

	mu.Lock()
	defer mu.Unlock()

	// test if the key exists
	if v, ok := cache[name]; ok {
		return v, nil
	}

	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return nil, err
	}

	res, err := fn(bytes.NewReader(out))
	if err != nil {
		return nil, err
	}

	cache[name] = res
	return res, nil
}

func parseCPUs(r io.Reader) (map[string]bool, error) {
	res := map[string]bool{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if scanner.Text() == "Available CPUs:" {
			continue
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			break
		}

		switch fields[0] {
		case "x86":
			if len(fields) >= 2 {
				res[fields[1]] = true
			}
		default:
			res[fields[0]] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

func parseMachines(r io.Reader) (map[string]bool, error) {
	res := map[string]bool{}

	scanner := bufio.NewScanner(r)
	scanner.Scan() // skip header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 1 {
			break
		}

		res[fields[0]] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

func parseNICs(r io.Reader) (map[string]bool, error) {
	res := map[string]bool{}

	scanner := bufio.NewScanner(r)

	// scan until we find the Network devices section
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "Network devices:") {
			break
		}
	}

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			break
		}

		res[strings.Trim(fields[1], `",`)] = true
	}

	return res, nil
}
