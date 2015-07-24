// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// NOTE: debian hosts need 'cgroup_enable=memory' added to the kernel command
// line. https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=534964

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"ipmac"
	log "minilog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"unsafe"
)

// posix capabilities. See:
// 	linux/include/linux/capability.h
// 	https://github.com/torvalds/linux/blob/master/include/linux/capability.h
//	https://github.com/torvalds/linux/blob/master/include/uapi/linux/capability.h

// includes code from:
// 	https://github.com/syndtr/gocapability/blob/master/capability/capability_linux.go

const (
	CGROUP_PATH = "/sys/fs/cgroup/minimega"
	CGROUP_ROOT = "/sys/fs/cgroup"
)

const (
	CAP_CHOWN            = uint64(1) << 0
	CAP_DAC_OVERRIDE     = uint64(1) << 1
	CAP_DAC_READ_SEARCH  = uint64(1) << 2
	CAP_FOWNER           = uint64(1) << 3
	CAP_FSETID           = uint64(1) << 4
	CAP_KILL             = uint64(1) << 5
	CAP_SETGID           = uint64(1) << 6
	CAP_SETUID           = uint64(1) << 7
	CAP_SETPCAP          = uint64(1) << 8
	CAP_LINUX_IMMUTABLE  = uint64(1) << 9
	CAP_NET_BIND_SERVICE = uint64(1) << 10
	CAP_NET_BROADCAST    = uint64(1) << 11
	CAP_NET_ADMIN        = uint64(1) << 12
	CAP_NET_RAW          = uint64(1) << 13
	CAP_IPC_LOCK         = uint64(1) << 14
	CAP_IPC_OWNDER       = uint64(1) << 15
	CAP_SYS_MODULE       = uint64(1) << 16
	CAP_SYS_RAWIO        = uint64(1) << 17
	CAP_SYS_CHROOT       = uint64(1) << 18
	CAP_SYS_PTRACE       = uint64(1) << 19
	CAP_SYS_PACCT        = uint64(1) << 20
	CAP_SYS_ADMIN        = uint64(1) << 21
	CAP_SYS_BOOT         = uint64(1) << 22
	CAP_SYS_NICE         = uint64(1) << 23
	CAP_SYS_RESOURCE     = uint64(1) << 24
	CAP_SYS_TIME         = uint64(1) << 25
	CAP_SYS_TTY_CONFIG   = uint64(1) << 26
	CAP_MKNOD            = uint64(1) << 27
	CAP_LEASE            = uint64(1) << 28
	CAP_AUDIT_WRITE      = uint64(1) << 29
	CAP_AUDIT_CONTROL    = uint64(1) << 30
	CAP_SETFCAP          = uint64(1) << 31
	CAP_MAC_OVERRIDE     = uint64(1) << 32
	CAP_MAC_ADMIN        = uint64(1) << 33
	CAP_SYSLOG           = uint64(1) << 34
	CAP_WAKE_ALARM       = uint64(1) << 35
	CAP_BLOCK_SUSPEND    = uint64(1) << 36
	CAP_AUDIT_READ       = uint64(1) << 37
	CAP_LAST_CAP         = 37
)

const (
	CAPV3 = 0x20080522
)

// DEFAULT_CAPS represents capabilities necessary for a full-system container
// and nothing more
const (
	DEFAULT_CAPS = CAP_CHOWN | CAP_DAC_OVERRIDE | CAP_FSETID | CAP_FOWNER | CAP_MKNOD | CAP_NET_RAW | CAP_SETGID | CAP_SETUID | CAP_SETFCAP | CAP_SETPCAP | CAP_NET_BIND_SERVICE | CAP_SYS_CHROOT | CAP_KILL | CAP_AUDIT_WRITE | CAP_NET_ADMIN | CAP_DAC_READ_SEARCH | CAP_AUDIT_CONTROL
)

type capHeader struct {
	version uint32
	pid     int
}

type capData struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

// only bother with version 3
type cap struct {
	header capHeader
	data   [2]capData
	bounds [2]uint32
}

const CONTAINER_FLAGS = syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_VFORK | syscall.SIGCHLD

