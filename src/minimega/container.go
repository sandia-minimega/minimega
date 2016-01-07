// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// NOTES:
// debian hosts need 'cgroup_enable=memory' added to the kernel command line.
// See https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=534964
//
// posix capabilities. See:
// 	linux/include/linux/capability.h
// 	https://github.com/torvalds/linux/blob/master/include/linux/capability.h
//	https://github.com/torvalds/linux/blob/master/include/uapi/linux/capability.h
//
// includes code from:
// 	https://github.com/syndtr/gocapability/blob/master/capability/capability_linux.go

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"ipmac"
	log "minilog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"unsafe"
)

const (
	CONTAINER_MAGIC = "CONTAINER"
	CONTAINER_NONE  = "CONTAINER_NONE"
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

var (
	CGROUP_PATH string
	CGROUP_ROOT string
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

var containerMaskedPaths = []string{
	"/proc/kcore",
}

var containerReadOnlyPaths = []string{
	"/proc/sys",
	"/proc/sysrq-trigger",
	"/proc/irq",
	"/proc/bus",
}

var containerUUIDLink = "/sys/devices/virtual/dmi/id/product_uuid"

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
	Init     []string
	Fifos    int
}

type ContainerVM struct {
	BaseVM          // embed
	ContainerConfig // embed

	pid           int
	effectivePath string
	listener      net.Listener
	netns         string
}

type ContainerIniter struct {
	// Guards
	once sync.Once

	Success bool
}

var ContainerInit ContainerIniter

// Ensure that ContainerVM implements the VM interface
var _ VM = (*ContainerVM)(nil)

func init() {
	// Reset everything to default
	for _, fns := range containerConfigFns {
		fns.Clear(&vmConfig.ContainerConfig)
	}
	CGROUP_ROOT = filepath.Join(*f_base, "cgroup")
	CGROUP_PATH = filepath.Join(CGROUP_ROOT, "minimega")
}

func (c *ContainerIniter) Init() {
	c.once.Do(func() {
		if err := c.init(); err != nil {
			log.Errorln(err)
		} else {
			c.Success = true
		}
	})
}

func (_ ContainerIniter) init() error {
	// mount our own cgroup namespace to avoid having to ever ever ever
	// deal with systemd
	log.Debug("cgroup mkdir: %v", CGROUP_ROOT)

	err := os.MkdirAll(CGROUP_ROOT, 0755)
	if err != nil {
		return fmt.Errorf("cgroup mkdir: %v", err)
	}

	err = syscall.Mount("minicgroup", CGROUP_ROOT, "cgroup", 0, "")
	if err != nil {
		return fmt.Errorf("cgroup mount: %v", err)
	}

	// inherit cpusets
	err = ioutil.WriteFile(filepath.Join(CGROUP_ROOT, "cgroup.clone_children"), []byte("1"), 0664)
	if err != nil {
		return fmt.Errorf("setting cgroup: %v", err)
	}
	err = ioutil.WriteFile(filepath.Join(CGROUP_ROOT, "memory.use_hierarchy"), []byte("1"), 0664)
	if err != nil {
		return fmt.Errorf("setting use_hierarchy: %v", err)
	}

	// create a minimega cgroup
	err = os.MkdirAll(CGROUP_PATH, 0755)
	if err != nil {
		return fmt.Errorf("creating minimega cgroup: %v", err)
	}

	return nil
}

func containerTeardown() {
	if ContainerInit.Success {
		err := os.Remove(CGROUP_PATH)
		if err != nil {
			log.Errorln(err)
		}
		err = syscall.Unmount(CGROUP_ROOT, 0)
		if err != nil {
			log.Errorln(err)
		}
	}
}

// golang can't easily support the typical clone+exec method of firing off a
// child process. We need a child process to do ample setup for containers
// *post* clone.  We have two options - duplicate the forkAndExec shim in
// src/syscall to do a clone and get into a new minimega that can finish the
// container setup, or start a new minimega with an 'nsinit' C shim *before*
// the runtime starts. Docker and others use the latter, though it's not
// entirely clear why they don't just borrow the forkAndExec method. We'll use
// the forkAndExec method here.
//
// This function is a shim to finalize container setup from a running minimega
// parent. It expects to be called inside namespace isolations (mount, pid,
// etc...) with args: minimega CONTAINER hostname fsPath...
//
// A number of fds get passed to the child on specific fd numbers:
//
// 	3: logging port, closed just before exec into init
// 	4: closed by the child before exec to elect to be frozen
// 	5: closed by the parent when the child returns to allow calling exec
// 	6: stdin
// 	7: stdout
// 	8: stderr
//
// A number of arguments are passed on os.Args to configure the container:
//	0 :  minimega binary
// 	1 :  CONTAINER
//	2 :  instance path
//	3 :  vm id
//	4 :  hostname ("CONTAINER_NONE" if none)
//	5 :  filesystem path
//	6 :  memory in megabytes
//	7 :  uuid
//	8 :  number of fifos
//	9 :  init program (relative to filesystem path)
//	10:  init args
func containerShim() {
	if len(os.Args) < 10 { // 10 because init args can be nil
		os.Exit(1)
	}

	// we log to fd(3), and close it before we move on to exec ourselves
	logFile := os.NewFile(uintptr(3), "")
	log.AddLogger("file", logFile, log.DEBUG, false)

	log.Debug("containerShim: %v", os.Args)

	// dup2 stdio
	err := syscall.Dup2(6, syscall.Stdin)
	if err != nil {
		log.Fatalln(err)
	}
	err = syscall.Dup2(7, syscall.Stdout)
	if err != nil {
		log.Fatalln(err)
	}
	err = syscall.Dup2(8, syscall.Stderr)
	if err != nil {
		log.Fatalln(err)
	}

	// get args
	vmInstancePath := os.Args[2]
	vmID, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatalln(err)
	}
	vmHostname := os.Args[4]
	if vmHostname == CONTAINER_NONE {
		vmHostname = ""
	}
	vmFSPath := os.Args[5]
	vmMemory, err := strconv.Atoi(os.Args[6])
	if err != nil {
		log.Fatalln(err)
	}
	vmUUID := os.Args[7]
	vmFifos, err := strconv.Atoi(os.Args[8])
	if err != nil {
		log.Fatalln(err)
	}
	vmInit := os.Args[9:]

	// set hostname
	log.Debug("vm %v hostname", vmID)
	if vmHostname != "" {
		_, err := processWrapper("hostname", vmHostname)
		if err != nil {
			log.Fatal("set hostname: %v", err)
		}
	}

	// setup the root fs
	log.Debug("vm %v containerSetupRoot", vmID)
	err = containerSetupRoot(vmFSPath)
	if err != nil {
		log.Fatal("containerSetupRoot: %v", err)
	}

	// mount defaults
	log.Debug("vm %v containerMountDefaults", vmID)
	err = containerMountDefaults(vmFSPath)
	if err != nil {
		log.Fatal("containerMountDefaults: %v", err)
	}

	// mknod
	log.Debug("vm %v containerMknodDevices", vmID)
	err = containerMknodDevices(vmFSPath)
	if err != nil {
		log.Fatal("containerMknodDevices: %v", err)
	}

	// pseudoterminals
	log.Debug("vm %v containerPtmx", vmID)
	err = containerPtmx(vmFSPath)
	if err != nil {
		log.Fatal("containerPtmx: %v", err)
	}

	// symlinks
	log.Debug("vm %v containerSymlinks", vmID)
	err = containerSymlinks(vmFSPath)
	if err != nil {
		log.Fatal("containerSymlinks: %v", err)
	}

	// remount key paths as read-only
	log.Debug("vm %v containerRemountReadOnly", vmID)
	err = containerRemountReadOnly(vmFSPath)
	if err != nil {
		log.Fatal("containerRemountReadOnly: %v", err)
	}

	// mask uuid path
	log.Debug("uuid bind mount: %v -> %v", vmUUID, containerUUIDLink)
	err = syscall.Mount(vmUUID, filepath.Join(vmFSPath, containerUUIDLink), "", syscall.MS_BIND, "")
	if err != nil {
		log.Fatal("containerUUIDLink: %v", err)
	}

	// bind mount fifos
	log.Debug("vm %v containerFifos", vmID)
	err = containerFifos(vmFSPath, vmInstancePath, vmFifos)
	if err != nil {
		log.Fatal("containerFifos: %v", err)
	}

	// mask paths
	log.Debug("vm %v containerMaskPaths", vmID)
	err = containerMaskPaths(vmFSPath)
	if err != nil {
		log.Fatal("containerMaskPaths: %v", err)
	}

	// setup cgroups for this vm
	log.Debug("vm %v containerPopulateCgroups", vmID)
	err = containerPopulateCgroups(vmID, vmMemory)
	if err != nil {
		log.Fatal("containerPopulateCgroups: %v", err)
	}

	// chdir
	log.Debug("vm %v chdir", vmID)
	err = syscall.Chdir(vmFSPath)
	if err != nil {
		log.Fatal("chdir: %v", err)
	}

	// attempt to chroot
	log.Debug("vm %v containerChroot", vmID)
	err = containerChroot(vmFSPath)
	if err != nil {
		log.Fatal("containerChroot: %v", err)
	}

	// set capabilities
	log.Debug("vm %v containerSetCapabilities", vmID)
	err = containerSetCapabilities()
	if err != nil {
		log.Fatal("containerSetCapabilities: %v", err)
	}

	// in order to synchronize freezing the container before we call init,
	// we close fd(4) to signal the parent that we're ready to freeze. We
	// then read fd(5) in order to block for the parent. The parent will
	// freeze the child and close the other end of fd(5). Upon unfreezing,
	// the read will fail and we can move on to exec.

	log.Debug("sync for freezing")
	sync1 := os.NewFile(uintptr(4), "")
	sync2 := os.NewFile(uintptr(5), "")
	sync1.Close()
	var buf = make([]byte, 1)
	sync2.Read(buf)
	log.Debug("return from freezing")

	// close fds we don't want in init
	logFile.Close()

	// GO!
	log.Debug("vm %v exec: %v %v", vmID, vmInit)
	err = syscall.Exec(vmInit[0], vmInit, nil)
	if err != nil {
		log.Fatal("Exec: %v", err)
	}

	// the new child process will exit and the parent will catch it
	log.Fatalln("how did I get here?")
}

