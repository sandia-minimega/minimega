// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

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
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
	"unsafe"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/kr/pty"
)

const (
	CONTAINER_MAGIC        = "CONTAINER"
	CONTAINER_NONE         = "CONTAINER_NONE"
	CONTAINER_KILL_TIMEOUT = 5 * time.Second

	// UnmountRetries is the maximum number of times to try to unmount a busy
	// filesystem.
	UnmountRetries = 10
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
	// Configure the filesystem to use for launching a container. This should
	// be a root filesystem for a linux distribution (containing /dev, /proc,
	// /sys, etc.)
	//
	// Note: this configuration only applies to containers and must be specified.
	FilesystemPath string

	// Set a hostname for a container before launching the init program. If not
	// set, the hostname will be the VM name. The hostname can also be set by
	// the init program or other root process in the container.
	//
	// Note: this configuration only applies to containers.
	Hostname string

	// Set the init program and args to exec into upon container launch. This
	// will be PID 1 in the container.
	//
	// Note: this configuration only applies to containers.
	//
	// Default: "/init"
	Init []string

	// Containers start in a highly restricted environment. vm config preinit
	// allows running processes before isolation mechanisms are enabled. This
	// occurs when the vm is launched and before the vm is put in the building
	// state. preinit processes must finish before the vm will be allowed to
	// start.
	//
	// Specifically, the preinit command will be run after entering namespaces,
	// and mounting dependent filesystems, but before cgroups and root
	// capabilities are set, and before entering the chroot. This means that
	// the preinit command is run as root and can control the host.
	//
	// For example, to run a script that enables ip forwarding, which is not
	// allowed during runtime because /proc is mounted read-only, add a preinit
	// script:
	//
	// 	vm config preinit enable_ip_forwarding.sh
	//
	// Note: this configuration only applies to containers.
	Preinit string

	// Set the number of named pipes to include in the container for
	// container-host communication. Named pipes will appear on the host in the
	// instance directory for the container as fifoN, and on the container as
	// /dev/fifos/fifoN.
	//
	// Fifos are created using mkfifo() and have all of the same usage
	// constraints.
	//
	// Note: this configuration only applies to containers.
	Fifos uint64

	// Attach one or more volumes to a container. These directories will be
	// mounted inside the container at the specified location.
	//
	// For example, to mount /scratch/data to /data inside the container:
	//
	//  vm config volume /data /scratch/data
	//
	// Commands with the same <key> will overwrite previous volumes:
	//
	//  vm config volume /data /scratch/data2
	//  vm config volume /data
	//  /scratch/data2
	//
	// Note: this configuration only applies to containers.
	//
	// Default: empty map
	VolumePaths map[string]string
}

type ContainerVM struct {
	*BaseVM         // embed
	ContainerConfig // embed

	effectivePath   string
	ptyUnixListener net.Listener
	ptyTCPListener  net.Listener
	netns           string

	ConsolePort int

	scrollBack         *byteFifo
	consoleMultiWriter *mutableMultiWriter
}

// Ensure that ContainerVM implements the VM interface
var _ VM = (*ContainerVM)(nil)

var (
	containerInitLock    sync.Mutex
	containerInitOnce    bool
	containerInitSuccess bool
)

func containerInit() error {
	containerInitLock.Lock()
	defer containerInitLock.Unlock()

	if containerInitOnce {
		return nil
	}
	containerInitOnce = true

	// create minimega freezer and memory cgroups
	log.Debug("cgroup init: %v", *f_cgroup)

	cgroupFreezer := filepath.Join(*f_cgroup, "freezer", "minimega")
	cgroupMemory := filepath.Join(*f_cgroup, "memory", "minimega")
	cgroupDevices := filepath.Join(*f_cgroup, "devices", "minimega")
	cgroupCPU := filepath.Join(*f_cgroup, "cpu", "minimega")
	cgroups := []string{cgroupFreezer, cgroupMemory, cgroupDevices, cgroupCPU}

	for _, cgroup := range cgroups {
		if err := os.MkdirAll(cgroup, 0755); err != nil {
			return fmt.Errorf("cgroup mkdir: %v", err)
		}

		// inherit cpusets
		if err := ioutil.WriteFile(filepath.Join(cgroup, "cgroup.clone_children"), []byte("1"), 0664); err != nil {
			return fmt.Errorf("setting cgroup: %v", err)
		}
	}

	if err := ioutil.WriteFile(filepath.Join(cgroupMemory, "memory.use_hierarchy"), []byte("1"), 0664); err != nil {
		return fmt.Errorf("setting use_hierarchy: %v", err)
	}

	// clean potentially old cgroup noise
	containerCleanCgroupDirs()

	containerInitSuccess = true
	return nil
}