var containerMaskPaths = []string{
	"/proc/kcore",
}

var containerReadOnlyPaths = []string{
	"/proc/sys",
	"/proc/sysrq-trigger",
	"/proc/irq",
	"/proc/bus",
}

var containerLinks = [][2]string{
	{"/proc/self/fd", "/dev/fd"},
	{"/proc/self/0", "/dev/stdin"},
	{"/proc/self/1", "/dev/stdout"},
	{"/proc/self/2", "/dev/stderr"},
}

var containerDevices []string = []string{
	"c 1:3 rwm",    // null
	"c 1:5 rwm",    // zero
	"c 1:7 rwm",    // full
	"c 5:0 rwm",    // tty
	"c 1:8 rwm",    // random
	"c 1:9 rwm",    // urandom
	"c *:* m",      // mknod any character dev
	"b *:* m",      // mknod and block dev
	"c 5:1 rwm",    // /dev/console
	"c 4:0 rwm",    // /dev/tty0
	"c 4:1 rwm",    // /dev/tty1
	"c 136:* rwm",  // pts
	"c 5:2 rwm",    // ptmx
	"c 10:200 rwm", // ?
}

type Dev struct {
	Name  string
	Major int
	Minor int
	Type  string
	Mode  int
}

var containerDeviceNames []*Dev = []*Dev{
	&Dev{
		Name:  "/dev/null",
		Major: 1,
		Minor: 3,
		Type:  "c",
		Mode:  438,
	},
	&Dev{
		Name:  "/dev/zero",
		Major: 1,
		Minor: 5,
		Type:  "c",
		Mode:  438,
	},
	&Dev{
		Name:  "/dev/full",
		Major: 1,
		Minor: 7,
		Type:  "c",
		Mode:  438,
	},
	&Dev{
		Name:  "/dev/tty",
		Major: 5,
		Minor: 0,
		Type:  "c",
		Mode:  438,
	},
	&Dev{
		Name:  "/dev/random",
		Major: 1,
		Minor: 8,
		Type:  "c",
		Mode:  438,
	},
	&Dev{
		Name:  "/dev/urandom",
		Major: 1,
		Minor: 9,
		Type:  "c",
		Mode:  438,
	},
}

type ContainerConfig struct {
	FSPath   string
	Hostname string
	Init     string
	Args     []string
}

type ContainerVM struct {
	BaseVM          // embed
	ContainerConfig // embed

	pid int
}

func (vm *ContainerVM) UpdateCCActive() {
	vm.ActiveCC = ccHasClient(vm.UUID)
}

// Ensure that ContainerVM implements the VM interface
var _ VM = (*KvmVM)(nil)

// Valid names for output masks for vm kvm info, in preferred output order
var containerMasks = []string{
	"id", "name", "state", "memory", "type", "vlan", "bridge", "tap",
	"mac", "ip", "ip6", "bandwidth", "filesystem", "snapshot", "uuid",
	"cc_active", "tags",
}

func init() {
	// Reset everything to default
	for _, fns := range containerConfigFns {
		fns.Clear(&vmConfig.ContainerConfig)
	}

	// create a minimega cgroup
	// TODO: ensure devices cgroups are mounted
	// for now just assume it's at /sys/fs/cgroup
	err := os.MkdirAll(CGROUP_PATH, 0755)
	if err != nil {
		fmt.Printf("creating minimega cgroup: %v", err)
		os.Exit(1)
	}

	// inherit cpusets
	err = ioutil.WriteFile(filepath.Join(CGROUP_ROOT, "cgroup.clone_children"), []byte("1"), 0664)
	if err != nil {
		fmt.Printf("setting cgroup: %v", err)
		os.Exit(1)
	}
}

// containers don't return screenshots
func (vm *ContainerVM) Screenshot(fpath string, size int) error {
	data, err := base64.StdEncoding.DecodeString(containerScreenshot)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fpath, data, 0644)
}

// Copy makes a deep copy and returns reference to the new struct.
func (old *ContainerConfig) Copy() *ContainerConfig {
	res := new(ContainerConfig)

	// Copy all fields
	*res = *old

	// Make deep copy of slices
	// none yet - placeholder

	return res
}

