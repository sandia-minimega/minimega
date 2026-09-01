// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	bpfDeviceBlock = 1
	bpfDeviceChar  = 2

	bpfAccessMknod = 1
	bpfAccessRead  = 2
	bpfAccessWrite = 4
)

const (
	bpfLoadWordCode      = 0x61
	bpfMoveRegisterCode  = 0xbf
	bpfRightShiftCode    = 0x77
	bpfAndCode           = 0x57
	bpfJumpNotEqualCode  = 0x55
	bpfJumpSetCode       = 0x45
	bpfMoveImmediateCode = 0xb7
	bpfExitCode          = 0x95
)

type containerDeviceRule struct {
	deviceType int32
	major      int32
	minor      int32
	access     int32
}

type bpfInstruction struct {
	code      uint8
	dstSource uint8
	offset    int16
	immediate int32
}

type bpfProgLoadAttr struct {
	progType           uint32
	instructionCount   uint32
	instructions       uint64
	license            uint64
	logLevel           uint32
	logSize            uint32
	logBuffer          uint64
	kernelVersion      uint32
	progFlags          uint32
	progName           [16]byte
	progIfIndex        uint32
	expectedAttachType uint32
}

type bpfProgAttachAttr struct {
	targetFD     uint32
	attachBpfFD  uint32
	attachType   uint32
	attachFlags  uint32
	replaceBpfFD uint32
}

func containerAttachDeviceFilter(cgroup string, devices []string) error {
	rules := make([]containerDeviceRule, 0, len(devices))
	for _, device := range devices {
		rule, err := parseContainerDeviceRule(device)
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}

	instructions := containerDeviceFilterProgram(rules)
	license := []byte("GPL\x00")
	logBuffer := make([]byte, 64*1024)
	loadAttr := bpfProgLoadAttr{
		progType:           unix.BPF_PROG_TYPE_CGROUP_DEVICE,
		instructionCount:   uint32(len(instructions)),
		instructions:       uint64(uintptr(unsafe.Pointer(&instructions[0]))),
		license:            uint64(uintptr(unsafe.Pointer(&license[0]))),
		logLevel:           1,
		logSize:            uint32(len(logBuffer)),
		logBuffer:          uint64(uintptr(unsafe.Pointer(&logBuffer[0]))),
		expectedAttachType: unix.BPF_CGROUP_DEVICE,
	}

	programFD, _, errno := unix.Syscall(unix.SYS_BPF, unix.BPF_PROG_LOAD, uintptr(unsafe.Pointer(&loadAttr)), unsafe.Sizeof(loadAttr))
	runtime.KeepAlive(instructions)
	runtime.KeepAlive(license)
	runtime.KeepAlive(logBuffer)
	if errno != 0 {
		return fmt.Errorf("loading cgroup device BPF program: %v: %s", errno, strings.TrimRight(string(logBuffer), "\x00"))
	}
	defer unix.Close(int(programFD))

	cgroupFile, err := os.Open(cgroup)
	if err != nil {
		return fmt.Errorf("opening cgroup %q: %v", cgroup, err)
	}
	defer cgroupFile.Close()

	attachAttr := bpfProgAttachAttr{
		targetFD:    uint32(cgroupFile.Fd()),
		attachBpfFD: uint32(programFD),
		attachType:  unix.BPF_CGROUP_DEVICE,
	}
	_, _, errno = unix.Syscall(unix.SYS_BPF, unix.BPF_PROG_ATTACH, uintptr(unsafe.Pointer(&attachAttr)), unsafe.Sizeof(attachAttr))
	runtime.KeepAlive(cgroupFile)
	if errno != 0 {
		return fmt.Errorf("attaching cgroup device BPF program: %v", errno)
	}

	return nil
}

