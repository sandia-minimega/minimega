package main

import (
	"reflect"
	"strings"
	"testing"
)

func testAndroidConfig() VMConfig {
	cfg := NewVMConfig()
	cfg.AndroidConfig.AVDName = "Pixel_9a"

	return cfg
}

func TestNewAndroidRequiresAVDName(t *testing.T) {
	cfg := NewVMConfig()

	_, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err == nil {
		t.Fatal("expected NewAndroid to fail without android-avd")
	}
}

func TestNewAndroidNormalizesNetworkDrivers(t *testing.T) {
	cfg := testAndroidConfig()
	cfg.BaseConfig.Networks = NetConfigs{
		{
			Alias:  "100",
			VLAN:   100,
			Bridge: DefaultBridge,
			Driver: "",
		},
		{
			Alias:  "101",
			VLAN:   101,
			Bridge: DefaultBridge,
			Driver: DefaultKVMDriver,
		},
		{
			Alias:  "102",
			VLAN:   102,
			Bridge: DefaultBridge,
			Driver: DefaultAndroidNetDriver,
		},
		{
			Alias:  "103",
			VLAN:   103,
			Bridge: DefaultBridge,
			Driver: "custom-driver",
		},
	}

	vm, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err != nil {
		t.Fatalf("NewAndroid failed: %v", err)
	}
	vm.lock.Unlock()

	got := []string{
		vm.Networks[0].Driver,
		vm.Networks[1].Driver,
		vm.Networks[2].Driver,
		vm.Networks[3].Driver,
	}

	want := []string{
		DefaultAndroidNetDriver,
		DefaultAndroidNetDriver,
		DefaultAndroidNetDriver,
		"custom-driver",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected Android network drivers: got %#v, want %#v", got, want)
	}
}

func TestFilterAndroidQEMUArgs(t *testing.T) {
	in := []string{
		"-name", "0",
		"-m", "4096",
		"-vnc", "unix:/tmp/minimega/0/vnc",
		"-smp", "8",
		"-qmp", "unix:/tmp/minimega/0/qmp,server=on",
		"-vga", "std",
		"-net", "none",
		"-netdev", "tap,id=mega_tap0,script=no,ifname=mega_tap0",
		"-device", "driver=virtio-net-pci,netdev=mega_tap0,mac=00:11:22:33:44:55,bus=pci.1,addr=0x1",
		"-uuid", "00000000-0000-0000-0000-000000000000",
	}

	got := filterAndroidQEMUArgs(in)

	want := []string{
		"-name", "0",
		"-m", "4096",
		"-smp", "8",
		"-qmp", "unix:/tmp/minimega/0/qmp,server=on",
		"-net", "none",
		"-netdev", "tap,id=mega_tap0,script=no,ifname=mega_tap0",
		"-device", "driver=virtio-net-pci,netdev=mega_tap0,mac=00:11:22:33:44:55,bus=pci.1,addr=0x1",
		"-uuid", "00000000-0000-0000-0000-000000000000",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered args:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestAndroidInfoFields(t *testing.T) {
	cfg := testAndroidConfig()

	vm, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err != nil {
		t.Fatalf("NewAndroid failed: %v", err)
	}
	vm.lock.Unlock()

	vm.ConsolePort = 5554
	vm.ADBPort = 5555
	vm.serial = "emulator-5554"

	tests := map[string]string{
		"android_avd":          "Pixel_9a",
		"android_console_port": "5554",
		"android_adb_port":     "5555",
		"android_serial":       "emulator-5554",
		"type":                 "android",
		"name":                 "phone0",
	}

	for field, want := range tests {
		got, err := vm.Info(field)
		if err != nil {
			t.Fatalf("Info(%q) failed: %v", field, err)
		}

		if got != want {
			t.Fatalf("Info(%q) = %q, want %q", field, got, want)
		}
	}
}

func TestAndroidConsoleBasePortValidation(t *testing.T) {
	tests := []struct {
		name    string
		port    uint64
		wantErr bool
	}{
		{
			name:    "unset",
			port:    0,
			wantErr: false,
		},
		{
			name:    "valid default emulator range",
			port:    5554,
			wantErr: false,
		},
		{
			name:    "valid custom even port",
			port:    5570,
			wantErr: false,
		},
		{
			name:    "low port",
			port:    80,
			wantErr: true,
		},
		{
			name:    "odd port",
			port:    5555,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAndroidConsoleBasePortValue(tt.port)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error for port %d", tt.port)
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for port %d: %v", tt.port, err)
			}
		})
	}
}

func TestAndroidCopyDeepCopiesConfigs(t *testing.T) {
	cfg := testAndroidConfig()
	cfg.AndroidConfig.ExtraArgs = []string{"-read-only"}
	cfg.KVMConfig.QemuAppend = []string{"-foo"}
	cfg.BaseConfig.Networks = NetConfigs{
		{
			Alias:  "100",
			VLAN:   100,
			Bridge: DefaultBridge,
			Driver: DefaultKVMDriver,
		},
	}

	vm, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err != nil {
		t.Fatalf("NewAndroid failed: %v", err)
	}
	vm.lock.Unlock()

	copyVM, ok := vm.Copy().(*AndroidVM)
	if !ok {
		t.Fatal("AndroidVM.Copy did not return *AndroidVM")
	}

	vm.AndroidConfig.ExtraArgs[0] = "-changed"
	vm.KVMConfig.QemuAppend[0] = "-changed"
	vm.BaseVM.Networks[0].Driver = "changed-driver"

	if got, want := copyVM.AndroidConfig.ExtraArgs[0], "-read-only"; got != want {
		t.Fatalf("AndroidConfig was not deep copied: got %q, want %q", got, want)
	}

	if got, want := copyVM.KVMConfig.QemuAppend[0], "-foo"; got != want {
		t.Fatalf("KVMConfig was not deep copied: got %q, want %q", got, want)
	}

	if got, want := copyVM.BaseVM.Networks[0].Driver, DefaultAndroidNetDriver; got != want {
		t.Fatalf("BaseConfig networks were not deep copied or normalized: got %q, want %q", got, want)
	}
}