func (vm *ContainerVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func NewContainer(name string) *ContainerVM {
	vm := new(ContainerVM)

	vm.BaseVM = *NewVM(name)
	vm.Type = CONTAINER

	vm.ContainerConfig = *vmConfig.ContainerConfig.Copy() // deep-copy configured fields

	return vm
}

func (vm *ContainerVM) GetInstancePath() string {
	return vm.instancePath
}

func (vm *ContainerVM) Launch(ack chan int) error {
	go vm.launch(ack)

	return nil
}

func (vm *ContainerVM) Start() error {
	log.Debugln("HEY")
	s := vm.GetState()

	stateMask := VM_PAUSED | VM_BUILDING | VM_QUIT | VM_ERROR
	if s&stateMask == 0 {
		return nil
	}

	if s == VM_QUIT || s == VM_ERROR {
		log.Info("restarting VM: %v", vm.ID)
		ack := make(chan int)
		go vm.launch(ack)
		log.Debug("ack restarted VM %v", <-ack)
	}

	log.Info("starting VM: %v", vm.ID)

	// TODO: container unpause

	freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
	err := ioutil.WriteFile(freezer, []byte("THAWED"), 0644)
	if err != nil {
		return err
	}

	vm.setState(VM_RUNNING)

	return nil
}

func (vm *ContainerVM) Stop() error {
	if vm.GetState() != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.ID)
	}

	log.Info("stopping VM: %v", vm.ID)

	freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
	err := ioutil.WriteFile(freezer, []byte("FROZEN"), 0644)
	if err != nil {
		return err
	}

	vm.setState(VM_PAUSED)

	return nil
}

func (vm *ContainerVM) Kill() error {
	// Close the channel to signal to all dependent goroutines that they should
	// stop. Anyone blocking on the channel will unblock immediately.
	// http://golang.org/ref/spec#Receive_operator
	close(vm.kill)
	// TODO: ACK if killed?
	return nil
}

func (vm *ContainerVM) String() string {
	return fmt.Sprintf("%s:%d:container", hostname, vm.ID)
}

func (vm *ContainerVM) Info(mask string) (string, error) {
	// If it's a field handled by the baseVM, use it.
	if v, err := vm.BaseVM.info(mask); err == nil {
		return v, nil
	}

	// If it's a configurable field, use the Print fn.
	if fns, ok := containerConfigFns[mask]; ok {
		return fns.Print(&vm.ContainerConfig), nil
	}

	switch mask {
	case "cc_active":
		return fmt.Sprintf("%v", vm.ActiveCC), nil
	}

	return "", fmt.Errorf("invalid mask: %s", mask)
}