func containerFifos(vmFSPath string, vmInstancePath string, vmFifos int) error {
	err := os.Mkdir(filepath.Join(vmFSPath, "/dev/fifos"), 0755)
	if err != nil {
		log.Errorln(err)
		return nil
	}
	for i := 0; i < vmFifos; i++ {
		src := filepath.Join(vmInstancePath, fmt.Sprintf("fifo%v", i))
		_, err := os.Stat(src)
		if err != nil {
			log.Errorln(err)
			return err
		}

		dst := filepath.Join(vmFSPath, fmt.Sprintf("/dev/fifos/fifo%v", i))

		// dst must exist for bind mounting to work
		f, err := os.Create(dst)
		if err != nil {
			log.Errorln(err)
			return err
		}
		f.Close()
		log.Debug("bind mounting: %v -> %v", src, dst)
		err = syscall.Mount(src, dst, "", syscall.MS_BIND, "")
		if err != nil {
			log.Errorln(err)
			return err
		}
	}
	return nil
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

func (vm *ContainerVM) Launch(ack chan int) error {
	go vm.launch(ack)

	return nil
}

func (vm *ContainerVM) Start() (err error) {
	// Update the state after the lock has been released
	defer func() {
		if err != nil {
			log.Errorln(err)
			vm.setState(VM_ERROR)
		} else {
			// launch() may have put the vm in the error state, don't change that
			if vm.GetState() != VM_ERROR {
				vm.setState(VM_RUNNING)
			}
		}
	}()

	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&VM_RUNNING != 0 {
		return nil
	}

	if vm.State == VM_QUIT || vm.State == VM_ERROR {
		log.Info("relaunching VM: %v", vm.ID)

		// Create a new channel since we closed the other one to indicate that
		// the VM should quit.
		vm.kill = make(chan bool)
		ack := make(chan int)

		go vm.launch(ack)

		// Unlock so that launch can do its thing. We will block on receiving
		// on the ack channel so that we know when launch has finished and it's
		// okay to reaquire the lock.
		vm.lock.Unlock()
		log.Debug("ack restarted VM %v", <-ack)
		vm.lock.Lock()
	}

	log.Info("starting VM: %v", vm.ID)

	freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
	err = ioutil.WriteFile(freezer, []byte("THAWED"), 0644)
	if err != nil {
		return err
	}

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

func (vm *ContainerVM) launchPreamble(ack chan int) error {
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

	// populate selfMacMap
	for _, net := range vm.Networks {
		if net.MAC == "" { // don't worry about empty mac addresses
			continue
		}

		if _, ok := selfMacMap[net.MAC]; ok {
			// if this vm specified the same mac address for two interfaces
			return fmt.Errorf("Cannot specify the same mac address for two interfaces")
		}
		selfMacMap[net.MAC] = true
	}

	// populate macMap, diskSnapshotted, and diskPersistent
	for _, vm2 := range vms {
		if vm == vm2 { // ignore this vm
			continue
		}

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
		return fmt.Errorf("disk path %v is already in use by another vm.", vm.FSPath)
	}

	return nil
}

func (vm *ContainerVM) launch(ack chan int) (err error) {
	log.Info("launching vm: %v", vm.ID)

	// Update the state after the lock has been released
	defer func() {
		if err != nil {
			vm.setState(VM_ERROR)

			// Only ACK for failures since, on success, launch may block
			ack <- vm.ID
		} else {
			vm.setState(VM_BUILDING)
		}
	}()

	vm.lock.Lock()
	defer vm.lock.Unlock()

	ContainerInit.Init()
	if !ContainerInit.Success {
		err = errors.New("cannot launch container VMs -- cgroups failed to initialize")
		return
	}

	s := vm.State
	restart := s == VM_QUIT || s == VM_ERROR

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT {
		if err = vm.launchPreamble(ack); err != nil {
			log.Errorln(err)
			return
		}
	}

	// write the config for this vm
	writeOrDie(filepath.Join(vm.instancePath, "config"), vm.Config().String())
	writeOrDie(filepath.Join(vm.instancePath, "name"), vm.Name)

	var waitChan = make(chan int)

	if !restart {
		if vm.Snapshot {
			err = vm.overlayMount()
			if err != nil {
				log.Error("overlayMount: %v", err)
				return
			}
		} else {
			vm.effectivePath = vm.FSPath
		}
	}

	// the child process will communicate with a fake console using pipes
	// to mimic stdio, and a fourth pipe for logging before the child execs
	// into the init program
	// two additional pipes are needed to synchronize freezing the child
	// before it enters the container
	parentLog, childLog, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}
	childStdin, parentStdin, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}
	parentStdout, childStdout, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}
	parentStderr, childStderr, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}
	parentSync1, childSync1, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}
	childSync2, parentSync2, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		return
	}

	// create the uuid path that will bind mount into sysfs in the
	// container
	uuidPath := filepath.Join(vm.instancePath, "uuid")
	ioutil.WriteFile(uuidPath, []byte(vm.UUID+"\n"), 0400)

	// create fifos
	for i := 0; i < vm.Fifos; i++ {
		p := filepath.Join(vm.instancePath, fmt.Sprintf("fifo%v", i))
		err = syscall.Mkfifo(p, 0660)
		if err != nil {
			log.Error("fifo: %v", err)
			return
		}
	}

	//	0 :  minimega binary
	// 	1 :  CONTAINER
	//	2 :  instance path
	//	3 :  vm id
	//	4 :  hostname ("CONTAINER_NONE" if none)
	//	5 :  filesystem path
	//	6 :  memory in megabytes
	//	7 :  uuid
	//	8 :  number of fifos
	//	9 :  init program (relative to filesystem path)
	//	10:  init args
	hn := vm.Hostname
	if hn == "" {
		hn = CONTAINER_NONE
	}
	args := []string{
		os.Args[0],
		CONTAINER_MAGIC,
		vm.instancePath,
		fmt.Sprintf("%v", vm.ID),
		hn,
		vm.effectivePath,
		vm.Memory,
		uuidPath,
		fmt.Sprintf("%v", vm.Fifos),
	}
	args = append(args, vm.Init...)

	// launch the container
	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: args,
		Env:  nil,
		Dir:  "",
		ExtraFiles: []*os.File{
			childLog,
			childSync1,
			childSync2,
			childStdin,
			childStdout,
			childStderr,
		},
		SysProcAttr: &syscall.SysProcAttr{
			Cloneflags: uintptr(CONTAINER_FLAGS),
		},
	}
	err = cmd.Start()
	if err != nil {
		vm.overlayUnmount()
		log.Error("start container: %v", err)
		return
	}

	vm.pid = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.ID, vm.pid)

	// log the child
	childLog.Close()
	log.LogAll(parentLog, log.DEBUG, "containerShim")

	go vm.console(parentStdin, parentStdout, parentStderr)

	go func() {
		err := cmd.Wait()
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
	success := true
	sendKillAck := false

	// TODO: add affinity funcs for containers
	// vm.CheckAffinity()

	// network creation for containers happens /after/ the container is
	// started, as we need the PID in order to attach a veth to the
	// container side of the network namespace.
	// create and add taps if we are associated with any networks

	// expose the network namespace to iptool
	err = vm.symlinkNetns()
	if err != nil {
		log.Error("symlinkNetns: %v", err)
		vm.State = VM_ERROR
		cmd.Process.Kill()
		<-waitChan
		success = false
	}

	if success {
		for i := range vm.Networks {
			net := &vm.Networks[i]

			b, err := getBridge(net.Bridge)
			if err != nil {
				log.Error("get bridge: %v", err)
				vm.State = VM_ERROR
				cmd.Process.Kill()
				<-waitChan
				success = false
				break
			}

			net.Tap, err = b.ContainerTapCreate(net.Tap, net.VLAN, vm.netns, net.MAC, i)
			if err != nil {
				log.Error("create tap: %v", err)
				vm.State = VM_ERROR
				cmd.Process.Kill()
				<-waitChan
				success = false
				break
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
						} else if update.IP6 != "" && !strings.HasPrefix(update.IP6, "fe80") {
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
	}

	if success {
		if len(vm.Networks) > 0 {
			taps := []string{}
			for _, net := range vm.Networks {
				taps = append(taps, net.Tap)
			}

			err := ioutil.WriteFile(filepath.Join(vm.instancePath, "taps"), []byte(strings.Join(taps, "\n")), 0666)
			if err != nil {
				log.Error("write instance taps file: %v", err)
				vm.State = VM_ERROR
				cmd.Process.Kill()
				<-waitChan
				success = false
			}
		}
	}

	childSync1.Close()
	if success {
		// wait for the freezer notification
		var buf = make([]byte, 1)
		parentSync1.Read(buf)
		freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
		err = ioutil.WriteFile(freezer, []byte("FROZEN"), 0644)
		if err != nil {
			log.Error("freezer: %v", err)
			vm.State = VM_ERROR
			cmd.Process.Kill()
			<-waitChan
			success = false
		}
		parentSync2.Close()
	} else {
		parentSync1.Close()
		parentSync2.Close()
	}

	// connect cc
	ccPath := filepath.Join(vm.effectivePath, "cc")
	err = ccNode.ListenUnix(ccPath)
	if err != nil {
		log.Errorln(err)
	}

	ack <- vm.ID

	if success {
		go func() {
			select {
			case <-waitChan:
				log.Info("VM %v exited", vm.ID)
			case <-vm.kill:
				log.Info("Killing VM %v", vm.ID)
				cmd.Process.Kill()

				// containers cannot return unless thawed, so thaw the
				// process if necessary
				freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
				err = ioutil.WriteFile(freezer, []byte("THAWED"), 0644)
				if err != nil {
					log.Error("freezer: %v", err)
					vm.setState(VM_ERROR)
					<-waitChan
				}

				<-waitChan
				sendKillAck = true // wait to ack until we've cleaned up
			}

			err = ccNode.CloseUDS(ccPath)
			if err != nil {
				log.Errorln(err)
			}

			vm.listener.Close()
			vm.unlinkNetns()

			for _, net := range vm.Networks {
				b, err := getBridge(net.Bridge)
				if err != nil {
					log.Error("get bridge: %v", err)
				} else {
					b.ContainerTapDestroy(net.VLAN, net.Tap)
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
		}()
	}

	return nil
}

func (vm *ContainerVM) Flush() error {
	// umount the overlay, if any
	if vm.Snapshot {
		err := vm.overlayUnmount()
		if err != nil {
			log.Errorln(err)
		}
	}
	return vm.BaseVM.Flush()
}

func (vm *ContainerVM) symlinkNetns() error {
	err := os.MkdirAll("/var/run/netns", 0755)
	if err != nil {
		return err
	}
	src := fmt.Sprintf("/proc/%v/ns/net", vm.pid)
	dst := fmt.Sprintf("/var/run/netns/meganet_%v", vm.ID)
	vm.netns = fmt.Sprintf("meganet_%v", vm.ID)
	return os.Symlink(src, dst)
}

func (vm *ContainerVM) unlinkNetns() error {
	dst := fmt.Sprintf("/var/run/netns/%v", vm.netns)
	return os.Remove(dst)
}

// create an overlay mount (linux 3.18 or greater) is snapshot mode is
// being used.
func (vm *ContainerVM) overlayMount() error {
	vm.effectivePath = filepath.Join(vm.instancePath, "fs")
	workPath := filepath.Join(vm.instancePath, "fs_work")

	err := os.MkdirAll(vm.effectivePath, 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(workPath, 0755)
	if err != nil {
		return err
	}

	// create the overlay mountpoint
	args := []string{
		"mount",
		"-t",
		"overlay",
		fmt.Sprintf("megamount_%v", vm.ID),
		"-o",
		fmt.Sprintf("lowerdir=%v,upperdir=%v,workdir=%v", vm.FSPath, vm.effectivePath, workPath),
		vm.effectivePath,
	}
	log.Debug("mounting overlay: %v", args)
	out, err := processWrapper(args...)
	if err != nil {
		log.Error("overlay mount: %v %v", err, out)
		return err
	}
	return nil
}

func (vm *ContainerVM) overlayUnmount() error {
	err := syscall.Unmount(vm.effectivePath, 0)
	if err != nil {
		log.Error("overlay unmount: %v", err)
		return err
	}

	return nil
}

func (vm *ContainerVM) console(stdin, stdout, stderr *os.File) {
	socketPath := filepath.Join(vm.instancePath, "console")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Error("could not start unix domain socket console on vm %v: %v", vm.ID, err)
		return
	}
	vm.listener = l

	for {
		conn, err := l.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
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

func containerSetCapabilities() error {
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

func containerChroot(fsPath string) error {
	err := syscall.Mount(fsPath, "/", "", syscall.MS_MOVE, "")
	if err != nil {
		return err
	}
	err = syscall.Chroot(".")
	if err != nil {
		return err
	}
	return syscall.Chdir("/")
}

func containerPopulateCgroups(vmID, vmMemory int) error {
	cgroupPath := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vmID))
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
	err = ioutil.WriteFile(memory, []byte(fmt.Sprintf("%vM", vmMemory)), 0644)
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

func containerMaskPaths(fsPath string) error {
	for _, v := range containerMaskedPaths {
		p := filepath.Join(fsPath, v)
		err := syscall.Mount("/dev/null", p, "", syscall.MS_BIND, "")
		if err != nil {
			return err
		}
	}
	return nil
}

func containerRemountReadOnly(fsPath string) error {
	for _, v := range containerReadOnlyPaths {
		p := filepath.Join(fsPath, v)
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

func containerSymlinks(fsPath string) error {
	for _, l := range containerLinks {
		path := filepath.Join(fsPath, l[1])
		os.Remove(path)
		err := os.Symlink(l[0], path)
		if err != nil {
			return err
		}
	}
	return nil
}

func containerPtmx(fsPath string) error {
	path := filepath.Join(fsPath, "/dev/ptmx")
	os.Remove(path)
	err := os.Symlink("pts/ptmx", path)
	if err != nil {
		return err
	}

	// bind mount /dev/console
	path = filepath.Join(fsPath, "/dev/console")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	f.Close()
	return syscall.Mount("/dev/console", path, "", syscall.MS_BIND, "")
}

func containerMknodDevices(fsPath string) error {
	for _, v := range containerDeviceNames {
		path := filepath.Join(fsPath, v.Name)
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

func containerSetupRoot(fsPath string) error {
	err := syscall.Mount("", "/", "", syscall.MS_SLAVE|syscall.MS_REC, "")
	if err != nil {
		return err
	}
	return syscall.Mount(fsPath, fsPath, "bind", syscall.MS_BIND|syscall.MS_REC, "")
}

func containerMountDefaults(fsPath string) error {
	log.Debug("mountDefaults: %v", fsPath)

	var err error

	err = syscall.Mount("proc", filepath.Join(fsPath, "proc"), "proc", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("tmpfs", filepath.Join(fsPath, "dev"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_STRICTATIME, "mode=755")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(fsPath, "dev", "shm"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(fsPath, "dev", "mqueue"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = os.MkdirAll(filepath.Join(fsPath, "dev", "pts"), 0755)
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("tmpfs", filepath.Join(fsPath, "dev", "shm"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("pts", filepath.Join(fsPath, "dev", "pts"), "devpts", syscall.MS_NOEXEC|syscall.MS_NOSUID, "newinstance,ptmxmode=666,gid=5,mode=620")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = syscall.Mount("sysfs", filepath.Join(fsPath, "sys"), "sysfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_RDONLY, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

// aggressively cleanup container cruff, called by the nuke api
func containerNuke() {
	// walk /sys/fs/cgroup/minimega for tasks, killing each one
	err := filepath.Walk(CGROUP_PATH, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		log.Debug("walking file: %v", path)

		switch info.Name() {
		case "tasks":
			d, err := ioutil.ReadFile(path)
			pids := strings.Fields(string(d))
			for _, pid := range pids {
				log.Debug("found pid: %v", pid)

				log.Infoln("killing process:", pid)
				out, err := processWrapper("kill", "-9", pid)
				if err != nil {
					log.Error("%v: %v", err, out)
				}
			}
			// remove the directory for this vm
			dir := filepath.Dir(path)
			err = os.Remove(dir)
			if err != nil {
				log.Errorln(err)
			}
		}
		return nil
	})

	// remove cgroup structure
	err = os.Remove(CGROUP_PATH)
	if err != nil {
		log.Errorln(err)
	}

	// umount megamount_*
	d, err := ioutil.ReadFile("/proc/mounts")
	mounts := strings.Fields(string(d))
	for _, m := range mounts {
		if strings.Contains(m, "megamount") {
			err := syscall.Unmount(m, 0)
			if err != nil {
				log.Error("overlay unmount: %v", err)
			}
		}
	}

	// remove meganet_* from /var/run/netns
	netns, err := ioutil.ReadDir("/var/run/netns")
	if err != nil {
		log.Errorln(err)
	} else {
		for _, n := range netns {
			if strings.Contains(n.Name(), "meganet") {
				err := os.Remove(filepath.Join("/var/run/netns", n.Name()))
				if err != nil {
					log.Errorln(err)
				}
			}
		}
	}
}
