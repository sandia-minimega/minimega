// Copyright 2016-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package recovery

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type QemuProcess struct {
	PID  int
	UUID string
	VMID string
}

type MinimegaQemuKVM struct {
	QemuProcess

	Namespace string
	State     string
}

// RecoverVMs attempts to read through and recover each VM linked in the
// namespaces directory. If a VM's state is either RUNNING or PAUSED, an
// existing QEMU process is searched for.
func RecoverVMs(root string) (map[string][]MinimegaQemuKVM, error) {
	_, err := os.Stat(filepath.Join(root, "namespaces"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		} else {
			return nil, fmt.Errorf("inspecting minimega namespaces: %w", err)
		}
	}

	procs, err := processes()
	if err != nil {
		return nil, fmt.Errorf("discovering QEMU processes: %w", err)
	}

	// namespace --> slice of minimega KVMs
	vms := make(map[string][]MinimegaQemuKVM)

	err = filepath.Walk(filepath.Join(root, "namespaces"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// if path is <root>/namespaces/foobar/<uuid>, then rel is foobar/<uuid>
		rel, _ := filepath.Rel(filepath.Join(root, "namespaces"), path)
		// details[0] is namespace, details[1] is vm uuid
		details := strings.Split(rel, "/")

		link, err := filepath.EvalSymlinks(path)
		if err != nil {
			log.Warn("vm %s in namespace %s doesn't have a valid ID -- skipping", details[1], details[0])
			return nil
		}

		vmid, _ := filepath.Rel(root, link)

		vm := MinimegaQemuKVM{
			QemuProcess: QemuProcess{
				VMID: vmid,
				UUID: details[1],
			},
			Namespace: details[0],
		}

		body, err := ioutil.ReadFile(filepath.Join(link, "state"))
		if err == nil {
			vm.State = strings.TrimSpace(string(body))
		} else if errors.Is(err, os.ErrNotExist) {
			vm.State = "BUILDING"
		} else {
			return fmt.Errorf("reading state file for vm %s: %w", vmid, err)
		}

		if vm.State == "BUILDING" || vm.State == "RUNNING" || vm.State == "PAUSED" {
			if proc, ok := procs[vm.UUID]; ok {
				if proc.VMID == vm.VMID {
					vm.PID = proc.PID
				} else {
					return fmt.Errorf("unknown mismatch between vm with UUID %s and ID %s", vm.UUID, proc.VMID)
				}
			} else {
				vm.State = "QUIT"
			}
		}

		vms[vm.Namespace] = append(vms[vm.Namespace], vm)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("recovering vms: %w", err)
	}

	return vms, nil
}

// processes finds all the `qemu-system-x86` processes running and determines
// which ones are related to minimega by looking at the process arguments for
// specific -name and -uuid options.
func processes() (map[string]QemuProcess, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}

	defer d.Close()

	procs := make(map[string]QemuProcess)

	for {
		names, err := d.Readdirnames(10)
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		for _, name := range names {
			// We only care if the name starts with a numeric
			if name[0] < '0' || name[0] > '9' {
				continue
			}

			// From this point forward, any errors we just ignore, because
			// it might simply be that the process doesn't exist anymore.
			pid, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}

			body, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			if err != nil {
				continue
			}

			// First, parse out the image name
			data := string(body)
			start := strings.IndexRune(data, '(') + 1
			end := strings.IndexRune(data[start:], ')')

			if name := data[start : start+end]; name == "qemu-system-x86" {
				body, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
				if err != nil {
					continue
				}

				data := string(body)
				args := strings.Split(data, "\x00")

				var name string
				var uuid string

				for i := len(args) - 1; i >= 0; i-- {
					switch args[i] {
					case "-name":
						name = args[i+1]
					case "-uuid":
						uuid = args[i+1]
					}

					if name != "" && uuid != "" {
						break
					}
				}

				if name != "" && uuid != "" {
					procs[uuid] = QemuProcess{PID: int(pid), UUID: uuid, VMID: name}
				}
			}
		}
	}

	return procs, nil
}
