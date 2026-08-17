package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"
)

type AndroidConfig struct {
	// Configure the host-side Android SDK root directory.
	//
	// This path is interpreted on the minimega host and is not treated as a
	// file served from the minimega files directory.
	SDKPath string `config:"android-sdk" path:"false"`

	// Configure the host-side Android emulator binary.
	//
	// This value may be an absolute path or a binary name resolvable via the
	// host PATH.
	EmulatorPath string `config:"android-emulator" path:"false"`

	// Configure the host-side adb binary.
	//
	// This value may be an absolute path or a binary name resolvable via the
	// host PATH.
	ADBPath string `config:"android-adb" path:"false"`

	// Configure the Android Virtual Device (AVD) name to use when launching
	// Android VMs.
	AVDName string `config:"android-avd"`

	// Configure the host-side directory containing Android AVD data.
	//
	// This should generally contain entries such as:
	//   <avd-dir>/<avd-name>.avd
	AVDDir string `config:"android-avd-dir" path:"false"`

	// Launch Android VMs without creating a local emulator window.
	//
	// Default: true
	NoWindow bool `config:"android-no-window"`

	// Configure the preferred starting console port for Android emulator instances.
	//
	// This value is a hint, not a guaranteed assignment. minimega searches for the
	// first available Android console/ADB port pair starting at this port. If the
	// requested pair is already reserved or unavailable, the next valid pair is
	// used.
	//
	// If set to 0, minimega starts searching at the beginning of the valid Android
	// emulator console port range. Console ports are even ports in the range
	// 5554-5680, and the corresponding ADB port is console+1.
	//
	// If nonzero, this value must be an even port in the valid console port range.
	// Starting later in the range reduces the number of candidate port pairs.
	//
	// The valid range contains 64 console/ADB port pairs, so a single minimega
	// host can run at most 64 Android emulator VMs concurrently, and fewer if some
	// ports in the range are already in use. In a multi-host namespace, this limit
	// applies independently to each host.
	//
	// Default: 0
	ConsoleBasePort uint64 `config:"android-console-base-port" validate:"validateAndroidConsoleBasePort"`

	// Additional raw arguments to append to the Android emulator command line.
	ExtraArgs []string `config:"android-extra-args"`

	// Request writable system behavior for the Android emulator runtime.
	//
	// Default: false
	WritableSystem bool `config:"android-writable-system"`
}

func (old AndroidConfig) Copy() AndroidConfig {
	res := old
	res.ExtraArgs = make([]string, len(old.ExtraArgs))
	copy(res.ExtraArgs, old.ExtraArgs)
	return res
}

func (vm *AndroidConfig) String() string {
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Android configuration:")
	fmt.Fprintf(w, "SDK Path:\t%v\n", vm.SDKPath)
	fmt.Fprintf(w, "Emulator Path:\t%v\n", vm.EmulatorPath)
	fmt.Fprintf(w, "ADB Path:\t%v\n", vm.ADBPath)
	fmt.Fprintf(w, "AVD Name:\t%v\n", vm.AVDName)
	fmt.Fprintf(w, "AVD Dir:\t%v\n", vm.AVDDir)
	fmt.Fprintf(w, "No Window:\t%v\n", vm.NoWindow)
	fmt.Fprintf(w, "Console Base Port:\t%v\n", vm.ConsoleBasePort)
	fmt.Fprintf(w, "Extra Args:\t%v\n", vm.ExtraArgs)
	fmt.Fprintf(w, "Writable System:\t%v\n", vm.WritableSystem)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func androidConfiguredAny() bool {
	for _, ns := range namespaces {
		if androidConfigured(ns.vmConfig.AndroidConfig) {
			return true
		}
	}
	return false
}

func findAndroidTool(path, name string) (string, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}

	return exec.LookPath(name)
}

func validateAndroidConsoleBasePortValue(port uint64) error {
	if port == 0 {
		return nil
	}

	if port%2 != 0 {
		return fmt.Errorf("android-console-base-port must be 0 or an even port")
	}

	if port < MinAndroidConsolePort || port > MaxAndroidConsolePort {
		return fmt.Errorf(
			"android-console-base-port must be 0 or an even port in range %d-%d",
			MinAndroidConsolePort,
			MaxAndroidConsolePort,
		)
	}

	return nil
}

func validateAndroidConsoleBasePort(_ VMConfig, port uint64) error {
	return validateAndroidConsoleBasePortValue(port)
}

func validateAndroidConfig(cfg AndroidConfig) error {
	return validateAndroidConsoleBasePortValue(cfg.ConsoleBasePort)
}

func androidAVDExists(cfg AndroidConfig) error {
	if cfg.AVDName == "" || cfg.AVDDir == "" {
		return nil
	}

	p := filepath.Join(cfg.AVDDir, cfg.AVDName+".avd")
	if _, err := os.Stat(p); err != nil {
		return err
	}

	return nil
}

func androidConfigured(cfg AndroidConfig) bool {
	return cfg.SDKPath != "" ||
		cfg.EmulatorPath != "" ||
		cfg.ADBPath != "" ||
		cfg.AVDName != "" ||
		cfg.AVDDir != "" ||
		cfg.ConsoleBasePort != 0 ||
		len(cfg.ExtraArgs) > 0 ||
		cfg.WritableSystem
}

func checkAndroidDependencies(cfg AndroidConfig) error {
	if err := validateAndroidConfig(cfg); err != nil {
		return err
	}

	if cfg.SDKPath != "" {
		if _, err := os.Stat(cfg.SDKPath); err != nil {
			return fmt.Errorf("android sdk path invalid: %v", err)
		}
	}

	if _, err := findAndroidTool(cfg.EmulatorPath, "emulator"); err != nil {
		return fmt.Errorf("android emulator not found: %v", err)
	}

	if _, err := findAndroidTool(cfg.ADBPath, "adb"); err != nil {
		return fmt.Errorf("android adb not found: %v", err)
	}

	if !lsModule("kvm") {
		return fmt.Errorf("android requires kvm but kvm kernel module not detected")
	}

	if err := androidAVDExists(cfg); err != nil {
		return fmt.Errorf("android avd invalid: %v", err)
	}

	return nil
}
