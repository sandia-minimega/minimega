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
	DefaultAndroidConsolePort = 5554
	MaxAndroidConsolePort     = 5682

	// Android Emulator's QEMU backend does not support minimega's KVM default
	// e1000 NIC. Use virtio-net-pci for Android tap-backed NICs.
	DefaultAndroidNetDriver = "virtio-net-pci"

	AndroidQMPConnectRetry = 300
	AndroidQMPConnectDelay = 100
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

	// Normalize Android network devices to a NIC model supported by the Android
	// Emulator backend. This preserves minimega's existing NetConfig/qemuArgs flow
	// while avoiding the KVM default e1000 device, which Android's backend QEMU
	// rejects.
	for i := range vm.Networks {
		if vm.Networks[i].Driver == "" || vm.Networks[i].Driver == DefaultKVMDriver {
			vm.Networks[i].Driver = DefaultAndroidNetDriver
		}
	}

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
		return vm.setErrorf("unable to start android VM: %v", err)
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
		return vm.setErrorf("unable to stop android VM: %v", err)
	}

	vm.setState(VM_PAUSED)

	return nil
}

func (vm *AndroidVM) Flush() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	for _, net := range vm.Networks {
		// Android VMs can enter ERROR before taps are created. Nothing to clean
		// up in that case.
		if net.Tap == "" {
			continue
		}

		// Disconnected taps are no longer associated with a bridge.
		if net.VLAN == DisconnectedVLAN || net.Bridge == "" {
			if err := bridge.DestroyTap(net.Tap); err != nil {
				log.Error("leaked android tap %v: %v", net.Tap, err)
			}

			continue
		}

		br, err := getBridge(net.Bridge)
		if err != nil {
			// Be defensive during cleanup. If bridge lookup fails, still try to
			// destroy the tap directly.
			log.Warn("unable to get bridge %v while flushing android tap %v: %v", net.Bridge, net.Tap, err)

			if err2 := bridge.DestroyTap(net.Tap); err2 != nil {
				return fmt.Errorf("unable to clean android tap %v: bridge lookup failed: %v; direct destroy failed: %v", net.Tap, err, err2)
			}

			continue
		}

		if err := br.DestroyTap(net.Tap); err != nil {
			log.Error("leaked android tap %v: %v", net.Tap, err)
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

	emulator, err := findAndroidEmulator(vm.EmulatorPath)
	if err != nil {
		return vm.setErrorf("android emulator not found: %v", err)
	}

	// adb is not strictly required to exec the emulator, but validating it here
	// catches common broken Android runtime configurations early.
	if _, err := findADB(vm.ADBPath); err != nil {
		return vm.setErrorf("android adb not found: %v", err)
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

	if err := vm.createTaps(); err != nil {
		return err
	}

	// This MUST be done after vm.createTaps.
	if err := vm.createBonds(); err != nil {
		return err
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

	log.Debug("android backend qemu args before filter for vm %v: %#v", vm.ID, args)

	args = filterAndroidQEMUArgs(args)

	log.Debug("android backend qemu args after filter for vm %v: %#v", vm.ID, args)

	return args
}

// filterAndroidQEMUArgs implements the important parts of the old
// android-qemu-wrapper.sh argument filtering. These are QEMU args generated by
// minimega that should not be passed to the Android emulator backend through
// -qemu.
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
	delay := AndroidQMPConnectDelay * time.Millisecond

	for count := 0; count < AndroidQMPConnectRetry; count++ {
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
	androidPortMu.Lock()
	defer androidPortMu.Unlock()

	start := DefaultAndroidConsolePort
	stop := MaxAndroidConsolePort

	if base != 0 {
		start = int(base)
		if start%2 != 0 {
			start++
		}

		stop = start + 128
	}

	for port := start; port <= stop; port += 2 {
		adb := port + 1

		if androidReservedPort[port] || androidReservedPort[adb] {
			continue
		}

		if !tcpPortAvailable(port) || !tcpPortAvailable(adb) {
			continue
		}

		androidReservedPort[port] = true
		androidReservedPort[adb] = true

		return port, adb, nil
	}

	return 0, 0, fmt.Errorf("no available android console/adb port pair starting at %d", start)
}

func releaseAndroidPortPair(console int) {
	androidPortMu.Lock()
	defer androidPortMu.Unlock()

	delete(androidReservedPort, console)
	delete(androidReservedPort, console+1)
}

func tcpPortAvailable(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}

	l.Close()
	return true
}

func (vm *AndroidVM) createTapName(bridge string) (string, error) {
	br, err := getBridge(bridge)
	if err != nil {
		return "", vm.setErrorf("unable to get bridge %v: %v", bridge, err)
	}

	return br.CreateTapName(), nil
}

func (vm *AndroidVM) addTap(name, bridge, mac string, vlan int, qinq bool) (string, error) {
	br, err := getBridge(bridge)
	if err != nil {
		return name, vm.setErrorf("unable to get bridge %v: %v", bridge, err)
	}

	tap, err := br.CreateTap(name, mac, vlan)
	if err != nil {
		return tap, err
	}

	if qinq {
		if err := br.SetTapQinQ(tap, vlan); err != nil {
			return tap, err
		}
	}

	return tap, nil
}

func (vm *AndroidVM) createTaps() error {
	for i := range vm.Networks {
		nic := &vm.Networks[i]
		if nic.Tap != "" {
			// Tap has already been created.
			continue
		}

		tap, err := vm.addTap("", nic.Bridge, nic.MAC, nic.VLAN, nic.QinQ)
		if err != nil {
			return vm.setErrorf("unable to create android tap %v: %v", i, err)
		}

		nic.Tap = tap
	}

	if len(vm.Networks) > 0 {
		if err := vm.writeTaps(); err != nil {
			return vm.setErrorf("unable to write android taps: %v", err)
		}
	}

	return nil
}

func (vm *AndroidVM) createBonds() error {
	for i := range vm.Bonds {
		bond := &vm.Bonds[i]

		if bond.created {
			continue
		}

		if err := vm.addBond(bond); err != nil {
			return vm.setErrorf("unable to create android bond %v: %v", i, err)
		}

		bond.created = true
	}

	if len(vm.Bonds) > 0 {
		if err := vm.writeBonds(); err != nil {
			return vm.setErrorf("unable to write android bonds: %v", err)
		}
	}

	return nil
}

func (vm *AndroidVM) AddNIC(nic NetConfig) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if nic.MAC == "" {
		nic.MAC = randomMac()
	}

	// Android Emulator's backend QEMU does not support minimega's default
	// e1000 NIC, so normalize unspecified/default KVM drivers to Android's
	// supported default.
	if nic.Driver == "" || nic.Driver == DefaultKVMDriver {
		nic.Driver = DefaultAndroidNetDriver
	}

	var err error
	nic.Tap, err = vm.createTapName(nic.Bridge)
	if err != nil {
		return err
	}

	if _, err := vm.addTap(nic.Tap, nic.Bridge, nic.MAC, nic.VLAN, nic.QinQ); err != nil {
		return vm.setErrorf("unable to add android tap %v: %v", nic.Tap, err)
	}

	cleanupTap := func() {
		if nic.Bridge != "" && nic.VLAN != DisconnectedVLAN {
			if br, err := getBridge(nic.Bridge); err == nil {
				if err := br.DestroyTap(nic.Tap); err != nil {
					log.Error("unable to clean android tap %v after AddNIC failure: %v", nic.Tap, err)
				}
				return
			}
		}

		if err := bridge.DestroyTap(nic.Tap); err != nil {
			log.Error("unable to clean android tap %v after AddNIC failure: %v", nic.Tap, err)
		}
	}

	cleanupNetdev := func() {
		// Best-effort cleanup. NetDevAdd is issued through HMP, so use the same
		// path to remove it if device_add fails.
		cmd := fmt.Sprintf(
			`{"execute":"human-monitor-command","arguments":{"command-line":%q}}`,
			fmt.Sprintf("netdev_del %s", nic.Tap),
		)

		if _, err := vm.q.Raw(cmd); err != nil {
			log.Warn("unable to clean android netdev %v after AddNIC failure: %v", nic.Tap, err)
		}
	}

	r, err := vm.q.NetDevAdd("tap", nic.Tap, nic.Tap)
	if err != nil {
		cleanupTap()
		return err
	}
	if strings.TrimSpace(r) != "" {
		cleanupTap()
		return fmt.Errorf("android qmp netdev_add failed: %s", strings.TrimSpace(r))
	}
	log.Debugln("android qmp netdev_add response:", r)

	// Android Emulator's pci.0 is crowded/reserved by emulator-generated
	// devices. qemuArgs creates pci.1 for minimega devices, so hot-add Android
	// NICs there as well.
	r, err = vm.q.NicAdd(nic.Tap, nic.Tap, "pci.1", nic.Driver, nic.MAC)
	if err != nil {
		cleanupNetdev()
		cleanupTap()
		return err
	}
	if strings.TrimSpace(r) != "" {
		cleanupNetdev()
		cleanupTap()
		return fmt.Errorf("android qmp device_add failed: %s", strings.TrimSpace(r))
	}
	log.Debugln("android qmp device_add response:", r)

	vm.Networks = append(vm.Networks, nic)

	if err := vm.writeTaps(); err != nil {
		return vm.setErrorf("unable to write android taps: %v", err)
	}

	return nil
}