func (vm *ContainerConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current container configuration:")
	fmt.Fprintf(w, "Filesystem Path:\t%v\n", vm.FSPath)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (vm *ContainerVM) launchPreamble(ack chan int) bool {
	// check if the vm has a conflict with the disk or mac address of another vm
	// build state of currently running system
	macMap := map[string]bool{}
	selfMacMap := map[string]bool{}
	diskSnapshotted := map[string]bool{}
	diskPersistent := map[string]bool{}

	vmLock.Lock()
	defer vmLock.Unlock()

	err := os.MkdirAll(vm.instancePath, os.FileMode(0700))
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	// generate a UUID if we don't have one
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}

	// populate selfMacMap
	for _, net := range vm.Networks {
		if net.MAC == "" { // don't worry about empty mac addresses
			continue
		}

		if _, ok := selfMacMap[net.MAC]; ok {
			// if this vm specified the same mac address for two interfaces
			log.Errorln("Cannot specify the same mac address for two interfaces")
			vm.setState(VM_ERROR)
			ack <- vm.ID // signal that this vm is "done" launching
			return false
		}
		selfMacMap[net.MAC] = true
	}

	stateMask := VM_BUILDING | VM_RUNNING | VM_PAUSED

	// populate macMap, diskSnapshotted, and diskPersistent
	for _, vm2 := range vms {
		if vm == vm2 { // ignore this vm
			continue
		}

		s := vm2.GetState()

		if s&stateMask != 0 {
			// populate mac addresses set
			for _, net := range vm2.Config().Networks {
				macMap[net.MAC] = true
			}

			if vm2, ok := vm2.(*ContainerVM); ok {
				// populate disk sets
				if vm2.Snapshot {
					diskSnapshotted[vm2.FSPath] = true
				} else {
					diskPersistent[vm2.FSPath] = true
				}

			}
		}
	}

	// check for mac address conflicts and fill in unspecified mac addresses without conflict
	for i := range vm.Networks {
		net := &vm.Networks[i]

		if net.MAC == "" { // create mac addresses where unspecified
			existsOther, existsSelf, newMac := true, true, "" // entry condition/initialization
			for existsOther || existsSelf {                   // loop until we generate a random mac that doesn't conflict (already exist)
				newMac = randomMac()               // generate a new mac address
				_, existsOther = macMap[newMac]    // check it against the set of mac addresses from other vms
				_, existsSelf = selfMacMap[newMac] // check it against the set of mac addresses specified from this vm
			}

			net.MAC = newMac          // set the unspecified mac address
			selfMacMap[newMac] = true // add this mac to the set of mac addresses for this vm
		}
	}

	// check for disk conflict
	_, existsSnapshotted := diskSnapshotted[vm.FSPath]                   // check if another vm is using this disk in snapshot mode
	_, existsPersistent := diskPersistent[vm.FSPath]                     // check if another vm is using this disk in persistent mode (snapshot=false)
	if existsPersistent || (vm.Snapshot == false && existsSnapshotted) { // if we have a disk conflict
		log.Error("disk path %v is already in use by another vm.", vm.FSPath)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return false
	}

	return true
}

