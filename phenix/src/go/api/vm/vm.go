package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"phenix/api/experiment"
	"phenix/internal/common"
	"phenix/internal/file"
	"phenix/internal/mm"
	"phenix/internal/mm/mmcli"

	"golang.org/x/sync/errgroup"
)

var vlanAliasRegex = regexp.MustCompile(`(.*) \(\d*\)`)

func Count(expName string) (int, error) {
	if expName == "" {
		return 0, fmt.Errorf("no experiment name provided")
	}

	exp, err := experiment.Get(expName)
	if err != nil {
		return 0, fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	return len(exp.Spec.Topology().Nodes()), nil
}

// List collects VMs, combining topology settings with running VM details if the
// experiment is running. It returns a slice of VM structs and any errors
// encountered while gathering them.
func List(expName string) ([]mm.VM, error) {
	if expName == "" {
		return nil, fmt.Errorf("no experiment name provided")
	}

	exp, err := experiment.Get(expName)
	if err != nil {
		return nil, fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	var (
		running = make(map[string]mm.VM)
		vms     []mm.VM
	)

	if exp.Running() {
		for _, vm := range mm.GetVMInfo(mm.NS(expName)) {
			running[vm.Name] = vm
		}
	}

	for idx, node := range exp.Spec.Topology().Nodes() {
		vm := mm.VM{
			ID:         idx,
			Name:       node.General().Hostname(),
			Experiment: exp.Spec.ExperimentName(),
			CPUs:       node.Hardware().VCPU(),
			RAM:        node.Hardware().Memory(),
			Disk:       node.Hardware().Drives()[0].Image(),
			Interfaces: make(map[string]string),
			DoNotBoot:  *node.General().DoNotBoot(),
			OSType:     string(node.Hardware().OSType()),
		}

		for _, iface := range node.Network().Interfaces() {
			vm.IPv4 = append(vm.IPv4, iface.Address())
			vm.Networks = append(vm.Networks, iface.VLAN())
			vm.Interfaces[iface.VLAN()] = iface.Address()
		}

		if details, ok := running[vm.Name]; ok {
			vm.Host = details.Host
			vm.Running = details.Running
			vm.Networks = details.Networks
			vm.Taps = details.Taps
			vm.Uptime = details.Uptime
			vm.CPUs = details.CPUs
			vm.RAM = details.RAM
			vm.Disk = details.Disk

			// Reset slice of IPv4 addresses so we can be sure to align them correctly
			// with minimega networks below.
			vm.IPv4 = make([]string, len(details.Networks))

			// Since we get the IP from the experiment config, but the network name
			// from minimega (to preserve iface to network ordering), make sure the
			// ordering of IPs matches the odering of networks. We could just use a
			// map here, but then the iface to network ordering that minimega ensures
			// would be lost.
			for idx, nw := range details.Networks {
				// At this point, `nw` will look something like `EXP_1 (101)`. In the
				// experiment config, we just have `EXP_1` so we need to use that
				// portion from minimega as the `Interfaces` map key.
				if match := vlanAliasRegex.FindStringSubmatch(nw); match != nil {
					vm.IPv4[idx] = vm.Interfaces[match[1]]
				} else {
					vm.IPv4[idx] = "n/a"
				}
			}
		} else {
			vm.Host = exp.Spec.Schedules()[vm.Name]
		}

		vms = append(vms, vm)
	}

	return vms, nil
}

// Get retrieves the VM with the given name from the experiment with the given
// name. If the experiment is running, topology VM settings are combined with
// running VM details. It returns a pointer to a VM struct, and any errors
// encountered while retrieving the VM.
func Get(expName, vmName string) (*mm.VM, error) {
	if expName == "" {
		return nil, fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return nil, fmt.Errorf("no VM name provided")
	}

	exp, err := experiment.Get(expName)
	if err != nil {
		return nil, fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	var vm *mm.VM

	for idx, node := range exp.Spec.Topology().Nodes() {
		if node.General().Hostname() != vmName {
			continue
		}

		vm = &mm.VM{
			ID:         idx,
			Name:       node.General().Hostname(),
			Experiment: exp.Spec.ExperimentName(),
			CPUs:       node.Hardware().VCPU(),
			RAM:        node.Hardware().Memory(),
			Disk:       node.Hardware().Drives()[0].Image(),
			Interfaces: make(map[string]string),
			OSType:     string(node.Hardware().OSType()),
			Metadata:   make(map[string]interface{}),
		}

		for _, iface := range node.Network().Interfaces() {
			vm.IPv4 = append(vm.IPv4, iface.Address())
			vm.Networks = append(vm.Networks, iface.VLAN())
			vm.Interfaces[iface.VLAN()] = iface.Address()
		}

		for _, app := range exp.Apps() {
			for _, h := range app.Hosts() {
				if h.Hostname() == vm.Name {
					vm.Metadata[app.Name()] = h.Metadata
				}
			}
		}
	}

	if vm == nil {
		return nil, fmt.Errorf("VM %s not found in experiment %s", vmName, expName)
	}

	if !exp.Running() {
		vm.Host = exp.Spec.Schedules()[vm.Name]
		return vm, nil
	}

	details := mm.GetVMInfo(mm.NS(expName), mm.VMName(vmName))

	if len(details) != 1 {
		return vm, nil
	}

	vm.Host = details[0].Host
	vm.Running = details[0].Running
	vm.Networks = details[0].Networks
	vm.Taps = details[0].Taps
	vm.Uptime = details[0].Uptime
	vm.CPUs = details[0].CPUs
	vm.RAM = details[0].RAM
	vm.Disk = details[0].Disk

	// Reset slice of IPv4 addresses so we can be sure to align them correctly
	// with minimega networks below.
	vm.IPv4 = make([]string, len(details[0].Networks))

	// Since we get the IP from the experiment config, but the network name from
	// minimega (to preserve iface to network ordering), make sure the ordering of
	// IPs matches the odering of networks. We could just use a map here, but then
	// the iface to network ordering that minimega ensures would be lost.
	for idx, nw := range details[0].Networks {
		// At this point, `nw` will look something like `EXP_1 (101)`. In the exp,
		// we just have `EXP_1` so we need to use that portion from minimega as the
		// `Interfaces` map key.
		if match := vlanAliasRegex.FindStringSubmatch(nw); match != nil {
			vm.IPv4[idx] = vm.Interfaces[match[1]]
		} else {
			vm.IPv4[idx] = "n/a"
		}
	}

	return vm, nil
}

func Update(opts ...UpdateOption) error {
	o := newUpdateOptions(opts...)

	if o.exp == "" || o.vm == "" {
		return fmt.Errorf("experiment or VM name not provided")
	}

	running := experiment.Running(o.exp)

	if running && o.iface == nil {
		return fmt.Errorf("only interface connections can be updated while experiment is running")
	}

	// The only setting that can be updated while an experiment is running is the
	// VLAN an interface is connected to.
	if running {
		if o.iface.vlan == "" {
			return Disonnect(o.exp, o.vm, o.iface.index)
		} else {
			return Connect(o.exp, o.vm, o.iface.index, o.iface.vlan)
		}
	}

	exp, err := experiment.Get(o.exp)
	if err != nil {
		return fmt.Errorf("unable to get experiment %s: %w", o.exp, err)
	}

	vm := exp.Spec.Topology().FindNodeByName(o.vm)
	if vm == nil {
		return fmt.Errorf("unable to find VM %s in experiment %s", o.vm, o.exp)
	}

	if o.cpu != 0 {
		vm.Hardware().SetVCPU(o.cpu)
	}

	if o.mem != 0 {
		vm.Hardware().SetMemory(o.mem)
	}

	if o.disk != "" {
		vm.Hardware().Drives()[0].SetImage(o.disk)
	}

	if o.dnb != nil {
		vm.General().SetDoNotBoot(*o.dnb)
	}

	if o.host != nil {
		if *o.host == "" {
			delete(exp.Spec.Schedules(), o.vm)
		} else {
			exp.Spec.ScheduleNode(o.vm, *o.host)
		}
	}

	err = experiment.Save(experiment.SaveWithName(o.exp), experiment.SaveWithSpec(exp.Spec))
	if err != nil {
		return fmt.Errorf("unable to save experiment with updated VM: %w", err)
	}

	return nil
}

func Screenshot(expName, vmName, size string) ([]byte, error) {
	screenshot, err := mm.GetVMScreenshot(mm.NS(expName), mm.VMName(vmName), mm.ScreenshotSize(size))
	if err != nil {
		return nil, fmt.Errorf("getting VM screenshot: %w", err)
	}

	return screenshot, nil
}

// Pause stops a running VM with the given name in the experiment with the given
// name. It returns any errors encountered while pausing the VM.
func Pause(expName, vmName string) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	err := StopCaptures(expName, vmName)
	if err != nil && !errors.Is(err, ErrNoCaptures) {
		return fmt.Errorf("stopping captures for VM %s in experiment %s: %w", vmName, expName, err)
	}

	if err := mm.StopVM(mm.NS(expName), mm.VMName(vmName)); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	return nil
}

// Resume starts a paused VM with the given name in the experiment with the
// given name. It returns any errors encountered while resuming the VM.
func Resume(expName, vmName string) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	if err := mm.StartVM(mm.NS(expName), mm.VMName(vmName)); err != nil {
		return fmt.Errorf("resuming VM: %w", err)
	}

	return nil
}

// Redeploy redeploys a VM with the given name in the experiment with the given
// name. Multiple redeploy options can be passed to alter the resulting
// redeployed VM, such as CPU, memory, and disk options. It returns any errors
// encountered while redeploying the VM.
func Redeploy(expName, vmName string, opts ...RedeployOption) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	o := newRedeployOptions(opts...)

	var injects []string

	if o.inject {
		exp, err := experiment.Get(expName)
		if err != nil {
			return fmt.Errorf("getting experiment %s: %w", expName, err)
		}

		for _, n := range exp.Spec.Topology().Nodes() {
			if n.General().Hostname() != vmName {
				continue
			}

			if o.disk == "" {
				o.disk = n.Hardware().Drives()[0].Image()
				o.part = *n.Hardware().Drives()[0].InjectPartition()
			}

			for _, i := range n.Injections() {
				injects = append(injects, fmt.Sprintf("%s:%s", i.Src(), i.Dst()))
			}

			break
		}
	}

	mmOpts := []mm.Option{
		mm.NS(expName),
		mm.VMName(vmName),
		mm.CPU(o.cpu),
		mm.Mem(o.mem),
		mm.Disk(o.disk),
		mm.Injects(injects...),
		mm.InjectPartition(o.part),
	}

	if err := mm.RedeployVM(mmOpts...); err != nil {
		return fmt.Errorf("redeploying VM: %w", err)
	}

	return nil
}