func parseContainerDeviceRule(value string) (containerDeviceRule, error) {
	fields := strings.Fields(value)
	if len(fields) != 3 {
		return containerDeviceRule{}, fmt.Errorf("invalid device rule %q", value)
	}

	var rule containerDeviceRule
	switch fields[0] {
	case "b":
		rule.deviceType = bpfDeviceBlock
	case "c":
		rule.deviceType = bpfDeviceChar
	default:
		return containerDeviceRule{}, fmt.Errorf("invalid device type in rule %q", value)
	}

	device := strings.Split(fields[1], ":")
	if len(device) != 2 {
		return containerDeviceRule{}, fmt.Errorf("invalid device number in rule %q", value)
	}

	var err error
	rule.major, err = parseContainerDeviceNumber(device[0])
	if err != nil {
		return containerDeviceRule{}, fmt.Errorf("invalid major number in rule %q: %v", value, err)
	}
	rule.minor, err = parseContainerDeviceNumber(device[1])
	if err != nil {
		return containerDeviceRule{}, fmt.Errorf("invalid minor number in rule %q: %v", value, err)
	}

	for _, access := range fields[2] {
		switch access {
		case 'm':
			rule.access |= bpfAccessMknod
		case 'r':
			rule.access |= bpfAccessRead
		case 'w':
			rule.access |= bpfAccessWrite
		default:
			return containerDeviceRule{}, fmt.Errorf("invalid access in device rule %q", value)
		}
	}
	if rule.access == 0 {
		return containerDeviceRule{}, fmt.Errorf("missing access in device rule %q", value)
	}

	return rule, nil
}

func parseContainerDeviceNumber(value string) (int32, error) {
	if value == "*" {
		return -1, nil
	}

	number, err := strconv.ParseInt(value, 10, 32)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("expected non-negative integer or *")
	}
	return int32(number), nil
}

func containerDeviceFilterProgram(rules []containerDeviceRule) []bpfInstruction {
	// A cgroup device context encodes access in the upper 16 bits and device
	// type in the lower 16 bits. Keep those values and major/minor in registers
	// while testing each allow-list rule.
	instructions := []bpfInstruction{
		bpfLoadWord(2, 1, 0),
		bpfMoveRegister(3, 2),
		bpfAnd(3, 0xffff),
		bpfRightShift(2, 16),
		bpfLoadWord(4, 1, 4),
		bpfLoadWord(5, 1, 8),
	}

	for _, rule := range rules {
		start := len(instructions)
		instructions = append(instructions, bpfJumpNotEqual(3, rule.deviceType))
		if rule.major >= 0 {
			instructions = append(instructions, bpfJumpNotEqual(4, rule.major))
		}
		if rule.minor >= 0 {
			instructions = append(instructions, bpfJumpNotEqual(5, rule.minor))
		}
		if denied := int32(bpfAccessMknod|bpfAccessRead|bpfAccessWrite) &^ rule.access; denied != 0 {
			instructions = append(instructions, bpfJumpSet(2, denied))
		}
		instructions = append(instructions, bpfMoveImmediate(0, 1), bpfExit())

		next := len(instructions)
		for i := start; i < next-2; i++ {
			instructions[i].offset = int16(next - i - 1)
		}
	}

	return append(instructions, bpfMoveImmediate(0, 0), bpfExit())
}

func bpfLoadWord(destination, source uint8, offset int16) bpfInstruction {
	return bpfInstruction{code: bpfLoadWordCode, dstSource: bpfRegisters(destination, source), offset: offset}
}

func bpfMoveRegister(destination, source uint8) bpfInstruction {
	return bpfInstruction{code: bpfMoveRegisterCode, dstSource: bpfRegisters(destination, source)}
}

func bpfRightShift(destination uint8, immediate int32) bpfInstruction {
	return bpfInstruction{code: bpfRightShiftCode, dstSource: bpfRegisters(destination, 0), immediate: immediate}
}

func bpfAnd(destination uint8, immediate int32) bpfInstruction {
	return bpfInstruction{code: bpfAndCode, dstSource: bpfRegisters(destination, 0), immediate: immediate}
}

func bpfJumpNotEqual(destination uint8, immediate int32) bpfInstruction {
	return bpfInstruction{code: bpfJumpNotEqualCode, dstSource: bpfRegisters(destination, 0), immediate: immediate}
}

func bpfJumpSet(destination uint8, immediate int32) bpfInstruction {
	return bpfInstruction{code: bpfJumpSetCode, dstSource: bpfRegisters(destination, 0), immediate: immediate}
}

func bpfMoveImmediate(destination uint8, immediate int32) bpfInstruction {
	return bpfInstruction{code: bpfMoveImmediateCode, dstSource: bpfRegisters(destination, 0), immediate: immediate}
}

func bpfExit() bpfInstruction {
	return bpfInstruction{code: bpfExitCode}
}

func bpfRegisters(destination, source uint8) uint8 {
	return destination | source<<4
}