func (vm *ContainerVM) launch(ack chan int) {
	log.Info("launching vm: %v", vm.ID)

	s := vm.GetState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT && !vm.launchPreamble(ack) {
		return
	}

	vm.setState(VM_BUILDING)

	// write the config for this vm
	config := vm.String()
	err := ioutil.WriteFile(vm.instancePath+"config", []byte(config), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	err = ioutil.WriteFile(vm.instancePath+"name", []byte(vm.Name), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	var waitChan = make(chan int)

	// clear taps, we may have come from the quit state
	for i := range vm.Networks {
		vm.Networks[i].Tap = ""
	}

	// create and add taps if we are associated with any networks
	for i := range vm.Networks {
		net := &vm.Networks[i]

		b, err := getBridge(net.Bridge)
		if err != nil {
			log.Error("get bridge: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}

		net.Tap, err = b.TapCreate(net.VLAN)
		if err != nil {
			log.Error("create tap: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}

		updates := make(chan ipmac.IP)
		go func(vm *ContainerVM, net *NetConfig) {
			defer close(updates)
			for {
				// TODO: need to acquire VM lock?
				select {
				case update := <-updates:
					if update.IP4 != "" {
						net.IP4 = update.IP4
					} else if net.IP6 != "" && strings.HasPrefix(update.IP6, "fe80") {
						log.Debugln("ignoring link-local over existing IPv6 address")
					} else if update.IP6 != "" {
						net.IP6 = update.IP6
					}
				case <-vm.kill:
					b.iml.DelMac(net.MAC)
					return
				}
			}
		}(vm, net)

		b.iml.AddMac(net.MAC, updates)
	}

	if len(vm.Networks) > 0 {
		taps := []string{}
		for _, net := range vm.Networks {
			taps = append(taps, net.Tap)
		}

		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(taps, "\n")), 0666)
		if err != nil {
			log.Error("write instance taps file: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}
	}

	parentStdout, childStdout, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return
	}
	parentStderr, childStderr, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return
	}
	childStdin, parentStdin, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return
	}

	// launch the container
	pid, err := vm.clone()
	if err != nil {
		log.Error("clone: %v", err)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return
	}

	log.Debug("clone pid: %v", pid)

	if pid != 0 {
		// parent

		go vm.console(parentStdin, parentStdout, parentStderr)

		vm.pid = pid
		log.Debug("vm %v has pid %v", vm.ID, vm.pid)

		p, err := os.FindProcess(pid)
		if err != nil {
			log.Fatal("FindProcess: %v", err)
		}

		go func() {
			_, err := p.Wait()
			vm.setState(VM_QUIT)
			if err != nil {
				if err.Error() != "signal: killed" { // because we killed it
					log.Error("kill container: %v", err)
					vm.setState(VM_ERROR)
				}
			}
			waitChan <- vm.ID
		}()

		// we can't just return on error at this point because we'll
		// leave dangling goroutines, we have to clean up on failure
		sendKillAck := false

		// TODO: add affinity funcs for containers
		// vm.CheckAffinity()

		ack <- vm.ID

		select {
		case <-waitChan:
			log.Info("VM %v exited", vm.ID)
		case <-vm.kill:
			log.Info("Killing VM %v", vm.ID)
			p.Kill()

			// containers cannot return unless thawed, so thaw the
			// process if necessary
			vm.Start()

			<-waitChan
			sendKillAck = true // wait to ack until we've cleaned up
		}

		// TODO: umountDefaults

		for _, net := range vm.Networks {
			b, err := getBridge(net.Bridge)
			if err != nil {
				log.Error("get bridge: %v", err)
			} else {
				b.TapDestroy(net.VLAN, net.Tap)
			}
		}

		// clean up the cgroup directory
		cgroupPath := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID))
		err = os.Remove(cgroupPath)
		if err != nil {
			log.Errorln(err)
		}

		if sendKillAck {
			killAck <- vm.ID
		}
	} else {
		// child - use log.Fatalln like it's going out of style

		// set hostname
		if vm.Hostname != "" {
			_, err := exec.Command(process("hostname"), vm.Hostname).Output()
			if err != nil {
				log.Fatal("set hostname: %v", err)
			}
		}

		// setup the root fs
		err = vm.setupRoot()
		if err != nil {
			log.Fatal("setupRoot: %v", err)
		}

		// mount defaults
		err = vm.mountDefaults()
		if err != nil {
			log.Fatal("mountDefaults: %v", err)
		}

		// mknod
		err = vm.mknodDevices()
		if err != nil {
			log.Fatal("mknodDevices: %v", err)
		}

		// pseudoterminals
		err = vm.ptmx()
		if err != nil {
			log.Fatal("ptmx: %v", err)
		}

		err = syscall.Dup2(int(childStdout.Fd()), syscall.Stdout)
		if err != nil {
			log.Fatal("dup2 stdout: %v", err)
		}
		err = syscall.Dup2(int(childStderr.Fd()), syscall.Stderr)
		if err != nil {
			log.Fatal("dup2 stderr: %v", err)
		}
		err = syscall.Dup2(int(childStdin.Fd()), syscall.Stdin)
		if err != nil {
			log.Fatal("dup2 stdin: %v", err)
		}

		// symlinks
		err = vm.symlinks()
		if err != nil {
			log.Fatal("symlinks: %v", err)
		}

		// remount key paths as read-only
		err = vm.remountReadOnly()
		if err != nil {
			log.Fatal("remountReadOnly: %v", err)
		}

		// mask paths
		err = vm.maskPaths()
		if err != nil {
			log.Fatal("maskPaths: %v", err)
		}

		// setup cgroups for this vm
		err = vm.populateCgroups()
		if err != nil {
			log.Fatal("populateCgroups: %v", err)
		}

		// chdir
		err = syscall.Chdir(vm.FSPath)
		if err != nil {
			log.Fatal("chdir: %v", err)
		}

		// attempt to chroot
		err = vm.chroot()
		if err != nil {
			log.Fatal("chroot: %v", err)
		}

		// set capabilities
		err = vm.setCapabilities()
		if err != nil {
			log.Fatal("setCapabilities: %v", err)
		}

		// TODO: figure out how to freeze the vm

		// GO!
		err = syscall.Exec(vm.Init, vm.Args, nil)
		if err != nil {
			log.Fatal("Exec: %v", err)
		}

		// the new child process will exit and the parent will catch it
		log.Fatalln("how did I get here?")
	}
}

func (vm *ContainerVM) console(stdin, stdout, stderr *os.File) {
	socketPath := filepath.Join(vm.instancePath, "console")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Error("could not start unix domain socket console on vm %v: %v", vm.ID, err)
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("console socket on vm %v: %v", vm.ID, err)
			continue
		}
		log.Debug("new connection!")

		go io.Copy(conn, stdout)
		go io.Copy(conn, stderr)
		io.Copy(stdin, conn)
		log.Debug("disconnected!")
	}
}