func containerTeardown() {
	cgroupFreezer := filepath.Join(*f_cgroup, "freezer", "minimega")
	cgroupMemory := filepath.Join(*f_cgroup, "memory", "minimega")
	cgroupDevices := filepath.Join(*f_cgroup, "devices", "minimega")
	cgroupCPU := filepath.Join(*f_cgroup, "cpu", "minimega")
	cgroups := []string{cgroupFreezer, cgroupMemory, cgroupDevices, cgroupCPU}

	for _, cgroup := range cgroups {
		if err := os.Remove(cgroup); err != nil {
			if containerInitSuccess {
				log.Errorln(err)
			}
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
//	3: logging port, closed just before exec into init
//	4: closed by the child before exec to elect to be frozen
//	5: closed by the parent when the child returns to allow calling exec
//	6: stdin
//	7: stdout
//	8: stderr
//
// A number of arguments are passed on flag.Args to configure the container:
//
//	0 :  CONTAINER
//	1 :  instance path
//	2 :  vm id
//	3 :  hostname ("CONTAINER_NONE" if none)
//	4 :  filesystem path
//	5 :  vcpus
//	6 :  memory in megabytes
//	7 :  uuid
//	8 :  number of fifos
//	9 :  preinit program
//	10+: source:target volumes, `--` signifies end
//	11+ :  init program and args (relative to filesystem path)
func containerShim() {
	args := flag.Args()
	if flag.NArg() < 11 { // 11 because init args can be nil
		os.Exit(1)
	}

	// we log to fd(3), and close it before we move on to exec ourselves
	logFile := os.NewFile(uintptr(3), "")
	log.AddLogger("file", logFile, log.DEBUG, false)

	log.Debug("containerShim: %v", args)

	// get args
	vmInstancePath := args[1]
	vmID, err := strconv.Atoi(args[2])
	if err != nil {
		log.Fatalln(err)
	}
	vmHostname := args[3]
	if vmHostname == CONTAINER_NONE {
		vmHostname = ""
	}
	vmFSPath := args[4]
	vmVCPUs, err := strconv.Atoi(args[5])
	if err != nil {
		log.Fatalln(err)
	}
	vmMemory, err := strconv.Atoi(args[6])
	if err != nil {
		log.Fatalln(err)
	}
	vmUUID := args[7]
	vmFifos, err := strconv.Atoi(args[8])
	if err != nil {
		log.Fatalln(err)
	}
	vmPreinit := args[9]

	// find `--` separator between vmVolumes and vmInit
	var vmVolumes, vmInit []string
	for i, v := range args[10:] {
		if v == "--" {
			vmInit = args[10+i+1:]
			break
		}
		vmVolumes = append(vmVolumes, v)
	}

	log.Debug("vmVolumes: %v", vmVolumes)
	log.Debug("vmInit: %v", vmInit)

	// set hostname
	log.Debug("vm %v hostname", vmID)
	if vmHostname != "" {
		if err := syscall.Sethostname([]byte(vmHostname)); err != nil {
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

	// mount volumes
	log.Debug("vm %v containerMountVolumes", vmID)
	err = containerMountVolumes(vmFSPath, vmVolumes)
	if err != nil {
		log.Fatal("containerMountVolumes: %v", err)
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

	// preinit
	if vmPreinit != CONTAINER_NONE {
		log.Debug("preinit: %v", vmPreinit)
		out, err := exec.Command(vmPreinit, vmPreinit).CombinedOutput()
		if err != nil {
			log.Fatal("containerPreinit: %v: %v", err, string(out))
		}
		if len(out) != 0 {
			log.Debug("containerPreinit: %v", string(out))
		}
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

	// mask uuid path if possible - not all platforms have dmi
	log.Debug("uuid bind mount: %v -> %v", vmUUID, containerUUIDLink)
	err = syscall.Mount(vmUUID, filepath.Join(vmFSPath, containerUUIDLink), "", syscall.MS_BIND, "")
	if err != nil {
		log.Warn("containerUUIDLink: %v", err)
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
	err = containerPopulateCgroups(vmID, vmVCPUs, vmMemory)
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
	log.Debug("vm %v exec: %v", vmID, vmInit)
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
func (old ContainerConfig) Copy() ContainerConfig {
	// Copy all fields
	res := old

	// Make deep copy of volumes
	res.VolumePaths = map[string]string{}
	for k, v := range old.VolumePaths {
		res.VolumePaths[k] = v
	}

	return res
}

func (vm *ContainerVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func NewContainer(name, namespace string, config VMConfig) (*ContainerVM, error) {
	vm := new(ContainerVM)

	vm.BaseVM = NewBaseVM(name, namespace, config)
	vm.Type = CONTAINER

	vm.ContainerConfig = config.ContainerConfig.Copy() // deep-copy configured fields

	// set hostname to VM's name if it's unspecified. note that the name arg
	// may be the empty string but NewBaseVM populates vm.Name with a default
	// value if that's the case.
	if vm.Hostname == "" {
		vm.Hostname = vm.Name
	}

	if vm.FilesystemPath == "" {
		return nil, errors.New("unable to create container without a configured filesystem")
	}

	return vm, nil
}

func (vm *ContainerVM) Copy() VM {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm2 := new(ContainerVM)

	// Make shallow copies of all fields
	*vm2 = *vm

	// Make deep copies
	vm2.BaseVM = vm.BaseVM.copy()
	vm2.ContainerConfig = vm.ContainerConfig.Copy()

	return vm2
}

func (vm *ContainerVM) Launch() error {
	defer vm.lock.Unlock()

	return vm.launch()
}

func (vm *ContainerVM) Recover(string, int) error {
	vm.lock.Unlock()
	return nil
}

func (vm *ContainerVM) Start() (err error) {
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

		// Launch handles setting the VM to error state
		if err := vm.launch(); err != nil {
			return err
		}
	}

	log.Info("starting VM: %v", vm.ID)
	if err := vm.thaw(); err != nil {
		return vm.setErrorf("unstartable: %v", err)
	}

	vm.setState(VM_RUNNING)

	return nil
}

func (vm *ContainerVM) Stop() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.Name == "vince" {
		log.Warn("vince is unstoppable")
	}

	if vm.State != VM_RUNNING {
		return vmNotRunning(strconv.Itoa(vm.ID))
	}

	log.Info("stopping VM: %v", vm.ID)
	if err := vm.freeze(); err != nil {
		return vm.setErrorf("unstoppable: %v", err)
	}

	vm.setState(VM_PAUSED)

	return nil
}

func (vm *ContainerVM) String() string {
	return fmt.Sprintf("%s:%d:container", hostname, vm.ID)
}

func (vm *ContainerVM) Info(field string) (string, error) {
	// If the field is handled by BaseVM, return it
	if v, err := vm.BaseVM.Info(field); err == nil {
		return v, nil
	}

	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch field {
	case "console_port":
		return strconv.Itoa(vm.ConsolePort), nil
	case "volume":
		return marshal(vm.VolumePaths), nil
	case "pid":
		return strconv.Itoa(vm.Pid), nil
	}

	return vm.ContainerConfig.Info(field)
}

func (vm *ContainerVM) Conflicts(vm2 VM) error {
	switch vm2 := vm2.(type) {
	case *ContainerVM:
		return vm.ConflictsContainer(vm2)
	case *KvmVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	}

	return errors.New("unknown VM type")
}

// ConflictsContainer tests whether vm and vm2 share a filesystem and
// returns an error if one of them is not running in snapshot mode. Also
// checks whether the BaseVMs conflict.
func (vm *ContainerVM) ConflictsContainer(vm2 *ContainerVM) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.FilesystemPath == vm2.FilesystemPath && (!vm.Snapshot || !vm2.Snapshot) {
		return fmt.Errorf("filesystem conflict with vm %v: %v", vm.Name, vm.FilesystemPath)
	}

	return vm.BaseVM.conflicts(vm2.BaseVM)
}

func (vm *ContainerVM) Screenshot(size int) ([]byte, error) {
	return nil, errors.New("cannot take screenshot of container")
}

func (vm *ContainerConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Container configuration:")
	fmt.Fprintf(w, "Filesystem Path:\t%v\n", vm.FilesystemPath)
	fmt.Fprintf(w, "Hostname:\t%v\n", vm.Hostname)
	fmt.Fprintf(w, "Init:\t%v\n", vm.Init)
	fmt.Fprintf(w, "Pre-init:\t%v\n", vm.Preinit)
	fmt.Fprintf(w, "FIFOs:\t%v\n", vm.Fifos)
	fmt.Fprintf(w, "Volumes:\t\n")
	for k, v := range vm.VolumePaths {
		fmt.Fprintf(w, "\t%v -> %v\n", k, v)
	}
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

// launch is the low-level launch function for Container VMs. The caller should
// hold the VM's lock.
func (vm *ContainerVM) launch() error {
	log.Info("launching vm: %v", vm.ID)

	err := containerInit()
	if err != nil || !containerInitSuccess {
		return vm.setErrorf("cgroups are not initialized, cannot continue: %v", err)
	}

	// If this is the first time launching the VM, do the final configuration
	// check, create a directory for it, and setup the FS.
	if vm.State == VM_BUILDING {
		if err := os.MkdirAll(vm.instancePath, os.FileMode(0700)); err != nil {
			return vm.setErrorf("unable to create VM dir: %v", err)
		}

		if vm.Snapshot {
			if err := vm.overlayMount(); err != nil {
				return vm.setErrorf("overlayMount: %v", err)
			}
		} else {
			vm.effectivePath = vm.FilesystemPath
		}

		if err := vm.createInstancePathAlias(); err != nil {
			return vm.setErrorf("createInstancePathAlias: %v", err)
		}
	}

	mustWrite(vm.path("name"), vm.Name)

	// the child process will communicate with a fake console using pipes
	// to mimic stdio, and a fourth pipe for logging before the child execs
	// into the init program
	// two additional pipes are needed to synchronize freezing the child
	// before it enters the container
	parentLog, childLog, err := os.Pipe()
	if err != nil {
		return vm.setErrorf("pipe: %v", err)
	}
	parentSync1, childSync1, err := os.Pipe()
	if err != nil {
		return vm.setErrorf("pipe: %v", err)
	}
	childSync2, parentSync2, err := os.Pipe()
	if err != nil {
		return vm.setErrorf("pipe: %v", err)
	}

	// create the uuid path that will bind mount into sysfs in the
	// container
	uuidPath := vm.path("uuid")
	ioutil.WriteFile(uuidPath, []byte(vm.UUID+"\n"), 0400)

	// create fifos
	for i := uint64(0); i < vm.Fifos; i++ {
		p := vm.path(fmt.Sprintf("fifo%v", i))
		if err = syscall.Mkfifo(p, 0660); err != nil {
			return vm.setErrorf("fifo: %v", err)
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
	preinit := vm.Preinit
	if preinit == "" {
		preinit = CONTAINER_NONE
	}
	args := []string{
		os.Args[0],
		"-base",
		*f_base,
		CONTAINER_MAGIC,
		vm.instancePath,
		fmt.Sprintf("%v", vm.ID),
		hn,
		vm.effectivePath,
		strconv.FormatUint(vm.VCPUs, 10),
		strconv.FormatUint(vm.Memory, 10),
		uuidPath,
		fmt.Sprintf("%v", vm.Fifos),
		preinit,
	}
	for k, v := range vm.VolumePaths {
		// Create source:target pairs
		// TODO: should probably handle spaces
		args = append(args, fmt.Sprintf("%v:%v", v, k))

		// create source directory if it doesn't exist
		if err := os.MkdirAll(v, 0755); err != nil {
			return vm.setErrorf("start container: %v", err)
		}
	}
	// denotes end of volumes
	args = append(args, "--")
	args = append(args, vm.Init...)

	// launch the container
	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: args,
		ExtraFiles: []*os.File{
			childLog,
			childSync1,
			childSync2,
		},
		SysProcAttr: &syscall.SysProcAttr{
			Cloneflags: uintptr(CONTAINER_FLAGS),
		},
	}

	// Start the child and give it a pty
	pseudotty, err := pty.Start(cmd)
	if err != nil {
		vm.overlayUnmount()
		return vm.setErrorf("start container: %v", err)
	}

	vm.Pid = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.ID, vm.Pid)

	// log the child
	childLog.Close()
	log.LogAll(parentLog, log.DEBUG, "containerShim")

	go vm.console(pseudotty)

	// network creation for containers happens /after/ the container is
	// started, as we need the PID in order to attach a veth to the container
	// side of the network namespace. That means that unlike kvm vms, we MUST
	// create/destroy taps on launch/kill boundaries (kvm destroys taps on
	// flush).
	if err = vm.launchNetwork(); err != nil {
		log.Errorln(err)
	}

	childSync1.Close()
	if err == nil {
		// wait for the freezer notification
		var buf = make([]byte, 1)
		parentSync1.Read(buf)

		err = vm.freeze()

		parentSync2.Close()
	} else {
		parentSync1.Close()
		parentSync2.Close()
	}

	if err != nil {
		// Some error occurred.. clean up the process
		cmd.Process.Kill()

		vm.unlinkNetns()

		return vm.setErrorf("%v", err)
	}

	// Channel to signal when the process has exited
	errChan := make(chan error)

	// Create goroutine to wait for process to exit
	go func() {
		defer close(errChan)

		errChan <- cmd.Wait()
	}()

	go func() {
		defer vm.cond.Signal()

		cgroupFreezer := vm.cgroup("freezer")
		cgroupMemory := vm.cgroup("memory")
		cgroupDevices := vm.cgroup("devices")
		cgroupCPU := vm.cgroup("cpu")
		cgroups := []string{cgroupFreezer, cgroupMemory, cgroupDevices, cgroupCPU}

		select {
		case err := <-errChan:
			log.Info("VM %v exited", vm.ID)

			vm.lock.Lock()
			defer vm.lock.Unlock()

			// we don't need to check the error for a clean kill,
			// as there's no way to get here if we killed it.
			if err != nil {
				vm.setErrorf("killed container: %v", err)
			}
		case <-vm.kill:
			log.Info("Killing VM %v", vm.ID)

			vm.lock.Lock()
			defer vm.lock.Unlock()

			cmd.Process.Kill()

			// containers cannot exit unless thawed, so thaw it if necessary
			if err := vm.thaw(); err != nil {
				vm.setErrorf("unable to thaw container: %v", err)
			}

			// wait for the taskset to actually exit (from uninterruptible
			// sleep state).
			for {
				t, err := ioutil.ReadFile(filepath.Join(cgroupFreezer, "tasks"))
				if err != nil {
					vm.setErrorf("unable to read tasks: %v", err)
					break
				}
				if len(t) == 0 {
					break
				}

				count := strings.Count(string(t), "\n")
				log.Info("waiting on %d tasks for VM %v", count, vm.ID)
				time.Sleep(100 * time.Millisecond)
			}

			// drain errChan
			for err := range errChan {
				log.Debug("kill container: %v", err)
			}
		}

		if vm.ptyUnixListener != nil {
			vm.ptyUnixListener.Close()
		}
		if vm.ptyTCPListener != nil {
			vm.ptyTCPListener.Close()
		}

		vm.unlinkNetns()

		for _, net := range vm.Networks {
			br, err := getBridge(net.Bridge)
			if err != nil {
				log.Error("get bridge: %v", err)
			} else {
				br.DestroyTap(net.Tap)
			}
		}

		// clean up the cgroup directory
		for _, cgroup := range cgroups {
			if err := os.Remove(cgroup); err != nil {
				log.Errorln(err)
			}
		}

		if vm.State != VM_ERROR {
			// Set to QUIT unless we've already been put into the error state
			vm.setState(VM_QUIT)
		}
	}()

	return nil
}

func (vm *ContainerVM) Connect(cc *ron.Server, reconnect bool) error {
	if !vm.Backchannel || reconnect {
		return nil
	}

	cc.RegisterVM(vm)

	ccPath := filepath.Join(vm.effectivePath, "cc")

	return cc.ListenUnix(ccPath)
}

func (vm *ContainerVM) Disconnect(cc *ron.Server) error {
	if !vm.Backchannel {
		return nil
	}

	cc.UnregisterVM(vm)

	vm.lock.Lock()
	defer vm.lock.Unlock()

	ccPath := filepath.Join(vm.effectivePath, "cc")

	return cc.CloseUnix(ccPath)
}

func (vm *ContainerVM) launchNetwork() error {
	// create and add taps if we are associated with any networks
	// expose the network namespace to iptool
	if err := vm.symlinkNetns(); err != nil {
		return fmt.Errorf("symlinkNetns: %v", err)
	}

	for i := range vm.Networks {
		nic := &vm.Networks[i]

		// squash driver, not used by containers
		nic.Driver = ""

		br, err := getBridge(nic.Bridge)
		if err != nil {
			return fmt.Errorf("get bridge: %v", err)
		}

		nic.Tap, err = br.CreateContainerTap(nic.Tap, vm.netns, nic.MAC, nic.VLAN, i)
		if err != nil {
			return fmt.Errorf("create tap: %v", err)
		}

		if nic.QinQ {
			if err := br.SetTapQinQ(nic.Tap, nic.VLAN); err != nil {
				return err
			}
		}
	}

	if len(vm.Networks) > 0 {
		if err := vm.writeTaps(); err != nil {
			return fmt.Errorf("write taps: %v", err)
		}
	}

	return nil
}

func (vm *ContainerVM) Flush() error {
	// umount the overlay, if any
	if vm.Snapshot {
		if err := vm.overlayUnmount(); err != nil {
			return err
		}
	}

	return vm.BaseVM.Flush()
}

// NetworkConnect calls BaseVM.networkConnect when the container is running or
// paused. Otherwise, it makes changes to the nic and returns.
func (vm *ContainerVM) NetworkConnect(pos, vlan int, bridge string) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch vm.State {
	case VM_RUNNING, VM_PAUSED:
		// containers only have taps while they are running
		return vm.BaseVM.networkConnect(pos, vlan, bridge)
	}

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, container only has %v networks", pos, len(vm.Networks))
	}

	nic := &vm.Networks[pos]

	nic.VLAN = vlan

	if bridge != "" {
		nic.Bridge = bridge
	}
	if nic.Bridge == "" {
		nic.Bridge = DefaultBridge
	}

	return nil
}

// NetworkDisconnect mirrors the behavior of NetworkConnect.
func (vm *ContainerVM) NetworkDisconnect(pos int) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch vm.State {
	case VM_RUNNING, VM_PAUSED:
		// containers only have taps while they are running
		return vm.BaseVM.networkDisconnect(pos)
	}

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, container only has %v networks", pos, len(vm.Networks))
	}

	nic := &vm.Networks[pos]

	nic.Bridge = ""
	nic.VLAN = DisconnectedVLAN

	return nil
}

func (vm *ContainerVM) symlinkNetns() error {
	err := os.MkdirAll("/var/run/netns", 0755)
	if err != nil {
		return err
	}
	src := fmt.Sprintf("/proc/%v/ns/net", vm.Pid)
	dst := fmt.Sprintf("/var/run/netns/meganet_%v", vm.ID)
	vm.netns = fmt.Sprintf("meganet_%v", vm.ID)
	return os.Symlink(src, dst)
}

func (vm *ContainerVM) unlinkNetns() error {
	dst := fmt.Sprintf("/var/run/netns/%v", vm.netns)
	return os.Remove(dst)
}

// create an overlay mount (linux 3.18 or greater) if snapshot mode is
// being used.
func (vm *ContainerVM) overlayMount() error {
	if _, err := os.Stat(vm.FilesystemPath); os.IsNotExist(err) {
		return err
	}

	vm.effectivePath = vm.path("fs")
	workPath := vm.path("fs_work")

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
		fmt.Sprintf("lowerdir=%v,upperdir=%v,workdir=%v", vm.FilesystemPath, vm.effectivePath, workPath),
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
	for i := 0; i < UnmountRetries; i++ {
		err := syscall.Unmount(vm.effectivePath, 0)
		if err == nil {
			return nil
		}

		if err, ok := err.(syscall.Errno); ok && err == syscall.EBUSY {
			log.Info("filesystem busy for vm %v, sleeping", vm.ID)
			time.Sleep(time.Second)
			continue
		}

		return fmt.Errorf("overlay unmount: %v", err)
	}

	return fmt.Errorf("unable to unmount overlay")
}

func (vm *ContainerVM) console(pseudotty *os.File) {
	// initialize scrollback
	vm.scrollBack = NewByteFifo(1920) // 80x24 is 1920 characters, but we want a little history for e.g. vim
	// Create the multiwriter and add the scrollback to it to start
	vm.consoleMultiWriter = NewMutableMultiWriter(vm.scrollBack)
	go io.Copy(vm.consoleMultiWriter, pseudotty)

	serve := func(l net.Listener) {
		for {
			conn, err := l.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				log.Error("console %v for %v: %v", l.Addr(), vm.ID, err)
				continue
			}

			log.Info("new connection: %v -> %v for %v", conn.RemoteAddr(), l.Addr(), vm.ID)

			// copy from the connection to the pty (user input)
			go func() {
				io.Copy(pseudotty, conn)
				log.Info("disconnection: %v -> %v for %v", conn.RemoteAddr(), l.Addr(), vm.ID)
				vm.consoleMultiWriter.DelWriter(conn)
			}()

			// register conn into the mutable-multiwriter
			vm.consoleMultiWriter.AddWriter(conn)

			// Copy scrollback to conn
			conn.Write(vm.scrollBack.Get())
		}
	}

	l, err := net.Listen("unix", vm.path("console"))
	if err != nil {
		log.Error("could not start unix domain socket console on vm %v: %v", vm.ID, err)
		return
	}
	vm.ptyUnixListener = l

	go serve(l)

	l, err = net.Listen("tcp", ":0")
	if err != nil {
		log.Error("failed to open tcp socket for container console")
		return
	}
	vm.ptyTCPListener = l

	log.Info("container console listening on %v", l.Addr().String())
	vm.ConsolePort = l.Addr().(*net.TCPAddr).Port

	go serve(l)
}

func (vm *ContainerVM) freeze() error {
	freezer := filepath.Join(vm.cgroup("freezer"), "freezer.state")
	if err := ioutil.WriteFile(freezer, []byte("FROZEN"), 0644); err != nil {
		return fmt.Errorf("freezer: %v", err)
	}

	return nil
}

func (vm *ContainerVM) thaw() error {
	freezer := filepath.Join(vm.cgroup("freezer"), "freezer.state")
	if err := ioutil.WriteFile(freezer, []byte("THAWED"), 0644); err != nil {
		return fmt.Errorf("freezer: %v", err)
	}

	return nil
}

func (vm *ContainerVM) cgroup(s string) string {
	return filepath.Join(*f_cgroup, s, "minimega", strconv.Itoa(vm.ID))
}

func (vm *ContainerVM) ProcStats() (map[int]*ProcStats, error) {
	freezer := filepath.Join(vm.cgroup("freezer"), "cgroup.procs")
	b, err := ioutil.ReadFile(freezer)
	if err != nil {
		return nil, err
	}

	res := map[int]*ProcStats{}

	for i, v := range strings.Fields(string(b)) {
		if i >= ProcLimit {
			break
		}

		// should always be an int...
		if i, err := strconv.Atoi(v); err == nil {
			// supress errors... processes may have exited between reading
			// tasks and trying to read stats
			if p, err := GetProcStats(i); err == nil {
				res[i] = p
			}
		}
	}

	return res, nil
}

func (vm *ContainerVM) WriteConfig(w io.Writer) error {
	if err := vm.BaseConfig.WriteConfig(w); err != nil {
		return err
	}

	return vm.ContainerConfig.WriteConfig(w)
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

func containerPopulateCgroups(vmID, vcpus, memory int) error {
	cgroupFreezer := filepath.Join(*f_cgroup, "freezer", "minimega", strconv.Itoa(vmID))
	cgroupMemory := filepath.Join(*f_cgroup, "memory", "minimega", strconv.Itoa(vmID))
	cgroupDevices := filepath.Join(*f_cgroup, "devices", "minimega", strconv.Itoa(vmID))
	cgroupCPU := filepath.Join(*f_cgroup, "cpu", "minimega", strconv.Itoa(vmID))
	cgroups := []string{cgroupFreezer, cgroupMemory, cgroupDevices, cgroupCPU}

	for _, cgroup := range cgroups {
		if err := os.MkdirAll(cgroup, 0755); err != nil {
			return err
		}
	}

	// devices
	deny := filepath.Join(cgroupDevices, "devices.deny")
	allow := filepath.Join(cgroupDevices, "devices.allow")
	if err := ioutil.WriteFile(deny, []byte("a"), 0200); err != nil {
		return err
	}
	for _, a := range containerDevices {
		if err := ioutil.WriteFile(allow, []byte(a), 0200); err != nil {
			return err
		}
	}

	// Set CPU bandwidth control for the cgroup to emulate the desired number
	// of CPUs. This limits the tasks to a total run-time (quota) over a given
	// period. To emulate a given number of VCPUs, we compute the quota by
	// simply multipling the period by the number of VCPUs.  Both are then
	// converted to microseconds. Our default period is one second which allows
	// a high burst capacity. Based on:
	//
	// https://www.kernel.org/doc/Documentation/scheduler/sched-bwc.txt
	period := time.Second.Nanoseconds() / 1000
	quota := int64(vcpus) * time.Second.Nanoseconds() / 1000
	cfsPeriod := filepath.Join(cgroupCPU, "cpu.cfs_period_us")
	if err := ioutil.WriteFile(cfsPeriod, []byte(strconv.FormatInt(period, 10)), 0644); err != nil {
		return err
	}
	cfsQuota := filepath.Join(cgroupCPU, "cpu.cfs_quota_us")
	if err := ioutil.WriteFile(cfsQuota, []byte(strconv.FormatInt(quota, 10)), 0644); err != nil {
		return err
	}

	// memory
	memLimit := filepath.Join(cgroupMemory, "memory.limit_in_bytes")
	if err := ioutil.WriteFile(memLimit, []byte(fmt.Sprintf("%vM", memory)), 0644); err != nil {
		return err
	}

	// associate the pid with these permissions
	for _, cgroup := range cgroups {
		tasks := filepath.Join(cgroup, "cgroup.procs")
		if err := ioutil.WriteFile(tasks, []byte(fmt.Sprintf("%v", os.Getpid())), 0644); err != nil {
			return err
		}
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

// mkdirMount makes the target directory before trying to mount to it. Has
// the same arguments as syscall.Mount.
func mkdirMount(source, target, fstype string, flags uintptr, data string) error {
	log.Debug("mkdirMount: %v", target)

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	return syscall.Mount(source, target, fstype, flags, data)
}

func containerMountDefaults(fsPath string) error {
	log.Debug("mountDefaults: %v", fsPath)

	var err error

	err = mkdirMount("proc", filepath.Join(fsPath, "proc"), "proc", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = mkdirMount("tmpfs", filepath.Join(fsPath, "dev"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_STRICTATIME, "mode=755")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = mkdirMount("tmpfs", filepath.Join(fsPath, "dev", "shm"), "tmpfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = mkdirMount("pts", filepath.Join(fsPath, "dev", "pts"), "devpts", syscall.MS_NOEXEC|syscall.MS_NOSUID, "newinstance,ptmxmode=666,gid=5,mode=620")
	if err != nil {
		log.Errorln(err)
		return err
	}

	err = mkdirMount("sysfs", filepath.Join(fsPath, "sys"), "sysfs", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_RDONLY, "")
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

func containerMountVolumes(fsPath string, volumes []string) error {
	for _, v := range volumes {
		f := strings.Split(v, ":")
		if len(f) != 2 {
			return fmt.Errorf("invalid volume, expected `source:target`: %v", v)
		}

		source := f[0]
		target := filepath.Join(fsPath, f[1])

		if err := mkdirMount(source, target, "", syscall.MS_BIND, ""); err != nil {
			return err
		}
	}

	return nil
}

// aggressively cleanup container cruff, called by the nuke api
func containerNuke() {
	// walk minimega cgroups for tasks, killing each one
	cgroupFreezer := filepath.Join(*f_cgroup, "freezer", "minimega")
	cgroupMemory := filepath.Join(*f_cgroup, "memory", "minimega")
	cgroupDevices := filepath.Join(*f_cgroup, "devices", "minimega")
	cgroupCPU := filepath.Join(*f_cgroup, "cpu", "minimega")

	cgroups := []string{cgroupFreezer, cgroupMemory, cgroupDevices, cgroupCPU}

	for _, cgroup := range cgroups {
		if _, err := os.Stat(cgroup); err == nil {
			err := filepath.Walk(cgroup, containerNukeWalker)
			if err != nil {
				log.Errorln(err)
			}
		}
	}

	// Allow udev to sync
	time.Sleep(time.Second * 1)

	// umount megamount_*, this include overlayfs mounts
	d, err := ioutil.ReadFile("/proc/mounts")
	if err != nil {
		log.Errorln(err)
	} else {
		mounts := strings.Split(string(d), "\n")
		for _, m := range mounts {
			if strings.Contains(m, "megamount") {
				mount := strings.Split(m, " ")[1]
				if err := syscall.Unmount(mount, 0); err != nil {
					log.Error("overlay unmount %s: %v", m, err)
				}
			}
		}
	}

	containerCleanCgroupDirs()

	// remove meganet_* from /var/run/netns
	if _, err := os.Stat("/var/run/netns"); err == nil {
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
}

func containerNukeWalker(path string, info os.FileInfo, err error) error {
	if err != nil {
		return nil
	}

	log.Debug("walking file: %v", path)

	switch info.Name() {
	case "tasks":
		d, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}

		for _, pid := range strings.Fields(string(d)) {
			log.Debug("found pid: %v", pid)

			// attempt to unfreeze the cgroup first, ignoring any errors
			// the vm id is the second to last field in the path
			pathFields := strings.Split(path, string(os.PathSeparator))
			vmID := pathFields[len(pathFields)-2]

			freezer := filepath.Join(*f_cgroup, "freezer", "minimega", vmID, "freezer.state")
			if err := ioutil.WriteFile(freezer, []byte("THAWED"), 0644); err != nil {
				log.Debugln(err)
			}

			if i, err := strconv.Atoi(pid); err == nil {
				log.Info("killing process: %v", i)
				if err := syscall.Kill(i, syscall.SIGKILL); err == nil {
					continue
				} else if !strings.Contains(err.Error(), "no such process") {
					log.Error("unable to kill %v: %v", i, err)
				}
			}
		}
	}

	return nil
}

// remove state across cgroup mounts
func containerCleanCgroupDirs() {
	paths := []string{
		filepath.Join(*f_cgroup, "freezer", "minimega"),
		filepath.Join(*f_cgroup, "memory", "minimega"),
		filepath.Join(*f_cgroup, "devices", "minimega"),
		filepath.Join(*f_cgroup, "cpu", "minimega"),
	}
	for _, d := range paths {
		_, err := os.Stat(d)
		if err != nil {
			continue
		}

		err = filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if path == d {
				return nil
			}

			log.Debug("walking file: %v", path)

			if info.IsDir() {
				err = os.Remove(path)
				if err != nil {
					log.Errorln(err)
					return err
				}
			}

			return nil
		})
		if err != nil {
			continue
		}

		err = os.Remove(d)
		if err != nil {
			log.Errorln(err)
		}
	}
}