func TestAndroidEmulatorArgs(t *testing.T) {
	cfg := testAndroidConfig()
	cfg.AndroidConfig.NoWindow = true
	cfg.AndroidConfig.WritableSystem = true
	cfg.AndroidConfig.ExtraArgs = []string{"-read-only", "-verbose"}

	vm, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err != nil {
		t.Fatalf("NewAndroid failed: %v", err)
	}
	vm.lock.Unlock()

	vm.ConsolePort = 5554

	args := vm.emulatorArgs("/tmp/android-emulator.log")

	wantPrefix := []string{
		"-avd", "Pixel_9a",
		"-port", "5554",
		"-stdouterr-file", "/tmp/android-emulator.log",
		"-no-window",
		"-writable-system",
		"-read-only",
		"-verbose",
	}

	if len(args) < len(wantPrefix) {
		t.Fatalf("args too short: %#v", args)
	}

	if !reflect.DeepEqual(args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("unexpected emulator args prefix:\ngot  %#v\nwant %#v", args[:len(wantPrefix)], wantPrefix)
	}

	qemuIdx := -1
	for i, arg := range args {
		if arg == "-qemu" {
			qemuIdx = i
			break
		}
	}

	if qemuIdx == -1 {
		t.Fatalf("expected -qemu in args: %#v", args)
	}

	if qemuIdx <= len(wantPrefix)-1 {
		t.Fatalf("-qemu appeared before Android args were complete: %#v", args)
	}
}

func TestAndroidEnv(t *testing.T) {
	cfg := testAndroidConfig()
	cfg.AndroidConfig.SDKPath = "/opt/android-sdk"
	cfg.AndroidConfig.AVDDir = "/opt/android-avd"

	vm, err := NewAndroid("phone0", DefaultNamespace, cfg)
	if err != nil {
		t.Fatalf("NewAndroid failed: %v", err)
	}
	vm.lock.Unlock()

	env := vm.androidEnv()

	getLastEnv := func(key string) (string, bool) {
		prefix := key + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], prefix) {
				return strings.TrimPrefix(env[i], prefix), true
			}
		}
		return "", false
	}

	requireEnvEqual := func(key, want string) {
		t.Helper()

		got, ok := getLastEnv(key)
		if !ok {
			t.Fatalf("missing env key %s in %#v", key, env)
		}

		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	requireEnvContains := func(key, want string) {
		t.Helper()

		got, ok := getLastEnv(key)
		if !ok {
			t.Fatalf("missing env key %s in %#v", key, env)
		}

		if !strings.Contains(got, want) {
			t.Fatalf("%s does not contain %q: %q", key, want, got)
		}
	}

	requireEnvEqual("ANDROID_SDK_ROOT", "/opt/android-sdk")
	requireEnvEqual("ANDROID_HOME", "/opt/android-sdk")
	requireEnvEqual("ANDROID_AVD_HOME", "/opt/android-avd")
	requireEnvContains("LD_LIBRARY_PATH", "/opt/android-sdk/emulator/lib")
	requireEnvContains("LD_LIBRARY_PATH", "/opt/android-sdk/emulator/lib64")
	requireEnvContains("LD_LIBRARY_PATH", "/opt/android-sdk/emulator/lib64/qt/lib")
}

func TestParseVMTypeAndroid(t *testing.T) {
	vmType, err := ParseVMType("android")
	if err != nil {
		t.Fatalf("ParseVMType(android) failed: %v", err)
	}

	if vmType != ANDROID {
		t.Fatalf("ParseVMType(android) = %v, want %v", vmType, ANDROID)
	}

	if got := ANDROID.String(); got != "android" {
		t.Fatalf("ANDROID.String() = %q, want android", got)
	}
}

func TestNewVMAndroid(t *testing.T) {
	cfg := testAndroidConfig()

	vm, err := NewVM("phone0", DefaultNamespace, ANDROID, cfg)
	if err != nil {
		t.Fatalf("NewVM android failed: %v", err)
	}
	vm.(*AndroidVM).lock.Unlock()

	if vm.GetType() != ANDROID {
		t.Fatalf("NewVM android type = %v, want %v", vm.GetType(), ANDROID)
	}
}

func TestReserveAndroidPortPair(t *testing.T) {
	console, adb, err := reserveAndroidPortPair(5600)
	if err != nil {
		t.Fatalf("reserveAndroidPortPair failed: %v", err)
	}
	defer releaseAndroidPortPair(console)

	if console%2 != 0 {
		t.Fatalf("console port is not even: %d", console)
	}

	if adb != console+1 {
		t.Fatalf("adb port = %d, want console+1 %d", adb, console+1)
	}

	console2, adb2, err := reserveAndroidPortPair(5600)
	if err != nil {
		t.Fatalf("second reserveAndroidPortPair failed: %v", err)
	}
	defer releaseAndroidPortPair(console2)

	if console2 == console || adb2 == adb {
		t.Fatalf("reserved duplicate port pair: first %d/%d second %d/%d", console, adb, console2, adb2)
	}
}