func (vm *ContainerVM) setCapabilities() error {
	c := new(cap)
	c.header.version = CAPV3
	c.header.pid = os.Getpid()

	caps := DEFAULT_CAPS

	for i := uint(0); i < 32; i++ {
		// first word
		c.data[0].effective |= uint32(caps) & (1 << i)
		c.data[0].permitted |= uint32(caps) & (1 << i)
		c.data[0].inheritable |= uint32(caps) & (1 << i)
		c.bounds[0] |= uint32(caps) & (1 << i)

		// second word
		c.data[1].effective |= uint32(caps>>32) & (1 << i)
		c.data[1].permitted |= uint32(caps>>32) & (1 << i)
		c.data[1].inheritable |= uint32(caps>>32) & (1 << i)
		c.bounds[1] |= uint32(caps>>32) & (1 << i)
	}

	// bounding set
	var data [2]capData
	err := capget(&c.header, &data[0])
	if err != nil {
		return err
	}
	if uint32(CAP_SETPCAP)&data[0].effective != 0 {
		for i := uint(1); i <= CAP_LAST_CAP; i++ {
			if i <= 31 && c.bounds[0]&(1<<i) != 0 {
				continue
			}
			if i > 31 && c.bounds[1]&(1<<(i-32)) != 0 {
				continue
			}

			err = prctl(syscall.PR_CAPBSET_DROP, uintptr(i), 0, 0, 0)
			if err != nil {
				// Ignore EINVAL since the capability may not be supported in this system.
				if errno, ok := err.(syscall.Errno); ok && errno == syscall.EINVAL {
					err = nil
					continue
				}
				return err
			}
		}
	}

	return capset(&c.header, &c.data[0])
}

func capget(hdr *capHeader, data *capData) error {
	_, _, e1 := syscall.Syscall(syscall.SYS_CAPGET, uintptr(unsafe.Pointer(hdr)), uintptr(unsafe.Pointer(data)), 0)
	if e1 != 0 {
		return e1
	}
	return nil
}

func capset(hdr *capHeader, data *capData) error {
	_, _, e1 := syscall.Syscall(syscall.SYS_CAPSET, uintptr(unsafe.Pointer(hdr)), uintptr(unsafe.Pointer(data)), 0)
	if e1 != 0 {
		return e1
	}
	return nil
}

func prctl(option int, arg2, arg3, arg4, arg5 uintptr) error {
	_, _, e1 := syscall.Syscall6(syscall.SYS_PRCTL, uintptr(option), arg2, arg3, arg4, arg5, 0)
	if e1 != 0 {
		return e1
	}
	return nil
}

func (vm *ContainerVM) chroot() error {
	err := syscall.Mount(vm.FSPath, "/", "", syscall.MS_MOVE, "")
	if err != nil {
		log.Debug("could not MS_MOVE mount, using chroot+chdir")
	} else {
		return nil
	}
	err = syscall.Chroot(".")
	if err != nil {
		return err
	}
	return syscall.Chdir("/")
}