// Kill deletes a VM with the given name in the experiment with the given name.
// It returns any errors encountered while killing the VM.
func Kill(expName, vmName string) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	if err := mm.KillVM(mm.NS(expName), mm.VMName(vmName)); err != nil {
		return fmt.Errorf("killing VM: %w", err)
	}

	return nil
}

func Snapshots(expName, vmName string) ([]string, error) {
	snapshots, err := file.GetExperimentSnapshots(expName)
	if err != nil {
		return nil, fmt.Errorf("getting list of experiment snapshots: %w", err)
	}

	var (
		prefix = fmt.Sprintf("%s__", vmName)
		names  []string
	)

	for _, ss := range snapshots {
		if strings.HasPrefix(ss, prefix) {
			names = append(names, ss)
		}
	}

	return names, nil
}

func Snapshot(expName, vmName, out string, cb func(string)) error {
	vm, err := Get(expName, vmName)
	if err != nil {
		return fmt.Errorf("getting VM details: %w", err)
	}

	if !vm.Running {
		return errors.New("VM is not running")
	}

	out = strings.TrimSuffix(out, filepath.Ext(out))
	out = fmt.Sprintf("%s_%s__%s", expName, vmName, out)

	// ***** BEGIN: SNAPSHOT VM *****

	// Get minimega's snapshot path for VM

	cmd := mmcli.NewNamespacedCommand(expName)
	cmd.Command = "vm info"
	cmd.Columns = []string{"host", "id"}
	cmd.Filters = []string{"name=" + vmName}

	status := mmcli.RunTabular(cmd)

	if len(status) == 0 {
		return fmt.Errorf("VM %s not found", vmName)
	}

	cmd.Columns = nil
	cmd.Filters = nil

	var (
		host = status[0]["host"]
		fp   = fmt.Sprintf("%s/%s", common.MinimegaBase, status[0]["id"])
	)

	qmp := fmt.Sprintf(`{ "execute": "query-block" }`)
	cmd.Command = fmt.Sprintf("vm qmp %s '%s'", vmName, qmp)

	res, err := mmcli.SingleResponse(mmcli.Run(cmd))
	if err != nil {
		return fmt.Errorf("querying for block device details for VM %s: %w", vmName, err)
	}

	var v map[string][]mm.BlockDevice
	json.Unmarshal([]byte(res), &v)

	var device string

	for _, dev := range v["return"] {
		if dev.Inserted != nil {
			if strings.HasPrefix(dev.Inserted.File, fp) {
				device = dev.Device
				break
			}
		}
	}

	target := fmt.Sprintf("%s/images/%s.qc2", common.PhenixBase, out)

	qmp = fmt.Sprintf(`{ "execute": "drive-backup", "arguments": { "device": "%s", "sync": "top", "target": "%s" } }`, device, target)
	cmd.Command = fmt.Sprintf(`vm qmp %s '%s'`, vmName, qmp)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("starting disk snapshot for VM %s: %w", vmName, err)
	}

	qmp = fmt.Sprintf(`{ "execute": "query-block-jobs" }`)
	cmd.Command = fmt.Sprintf(`vm qmp %s '%s'`, vmName, qmp)

	for {
		res, err := mmcli.SingleResponse(mmcli.Run(cmd))
		if err != nil {
			return fmt.Errorf("querying for block device jobs for VM %s: %w", vmName, err)
		}

		var v map[string][]mm.BlockDeviceJobs
		json.Unmarshal([]byte(res), &v)

		if len(v["return"]) == 0 {
			break
		}

		for _, job := range v["return"] {
			if job.Device != device {
				continue
			}

			if cb != nil {
				// Cut progress in half since drive backup is 1 of 2 steps.
				progress := float64(job.Offset) / float64(job.Length)
				progress = progress * 0.5

				cb(fmt.Sprintf("%f", progress))
			}
		}

		time.Sleep(1 * time.Second)
	}

	// ***** END: SNAPSHOT VM *****

	// ***** BEGIN: MIGRATE VM *****

	cmd.Command = fmt.Sprintf("vm migrate %s %s.SNAP", vmName, out)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("starting memory snapshot for VM %s: %w", vmName, err)
	}

	cmd.Command = "vm migrate"
	cmd.Columns = []string{"name", "status", "complete (%)"}
	cmd.Filters = []string{"name=" + vmName}

	for {
		status := mmcli.RunTabular(cmd)[0]

		if cb != nil {
			if status["status"] == "completed" {
				cb("completed")
			} else {
				// Cut progress in half and add 0.5 to it since migrate is 2 of 2 steps.
				progress, _ := strconv.ParseFloat(status["complete (%)"], 64)
				progress = 0.5 + (progress * 0.5)

				cb(fmt.Sprintf("%f", progress))
			}
		}

		if status["status"] == "completed" {
			break
		}

		time.Sleep(1 * time.Second)
	}

	// ***** END: MIGRATE VM *****

	cmd.Command = fmt.Sprintf("vm start %s", vmName)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("resuming VM %s after snapshot: %w", vmName, err)
	}

	var (
		dst       = fmt.Sprintf("%s/images/%s/files", common.PhenixBase, expName)
		cmdPrefix string
	)

	if !mm.IsHeadnode(host) {
		cmdPrefix = "mesh send " + host
	}

	cmd = mmcli.NewCommand()
	cmd.Command = fmt.Sprintf("%s shell mkdir -p %s", cmdPrefix, dst)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("ensuring experiment files directory exists: %w", err)
	}

	final := strings.TrimPrefix(out, expName+"_")

	cmd.Command = fmt.Sprintf("%s shell mv %s/images/%s.SNAP %s/%s.SNAP", cmdPrefix, common.PhenixBase, out, dst, final)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("moving memory snapshot to experiment files directory: %w", err)
	}

	cmd.Command = fmt.Sprintf("%s shell mv %s/images/%s.qc2 %s/%s.qc2", cmdPrefix, common.PhenixBase, out, dst, final)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("moving disk snapshot to experiment files directory: %w", err)
	}

	return nil

}

