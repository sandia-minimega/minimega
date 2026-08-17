package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/qmp"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	MinAndroidConsolePort = 5554

	// Highest console port minimega will use as the base of a console/ADB pair.
	// The ADB port is console+1. The default Android emulator range is 5554 to 5682
	// allowing for 64 concurrent virtual devices per host as detailed in:
	// https://developer.android.com/studio/run/emulator-commandline
	MaxAndroidConsolePort = 5680
	MaxAndroidVMsPerHost  = (MaxAndroidConsolePort-MinAndroidConsolePort)/2 + 1
)

type AndroidVM struct {
	*BaseVM       // embed
	KVMConfig     // embed; reused for backend QEMU argument construction
	AndroidConfig // embed

	ConsolePort int
	ADBPort     int

	serial string
	cmd    *exec.Cmd
	q      qmp.Conn
}

// Ensure that AndroidVM implements the VM interface.
var _ VM = (*AndroidVM)(nil)

var (
	androidPortMu       sync.Mutex
	androidReservedPort = map[int]bool{}
)

func NewAndroid(name, namespace string, config VMConfig) (*AndroidVM, error) {
	vm := new(AndroidVM)

	vm.BaseVM = NewBaseVM(name, namespace, config)
	vm.Type = ANDROID

	vm.KVMConfig = config.KVMConfig.Copy()
	vm.AndroidConfig = config.AndroidConfig.Copy()

	if vm.AVDName == "" {
		return nil, errors.New("unable to create android VM without a configured android-avd")
	}

	return vm, nil
}

func (vm *AndroidVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func (vm *AndroidVM) Copy() VM {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm2 := new(AndroidVM)

	// Make shallow copies of all fields.
	*vm2 = *vm

	// Make deep copies.
	vm2.BaseVM = vm.BaseVM.copy()
	vm2.KVMConfig = vm.KVMConfig.Copy()
	vm2.AndroidConfig = vm.AndroidConfig.Copy()

	return vm2
}

func (vm *AndroidVM) Launch() error {
	defer vm.lock.Unlock()

	return vm.launch()
}

// setRecoverableErrorf records an Android runtime error without changing the VM
// state. This is used for errors where the emulator process may still be alive,
// so the VM should remain in a killable state.
func (vm *AndroidVM) setRecoverableErrorf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)

	log.Error("android vm %v: %v", vm.ID, err)
	vm.Tags["error"] = err.Error()

	return err
}

func (vm *AndroidVM) Recover(id string, pid int) error {
	// Full Android recovery is intentionally deferred.
	vm.ID, _ = strconv.Atoi(id)
	vm.Pid = pid
	vm.instancePath = filepath.Join(*f_base, id)

	vm.lock.Unlock()
	return nil
}

func (vm *AndroidVM) Start() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&VM_RUNNING != 0 {
		return nil
	}

	if vm.State == VM_QUIT || vm.State == VM_ERROR {
		log.Info("relaunching android VM: %v", vm.ID)

		vm.kill = make(chan bool)

		if err := vm.launch(); err != nil {
			return err
		}
	}

	log.Info("starting android VM: %v", vm.ID)

	if err := vm.q.Start(); err != nil {
		return vm.setRecoverableErrorf("unable to start android VM: %v", err)
	}

	vm.setState(VM_RUNNING)

	return nil
}

func (vm *AndroidVM) Stop() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State != VM_RUNNING {
		return vmNotRunning(strconv.Itoa(vm.ID))
	}

	log.Info("stopping android VM: %v", vm.ID)

	if err := vm.q.Stop(); err != nil {
		return vm.setRecoverableErrorf("unable to stop android VM: %v", err)
	}

	vm.setState(VM_PAUSED)

	return nil
}

func (vm *AndroidVM) Flush() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	for _, net := range vm.Networks {
		// Android networking is currently unsupported, so Android VMs may have
		// configured networks without created taps. Nothing to clean up.
		if net.Tap == "" {
			continue
		}

		// Handle already disconnected taps differently since they are not
		// assigned to any bridges.
		if net.VLAN == DisconnectedVLAN {
			if err := bridge.DestroyTap(net.Tap); err != nil {
				log.Error("leaked tap %v: %v", net.Tap, err)
			}

			continue
		}

		br, err := getBridge(net.Bridge)
		if err != nil {
			return err
		}

		if err := br.DestroyTap(net.Tap); err != nil {
			log.Error("leaked tap %v: %v", net.Tap, err)
		}
	}

	return vm.BaseVM.Flush()
}