func (vm *ContainerVM) populateCgroups() error {
	cgroupPath := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID))
	log.Debug("using cgroupPath: %v", cgroupPath)

	err := os.MkdirAll(cgroupPath, 0755)
	if err != nil {
		return err
	}

	deny := filepath.Join(cgroupPath, "devices.deny")
	allow := filepath.Join(cgroupPath, "devices.allow")
	tasks := filepath.Join(cgroupPath, "tasks")
	memory := filepath.Join(cgroupPath, "memory.limit_in_bytes")

	// devices
	err = ioutil.WriteFile(deny, []byte("a"), 0200)
	if err != nil {
		return err
	}
	for _, a := range containerDevices {
		err = ioutil.WriteFile(allow, []byte(a), 0200)
		if err != nil {
			return err
		}
	}

	// memory
	err = ioutil.WriteFile(memory, []byte(fmt.Sprintf("%vM", vm.Memory)), 0644)
	if err != nil {
		return err
	}

	// associate the pid with these permissions
	err = ioutil.WriteFile(tasks, []byte(fmt.Sprintf("%v", os.Getpid())), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (vm *ContainerVM) maskPaths() error {
	for _, v := range containerMaskPaths {
		p := filepath.Join(vm.FSPath, v)
		err := syscall.Mount("/dev/null", p, "", syscall.MS_BIND, "")
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *ContainerVM) remountReadOnly() error {
	for _, v := range containerReadOnlyPaths {
		p := filepath.Join(vm.FSPath, v)
		err := syscall.Mount("", p, "", syscall.MS_REMOUNT|syscall.MS_RDONLY, "")
		if err == nil {
			continue // this was actually a mountpoint
		}
		err = syscall.Mount(p, p, "", syscall.MS_BIND, "")
		if err != nil {
			return err
		}
		err = syscall.Mount(p, p, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_REC|syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "")
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *ContainerVM) symlinks() error {
	for _, l := range containerLinks {
		path := filepath.Join(vm.FSPath, l[1])
		os.Remove(path)
		err := os.Symlink(l[0], path)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *ContainerVM) ptmx() error {
	path := filepath.Join(vm.FSPath, "/dev/ptmx")
	os.Remove(path)
	err := os.Symlink("pts/ptmx", path)
	if err != nil {
		return err
	}

	// bind mount /dev/console
	path = filepath.Join(vm.FSPath, "/dev/console")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	f.Close()
	return syscall.Mount("/dev/console", path, "", syscall.MS_BIND, "")
}

func (vm *ContainerVM) mknodDevices() error {
	for _, v := range containerDeviceNames {
		path := filepath.Join(vm.FSPath, v.Name)
		mode := v.Mode
		if v.Type == "c" {
			mode |= syscall.S_IFCHR
		} else {
			mode |= syscall.S_IFBLK
		}
		dev := int((v.Major << 8) | (v.Minor & 0xff) | ((v.Minor & 0xfff00) << 12))
		err := syscall.Mknod(path, uint32(mode), dev)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vm *ContainerVM) setupRoot() error {
	err := syscall.Mount("", "/", "", syscall.MS_SLAVE|syscall.MS_REC, "")
	if err != nil {
		return err
	}
	return syscall.Mount(vm.FSPath, vm.FSPath, "bind", syscall.MS_BIND|syscall.MS_REC, "")
}

func (vm *ContainerVM) mountDefaults() error {
	log.Debug("mountDefaults: %v", vm.FSPath)

	var err error

	err = syscall.Mount("proc", filepath.Join(vm.FSPath, "proc"), "proc", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("tmpfs", filepath.Join(vm.FSPath, "dev"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_STRICTATIME, "mode=755")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(vm.FSPath, "dev", "shm"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(vm.FSPath, "dev", "mqueue"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(vm.FSPath, "dev", "pts"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("tmpfs", filepath.Join(vm.FSPath, "dev", "shm"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("pts", filepath.Join(vm.FSPath, "dev", "pts"), "devpts", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("sysfs", filepath.Join(vm.FSPath, "sys"), "sysfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_RDONLY, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

// syscall clone with namespace flags
func (vm *ContainerVM) clone() (int, error) {
	// see go/src/pkg/syscall/exec_unix.go
	syscall.ForkLock.Lock()

	r1, _, err1 := syscall.RawSyscall(syscall.SYS_CLONE, uintptr(CONTAINER_FLAGS), 0, 0)

	syscall.ForkLock.Unlock()

	if err1 != 0 {
		return 0, err1
	}

	// parent gets the pid, child == 0
	return int(r1), nil
}

// update the vm state, and write the state to file
func (vm *ContainerVM) setState(s VMState) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.State = s
	err := ioutil.WriteFile(vm.instancePath+"state", []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}

const containerScreenshot = `
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
rkJggg==
`