func Restore(expName, vmName, snap string) error {
	snap = strings.TrimSuffix(snap, filepath.Ext(snap))

	snapshots, err := Snapshots(expName, vmName)
	if err != nil {
		return fmt.Errorf("getting list of snapshots for VM: %w", err)
	}

	var found bool

	for _, ss := range snapshots {
		if snap == ss {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("snapshot does not exist on cluster")
	}

	snap = fmt.Sprintf("%s/files/%s", expName, snap)

	cmd := mmcli.NewNamespacedCommand(expName)
	cmd.Command = fmt.Sprintf("vm config clone %s", vmName)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("cloning config for VM %s: %w", vmName, err)
	}

	cmd.Command = fmt.Sprintf("vm config migrate %s.SNAP", snap)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("configuring migrate file for VM %s: %w", vmName, err)
	}

	cmd.Command = fmt.Sprintf("vm config disk %s.qc2,writeback", snap)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("configuring disk file for VM %s: %w", vmName, err)
	}

	cmd.Command = fmt.Sprintf("vm kill %s", vmName)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("killing VM %s: %w", vmName, err)
	}

	// TODO: explicitly flush killed VM by name once we start using that version
	// of minimega.
	cmd.Command = "vm flush"

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("flushing VMs: %w", err)
	}

	cmd.Command = fmt.Sprintf("vm launch kvm %s", vmName)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("relaunching VM %s: %w", vmName, err)
	}

	cmd.Command = "vm launch"

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("scheduling VM %s: %w", vmName, err)
	}

	cmd.Command = fmt.Sprintf("vm start %s", vmName)

	if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
		return fmt.Errorf("starting VM %s: %w", vmName, err)
	}

	return nil

}