func (vm *AndroidVM) String() string {
	return fmt.Sprintf("%s:%d:android", hostname, vm.ID)
}

func (vm *AndroidVM) Info(field string) (string, error) {
	// Let BaseVM answer generic fields first.
	if v, err := vm.BaseVM.Info(field); err == nil {
		return v, nil
	}

	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch field {
	case "android_avd":
		return vm.AVDName, nil
	case "android_console_port":
		return strconv.Itoa(vm.ConsolePort), nil
	case "android_adb_port":
		return strconv.Itoa(vm.ADBPort), nil
	case "android_serial":
		return vm.serial, nil
	case "pid":
		return strconv.Itoa(vm.Pid), nil
	}

	// Prefer Android-specific config fields, then fall back to KVM config fields.
	if v, err := vm.AndroidConfig.Info(field); err == nil {
		return v, nil
	}

	return vm.KVMConfig.Info(field)
}

func (vm *AndroidVM) Conflicts(vm2 VM) error {
	switch vm2 := vm2.(type) {
	case *AndroidVM:
		return vm.conflictsAndroid(vm2)
	case *KvmVM:
		vm.lock.Lock()
		defer vm.lock.Unlock()

		if err := vm.conflictsKVMDisks(vm2.KVMConfig.Disks, vm2.Snapshot); err != nil {
			return err
		}
		return vm.BaseVM.conflicts(vm2.BaseVM)
	case *ContainerVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	}

	return errors.New("unknown VM type")
}

func (vm *AndroidVM) conflictsAndroid(vm2 *AndroidVM) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if err := vm.conflictsKVMDisks(vm2.KVMConfig.Disks, vm2.Snapshot); err != nil {
		return err
	}

	return vm.BaseVM.conflicts(vm2.BaseVM)
}

func (vm *AndroidVM) conflictsKVMDisks(disks DiskConfigs, snapshot bool) error {
	for _, d := range vm.Disks {
		for _, d2 := range disks {
			if d.Path == d2.Path && (!vm.Snapshot || !snapshot) {
				return fmt.Errorf("disk conflict with android vm %v: %v", vm.Name, d)
			}
		}
	}

	return nil
}

func (vm *AndroidVM) Screenshot(size int) ([]byte, error) {
	return nil, errors.New("cannot take screenshot of android VM yet")
}

func (vm *AndroidVM) Connect(cc *ron.Server, reconnect bool) error {
	// Android guest-agent/backchannel support is intentionally deferred.
	return nil
}

func (vm *AndroidVM) Disconnect(cc *ron.Server) error {
	// Android guest-agent/backchannel support is intentionally deferred.
	return nil
}

func (vm *AndroidVM) ProcStats() (map[int]*ProcStats, error) {
	if vm.Pid <= 0 {
		return nil, errors.New("android VM has no PID")
	}

	p, err := GetProcStats(vm.Pid)
	if err != nil {
		return nil, err
	}

	return map[int]*ProcStats{vm.Pid: p}, nil
}

func (vm *AndroidVM) WriteConfig(w io.Writer) error {
	if err := vm.BaseConfig.WriteConfig(w); err != nil {
		return err
	}

	if err := vm.KVMConfig.WriteConfig(w); err != nil {
		return err
	}

	return vm.AndroidConfig.WriteConfig(w)
}

