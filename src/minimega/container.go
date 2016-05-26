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
	"time"
	"unsafe"
)

const (
	CONTAINER_MAGIC        = "CONTAINER"
	CONTAINER_NONE         = "CONTAINER_NONE"
	CONTAINER_KILL_TIMEOUT = 5 * time.Second
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
	Preinit  string
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

	// clean potentially old cgroup noise
	err = containerCleanCgroupDirs()
	if err != nil {
		return err
	}

	// create a minimega cgroup
	err = os.MkdirAll(CGROUP_PATH, 0755)
	if err != nil {
		return fmt.Errorf("creating minimega cgroup: %v", err)
	}

	containerInitSuccess = true
	return nil
}

func containerTeardown() {
	err := os.Remove(CGROUP_PATH)
	if err != nil {
		if containerInitSuccess {
			log.Errorln(err)
		}
	}
	err = syscall.Unmount(CGROUP_ROOT, 0)
	if err != nil {
		if containerInitSuccess {
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
//	9 :  preinit program
//	10:  init program (relative to filesystem path)
//	11:  init args
func containerShim() {
	if len(os.Args) < 11 { // 11 because init args can be nil
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
	vmPreinit := os.Args[9]
	vmInit := os.Args[10:]

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
func (old ContainerConfig) Copy() ContainerConfig {
	// Copy all fields
	res := old

	// Make deep copy of slices
	// none yet - placeholder

	return res
}

func (vm *ContainerVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func NewContainer(name string) *ContainerVM {
	vm := new(ContainerVM)

	vm.BaseVM = *NewBaseVM(name)
	vm.Type = CONTAINER

	vm.ContainerConfig = vmConfig.ContainerConfig.Copy() // deep-copy configured fields

	return vm
}

func (vm *ContainerVM) Launch() error {
	defer vm.lock.Unlock()

	return vm.launch()
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
		log.Errorln(err)
		vm.setError(err)
		return err
	}

	vm.setState(VM_RUNNING)

	return nil
}

func (vm *ContainerVM) Stop() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State != VM_RUNNING {
		return vmNotRunning(strconv.Itoa(vm.ID))
	}

	log.Info("stopping VM: %v", vm.ID)
	if err := vm.freeze(); err != nil {
		log.Errorln(err)
		vm.setError(err)
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

func (vm *ContainerVM) SaveConfig(w io.Writer) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	cmds := []string{"clear vm config"}
	cmds = append(cmds, saveConfig(baseConfigFns, &vm.BaseConfig)...)
	cmds = append(cmds, saveConfig(containerConfigFns, &vm.ContainerConfig)...)
	cmds = append(cmds, fmt.Sprintf("vm launch %s %s", vm.Type, vm.Name))

	_, err := io.WriteString(w, strings.Join(cmds, "\n"))
	return err
}

func (vm *ContainerConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current container configuration:")
	fmt.Fprintf(w, "Filesystem Path:\t%v\n", vm.FSPath)
	fmt.Fprintf(w, "Hostname:\t%v\n", vm.Hostname)
	fmt.Fprintf(w, "Init:\t%v\n", vm.Init)
	fmt.Fprintf(w, "Pre-init:\t%v\n", vm.Preinit)
	fmt.Fprintf(w, "FIFOs:\t%v\n", vm.Fifos)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

// launch is the low-level launch function for Container VMs. The caller should
// hold the VM's lock.
func (vm *ContainerVM) launch() error {
	log.Info("launching vm: %v", vm.ID)

	err := containerInit()
	if err != nil {
		log.Errorln(err)
		vm.setError(err)
		return err
	}
	if !containerInitSuccess {
		err = fmt.Errorf("cgroups are not initialized, cannot continue")
		log.Errorln(err)
		vm.setError(err)
		return err
	}

	// If this is the first time launching the VM, do the final configuration
	// check, create a directory for it, and setup the FS.
	if vm.State == VM_BUILDING {
		ccNode.RegisterVM(vm.UUID, vm)

		if err := os.MkdirAll(vm.instancePath, os.FileMode(0700)); err != nil {
			teardownf("unable to create VM dir: %v", err)
		}

		if vm.Snapshot {
			if err := vm.overlayMount(); err != nil {
				log.Error("overlayMount: %v", err)
				vm.setError(err)
				return err
			}
		} else {
			vm.effectivePath = vm.FSPath
		}
	}

	// write the config for this vm
	config := vm.BaseConfig.String() + vm.ContainerConfig.String()
	writeOrDie(filepath.Join(vm.instancePath, "config"), config)
	writeOrDie(filepath.Join(vm.instancePath, "name"), vm.Name)

	// the child process will communicate with a fake console using pipes
	// to mimic stdio, and a fourth pipe for logging before the child execs
	// into the init program
	// two additional pipes are needed to synchronize freezing the child
	// before it enters the container
	parentLog, childLog, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}
	childStdin, parentStdin, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}
	parentStdout, childStdout, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}
	parentStderr, childStderr, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}
	parentSync1, childSync1, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}
	childSync2, parentSync2, err := os.Pipe()
	if err != nil {
		log.Error("pipe: %v", err)
		vm.setError(err)
		return err
	}

	// create the uuid path that will bind mount into sysfs in the
	// container
	uuidPath := filepath.Join(vm.instancePath, "uuid")
	ioutil.WriteFile(uuidPath, []byte(vm.UUID+"\n"), 0400)

	// create fifos
	for i := 0; i < vm.Fifos; i++ {
		p := filepath.Join(vm.instancePath, fmt.Sprintf("fifo%v", i))
		if err = syscall.Mkfifo(p, 0660); err != nil {
			log.Error("fifo: %v", err)
			vm.setError(err)
			return err
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
		CONTAINER_MAGIC,
		vm.instancePath,
		fmt.Sprintf("%v", vm.ID),
		hn,
		vm.effectivePath,
		vm.Memory,
		uuidPath,
		fmt.Sprintf("%v", vm.Fifos),
		preinit,
	}
	args = append(args, vm.Init...)

	// launch the container
	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: args,
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

	if err = cmd.Start(); err != nil {
		vm.overlayUnmount()
		log.Error("start container: %v", err)
		vm.setError(err)
		return err
	}

	vm.pid = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.ID, vm.pid)

	// log the child
	childLog.Close()
	log.LogAll(parentLog, log.DEBUG, "containerShim")

	go vm.console(parentStdin, parentStdout, parentStderr)

	// TODO: add affinity funcs for containers
	// vm.CheckAffinity()

	// network creation for containers happens /after/ the container is
	// started, as we need the PID in order to attach a veth to the container
	// side of the network namespace. That means that unlike kvm vms, we MUST
	// create/destroy taps on launch/kill boundaries (kvm destroys taps on
	// flush).

	// create and add taps if we are associated with any networks
	// expose the network namespace to iptool
	if err = vm.symlinkNetns(); err != nil {
		log.Error("symlinkNetns: %v", err)
	}

	if err == nil {
		for i := range vm.Networks {
			net := &vm.Networks[i]

			br, err := bridges.Get(net.Bridge)
			if err != nil {
				log.Error("get bridge: %v", err)
				break
			}

			net.Tap, err = br.CreateContainerTap(net.Tap, vm.netns, net.MAC, net.VLAN, i)
			if err != nil {
				break
			}

			updates := make(chan ipmac.IP)
			go vm.macSnooper(net, updates)

			br.AddMac(net.MAC, updates)
		}
	}

	if err == nil && len(vm.Networks) > 0 {
		if err = vm.writeTaps(); err != nil {
			log.Errorln(err)
		}
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

	ccPath := filepath.Join(vm.effectivePath, "cc")

	if err == nil {
		// connect cc. Note that we have a local err here because we don't want
		// to prevent the VM from continuing to launch, even if we can't
		// connect to cc.
		if err := ccNode.ListenUnix(ccPath); err != nil {
			log.Warn("unable to connect to cc for vm %v: %v", vm.ID, err)
		}
	}

	if err != nil {
		// Some error occurred.. clean up the process
		cmd.Process.Kill()

		vm.setError(err)
		return err
	}

	go func() {
		var err error
		cgroupPath := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID))
		sendKillAck := false

		// Channel to signal when the process has exited
		var waitChan = make(chan bool)

		// Create goroutine to wait for process to exit
		go func() {
			err = cmd.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			log.Info("VM %v exited", vm.ID)

			vm.lock.Lock()
			defer vm.lock.Unlock()

			// we don't need to check the error for a clean kill,
			// as there's no way to get here if we killed it.
			if err != nil {
				log.Error("kill container: %v", err)
				vm.setError(err)
			}
		case <-vm.kill:
			log.Info("Killing VM %v", vm.ID)

			vm.lock.Lock()
			defer vm.lock.Unlock()

			cmd.Process.Kill()

			// containers cannot return unless thawed, so thaw the
			// process if necessary
			if err = vm.thaw(); err != nil {
				log.Errorln(err)
				vm.setError(err)
			}

			// wait for the taskset to actually exit (from
			// uninterruptible sleep state), or timeout.
			start := time.Now()

			for {
				if time.Since(start) > CONTAINER_KILL_TIMEOUT {
					err = fmt.Errorf("container kill timeout")
					log.Errorln(err)
					vm.setError(err)
					break
				}
				t, err := ioutil.ReadFile(filepath.Join(cgroupPath, "tasks"))
				if err != nil {
					log.Errorln(err)
					vm.setError(err)
					break
				}
				if len(t) == 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			sendKillAck = true // wait to ack until we've cleaned up
		}

		err = ccNode.CloseUDS(ccPath)
		if err != nil {
			log.Errorln(err)
		}

		vm.listener.Close()
		vm.unlinkNetns()

		for _, net := range vm.Networks {
			br, err := bridges.Get(net.Bridge)
			if err != nil {
				log.Error("get bridge: %v", err)
			} else {
				br.DelMac(net.MAC)
				br.DestroyTap(net.Tap)
			}
		}

		// clean up the cgroup directory
		err = os.Remove(cgroupPath)
		if err != nil {
			log.Errorln(err)
		}

		if vm.State != VM_ERROR {
			// Set to QUIT unless we've already been put into the error state
			vm.setState(VM_QUIT)
		}

		if sendKillAck {
			killAck <- vm.ID
		}
	}()

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

func (vm *ContainerVM) freeze() error {
	freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
	if err := ioutil.WriteFile(freezer, []byte("FROZEN"), 0644); err != nil {
		return fmt.Errorf("freezer: %v", err)
	}

	return nil
}

func (vm *ContainerVM) thaw() error {
	freezer := filepath.Join(CGROUP_PATH, fmt.Sprintf("%v", vm.ID), "freezer.state")
	if err := ioutil.WriteFile(freezer, []byte("THAWED"), 0644); err != nil {
		return fmt.Errorf("freezer: %v", err)
	}

	return nil
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
	// walk CGROUP_PATH for tasks, killing each one
	if _, err := os.Stat(CGROUP_PATH); err == nil {
		err := filepath.Walk(CGROUP_PATH, containerNukeWalker)
		if err != nil {
			log.Errorln(err)
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

	err = containerCleanCgroupDirs()
	if err != nil {
		log.Errorln(err)
	}

	// umount cgroup_root
	err = syscall.Unmount(CGROUP_ROOT, 0)
	if err != nil {
		log.Error("cgroup_root unmount: %v", err)
	}

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

		pids := strings.Fields(string(d))
		for _, pid := range pids {
			log.Debug("found pid: %v", pid)

			fmt.Println("killing process:", pid)
			processWrapper("kill", "-9", pid)
		}
	}

	return nil
}

// remove state across cgroup mounts
func containerCleanCgroupDirs() error {
	_, err := os.Stat(CGROUP_PATH)
	if err != nil {
		return nil
	}

	err = filepath.Walk(CGROUP_PATH, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if path == CGROUP_PATH {
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
		return err
	}

	return os.Remove(CGROUP_PATH)
}