func CommitToDisk(expName, vmName, out string, cb func(float64)) (string, error) {
	// Determine name of new disk image, if not provided.

	if out == "" {
		var err error

		out, err = GetNewDiskName(expName, vmName)
		if err != nil {
			return "", fmt.Errorf("getting new disk name for VM %s in experiment %s: %w", vmName, expName, err)
		}
	}

	base, err := getBaseImage(expName, vmName)
	if err != nil {
		return "", fmt.Errorf("getting base image for VM %s in experiment %s: %w", vmName, expName, err)
	}

	// Get compute node VM is running on.

	cmd := mmcli.NewNamespacedCommand(expName)
	cmd.Command = "vm info"
	cmd.Columns = []string{"host", "name", "id", "state"}
	cmd.Filters = []string{"name=" + vmName}

	status := mmcli.RunTabular(cmd)

	if len(status) == 0 {
		return "", fmt.Errorf("VM not found")
	}

	var (
		// Get current disk snapshot on the compute node (based on VM ID).
		snap = "/tmp/minimega/" + status[0]["id"] + "/disk-0.qcow2"
		node = status[0]["host"]
	)
	if(!filepath.IsAbs(base)) {
		base = common.PhenixBase + "/images/" + base
	}
	if(!filepath.IsAbs(out)) {
		out = common.PhenixBase + "/images/" + out
	}
	wait, ctx := errgroup.WithContext(context.Background())

	// Make copy of base image locally on headnode. Using a context here will help
	// cancel the potentially long running copy of a large base image if the other
	// Goroutine below fails.

	wait.Go(func() error {
		copier := newCopier()
		s := copier.subscribe()

		go func() {
			for p := range s {
				// If the callback is set, intercept it to reflect the copy stage as the
				// initial 80% of the effort.
				if cb != nil {
					cb(p * 0.8)
				}
			}
		}()

		if err := copier.copy(ctx, base, out); err != nil {
			os.Remove(out) // cleanup
			return fmt.Errorf("making copy of backing image: %w", err)
		}

		return nil
	})

	// VM can't be running or we won't be able to copy snapshot remotely.
	if status[0]["state"] != "QUIT" {
		cmd = mmcli.NewNamespacedCommand(expName)
		cmd.Command = "vm kill " + vmName

		if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
			return "", fmt.Errorf("stopping VM: %w", err)
		}
	}

	// Copy minimega snapshot disk on remote machine to a location (still on
	// remote machine) that can be seen by minimega files. Then use minimega `file
	// get` to copy it to the headnode.

	wait.Go(func() error {
		var cmdPrefix string

		if !mm.IsHeadnode(node) {
			cmdPrefix = "mesh send " + node
		}

		tmp := fmt.Sprintf("%s/images/%s/tmp", common.PhenixBase, expName)

		cmd := mmcli.NewCommand()
		cmd.Command = fmt.Sprintf("%s shell mkdir -p %s", cmdPrefix, tmp)

		if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
			return fmt.Errorf("ensuring experiment tmp directory exists: %w", err)
		}

		tmp = fmt.Sprintf("%s/images/%s/tmp/%s.qc2", common.PhenixBase, expName, vmName)
		cmd.Command = fmt.Sprintf("%s shell cp %s %s", cmdPrefix, snap, tmp)

		if err := mmcli.ErrorResponse(mmcli.Run(cmd)); err != nil {
			return fmt.Errorf("copying snapshot remotely: %w", err)
		}

		headnode, _ := os.Hostname()
		tmp = fmt.Sprintf("%s/tmp/%s.qc2", expName, vmName)

		if err := file.CopyFile(tmp, headnode, nil); err != nil {
			return fmt.Errorf("pulling snapshot to headnode: %w", err)
		}

		return nil
	})

	if err := wait.Wait(); err != nil {
		return "", fmt.Errorf("preparing images for rebase/commit: %w", err)
	}

	snap = fmt.Sprintf("%s/images/%s/tmp/%s.qc2", common.PhenixBase, expName, vmName)

	shell := exec.Command("qemu-img", "rebase", "-b", out, snap)

	res, err := shell.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rebasing snapshot (%s): %w", string(res), err)
	}

	done := make(chan struct{})
	defer close(done)

	if cb != nil {
		stat, _ := os.Stat(out)
		targetSize := float64(stat.Size())

		stat, _ = os.Stat(snap)
		targetSize += float64(stat.Size())

		go func() {
			for {
				select {
				case <-done:
					return
				default:
					// We sleep at the beginning instead of the end to ensure the command
					// we shell out to below has time to run before we try to stat the
					// destination file.
					time.Sleep(2 * time.Second)

					stat, err := os.Stat(out)
					if err != nil {
						continue
					}

					p := float64(stat.Size()) / targetSize

					cb(0.8 + (p * 0.2))
				}
			}
		}()
	}

	shell = exec.Command("qemu-img", "commit", snap)

	res, err = shell.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("committing snapshot (%s): %w", string(res), err)
	}

	out, _ = filepath.Rel(common.PhenixBase+"/images/", out)

	if err := file.SyncFile(out, nil); err != nil {
		return "", fmt.Errorf("syncing new backing image across cluster: %w", err)
	}

	return out, nil

}