// launch is the low-level Android launch function.
// Caller must hold vm.lock.
func (vm *AndroidVM) launch() error {
	log.Info("launching android vm: %v", vm.ID)

	if vm.State == VM_BUILDING {
		if err := os.MkdirAll(vm.instancePath, os.FileMode(0700)); err != nil {
			return fmt.Errorf("unable to create VM dir: %v", err)
		}

		if err := vm.createInstancePathAlias(); err != nil {
			return vm.setErrorf("createInstancePathAlias: %v", err)
		}
	}

	mustWrite(vm.path("name"), vm.Name)

	// From this point forward, vm.setErrorf() is safe because the instance
	// directory exists and the state file can be written.
	if err := validateAndroidLaunchConfig(vm.AndroidConfig); err != nil {
		return vm.setErrorf("android config invalid: %v", err)
	}

	emulator, err := findAndroidTool(vm.EmulatorPath, "emulator")
	if err != nil {
		return vm.setErrorf("android emulator not found: %v", err)
	}

	// adb is not strictly required to exec the emulator, but validating it here
	// catches common broken Android runtime configurations early.
	if _, err := findAndroidTool(vm.ADBPath, "adb"); err != nil {
		return vm.setErrorf("android adb not found: %v", err)
	}

	// Android emulator tap/network support is deferred. Avoid creating taps
	// and then failing later with obscure /dev/net/tun errors.
	if len(vm.Networks) > 0 || len(vm.Bonds) > 0 {
		return vm.setErrorf("android VM networking is not supported yet")
	}

	if vm.State == VM_BUILDING {
		// Android reuses KVMConfig/qemuArgs for backend QEMU arguments, so apply
		// the same disk snapshot behavior as KVM VMs.
		if vm.Snapshot {
			for i, d := range vm.Disks {
				dst := vm.path(fmt.Sprintf("disk-%v.qcow2", i))
				if err := diskSnapshot(d.Path, dst); err != nil {
					return vm.setErrorf("unable to snapshot %v: %v", d, err)
				}

				vm.Disks[i].SnapshotPath = dst
			}
		}
	}

	console, adb, err := reserveAndroidPortPair(vm.ConsoleBasePort)
	if err != nil {
		return vm.setErrorf("unable to reserve android console/adb port pair: %v", err)
	}

	vm.ConsolePort = console
	vm.ADBPort = adb
	vm.serial = fmt.Sprintf("emulator-%d", console)

	logFilePath := vm.path("android-emulator.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		releaseAndroidPortPair(console)
		return vm.setErrorf("unable to open android emulator log: %v", err)
	}

	args := vm.emulatorArgs(logFilePath)
	log.Debug("android emulator args for vm %v: %#v", vm.ID, args)

	cmd := &exec.Cmd{
		Path:   emulator,
		Args:   append([]string{emulator}, args...),
		Env:    vm.androidEnv(),
		Stdout: logFile,
		Stderr: logFile,
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		releaseAndroidPortPair(console)
		return vm.setErrorf("unable to start android emulator: %v", err)
	}

	vm.cmd = cmd
	vm.Pid = cmd.Process.Pid

	log.Info("android vm %v has pid %v", vm.ID, vm.Pid)

	waitChan := vm.waitForExit(cmd, logFile, console)

	if err := vm.connectQMP(); err != nil {
		cmd.Process.Kill()
		return vm.setErrorf("unable to connect to android QMP socket: %v", err)
	}

	go vm.qmpLogger()

	vm.waitToKill(cmd, waitChan)

	return nil
}

func (vm *AndroidVM) emulatorArgs(logFilePath string) []string {
	args := []string{
		"-avd", vm.AVDName,
		"-port", strconv.Itoa(vm.ConsolePort),
		"-stdouterr-file", logFilePath,
	}

	if vm.NoWindow {
		args = append(args, "-no-window")
	}

	if vm.WritableSystem {
		args = append(args, "-writable-system")
	}

	args = append(args, vm.ExtraArgs...)

	qemuArgs := vm.androidQEMUArgs()
	if len(qemuArgs) > 0 {
		args = append(args, "-qemu")
		args = append(args, qemuArgs...)
	}

	return args
}

func (vm *AndroidVM) androidQEMUArgs() []string {
	vmConfig := VMConfig{
		BaseConfig:    vm.BaseConfig,
		KVMConfig:     vm.KVMConfig,
		AndroidConfig: vm.AndroidConfig,
	}

	args := vmConfig.qemuArgs(vm.ID, vm.instancePath)
	args = vmConfig.applyQemuOverrides(args)

	return filterAndroidQEMUArgs(args)
}

// filterAndroidQEMUArgs implements important argument filtering.
// These are QEMU args generated by minimega that should not be
// passed to the Android emulator backend through QEMU.
func filterAndroidQEMUArgs(args []string) []string {
	argsWithValuesToRemove := map[string]bool{
		"-vnc": true,
		"-vga": true,
	}

	var res []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if argsWithValuesToRemove[arg] {
			// Skip this arg and its value, if present.
			if i+1 < len(args) {
				i++
			}
			continue
		}

		res = append(res, arg)
	}

	return res
}

func (vm *AndroidVM) androidEnv() []string {
	env := os.Environ()

	if vm.SDKPath != "" {
		env = append(env, "ANDROID_SDK_ROOT="+vm.SDKPath)
		env = append(env, "ANDROID_HOME="+vm.SDKPath)

		paths := []string{
			filepath.Join(vm.SDKPath, "emulator", "lib"),
			filepath.Join(vm.SDKPath, "emulator", "lib64"),
			filepath.Join(vm.SDKPath, "emulator", "lib64", "qt", "lib"),
		}

		if existing := os.Getenv("LD_LIBRARY_PATH"); existing != "" {
			paths = append([]string{existing}, paths...)
		}

		env = append(env, "LD_LIBRARY_PATH="+strings.Join(paths, string(os.PathListSeparator)))
	}

	if vm.AVDDir != "" {
		env = append(env, "ANDROID_AVD_HOME="+vm.AVDDir)
	}

	return env
}

func (vm *AndroidVM) connectQMP() (err error) {
	delay := QMP_CONNECT_DELAY * time.Millisecond

	for count := 0; count < QMP_CONNECT_RETRY; count++ {
		vm.q, err = qmp.Dial(vm.path("qmp"))
		if err == nil {
			log.Debug("android qmp dial to %v successful", vm.ID)
			return nil
		}

		log.Debug("android qmp dial to %v: %v, redialing in %v", vm.ID, err, delay)
		time.Sleep(delay)
	}

	return errors.New("android qmp timeout")
}

func (vm *AndroidVM) waitForExit(cmd *exec.Cmd, logFile *os.File, consolePort int) chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer logFile.Close()
		defer releaseAndroidPortPair(consolePort)

		err := cmd.Wait()

		vm.lock.Lock()
		defer vm.lock.Unlock()

		// Check if the process quit for some reason other than being killed.
		if err != nil && err.Error() != "signal: killed" {
			vm.setErrorf("android emulator exited: %v", err)
		} else if vm.State != VM_ERROR {
			vm.setState(VM_QUIT)
		}
	}()

	return done
}

func (vm *AndroidVM) waitToKill(cmd *exec.Cmd, done chan struct{}) {
	go func() {
		defer vm.cond.Signal()

		select {
		case <-done:
			log.Info("android VM %v exited", vm.ID)
		case <-vm.kill:
			log.Info("killing android VM %v", vm.ID)

			if cmd.Process != nil {
				cmd.Process.Kill()
			}

			<-done
		}
	}()
}

func (vm *AndroidVM) qmpLogger() {
	for v := vm.q.Message(); v != nil; v = vm.q.Message() {
		log.Info("Android VM %v received asynchronous QMP message: %v", vm.ID, v)
	}
}

func validateAndroidLaunchConfig(cfg AndroidConfig) error {
	if cfg.AVDName == "" {
		return errors.New("android-avd must be configured")
	}

	return checkAndroidDependencies(cfg)
}

func reserveAndroidPortPair(base uint64) (int, int, error) {
	if err := validateAndroidConsoleBasePortValue(base); err != nil {
		return 0, 0, err
	}

	start := MinAndroidConsolePort
	if base != 0 {
		start = int(base)
	}

	androidPortMu.Lock()
	defer androidPortMu.Unlock()

	for console := start; console <= MaxAndroidConsolePort; console += 2 {
		adb := console + 1

		if androidReservedPort[console] || androidReservedPort[adb] {
			continue
		}

		if !tcpPortPairAvailable(console, adb) {
			continue
		}

		androidReservedPort[console] = true
		androidReservedPort[adb] = true

		return console, adb, nil
	}

	return 0, 0, fmt.Errorf(
		"no available android console/adb port pair in range %d-%d; "+
			"each minimega host supports at most %d concurrent Android emulator VMs",
		start,
		MaxAndroidConsolePort+1,
		MaxAndroidVMsPerHost,
	)
}

func releaseAndroidPortPair(console int) {
	androidPortMu.Lock()
	defer androidPortMu.Unlock()

	delete(androidReservedPort, console)
	delete(androidReservedPort, console+1)
}

func tcpPortPairAvailable(console, adb int) bool {
	consoleListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", console))
	if err != nil {
		return false
	}
	defer consoleListener.Close()

	adbListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", adb))
	if err != nil {
		return false
	}
	defer adbListener.Close()

	return true
}
