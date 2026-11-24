// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// fcSnapshotType is the vm_type of a Firecracker snapshot in <prefix>.json.
const fcSnapshotType = "firecracker"

// Jail paths of the snapshot files that a restore loads.
const (
	fcRestoreState = "/snapshot.state"
	fcRestoreMem   = "/snapshot.mem"
)

// Tests replace these.
var (
	// fcSaveSnapshot creates the full snapshot of a vm save through c.
	fcSaveSnapshot = func(ctx context.Context, c *fcClient, statePath, memPath string) error {
		return c.CreateSnapshotSync(ctx, statePath, memPath)
	}

	// fcSnapshotHostFn returns the facts about the host and the Firecracker
	// binary fcBin that a snapshot records and a restore checks.
	fcSnapshotHostFn = fcSnapshotHostDefault

	// fcSaveRename renames the files of a vm save into place.
	fcSaveRename = os.Rename

	// fcRunSnapshotVersion runs `<path> --snapshot-version` and returns its
	// output.
	fcRunSnapshotVersion = func(path string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, path, "--snapshot-version").CombinedOutput()
		return string(out), err
	}
)

// fcSaves holds the latest vm save of each Firecracker VM, by VM ID.
var fcSaves = &fcExportRegistry{m: map[int]*fcExport{}}

// fcPrefixes holds the snapshot path prefixes that a vm save writes or a
// restore reads.
var fcPrefixes = &fcPrefixTable{saving: map[string]bool{}, locks: map[string]*fcPrefixLock{}}

// fcPrefixTable makes the saves to one prefix exclusive. A save holds the
// write lock of its prefix while it renames its files into place, and a
// restore holds the read lock from the sidecar read until the jail links the
// files.
type fcPrefixTable struct {
	mu     sync.Mutex
	saving map[string]bool
	locks  map[string]*fcPrefixLock
}

type fcPrefixLock struct {
	sync.RWMutex
	refs int
}

// fcPrefixKey returns the key of the snapshot path prefix p: the real path of
// its directory and the base name.
func fcPrefixKey(p string) string {
	p = filepath.Clean(p)
	return filepath.Join(fcResolve(filepath.Dir(p)), filepath.Base(p))
}

// beginSave records a save to prefix p. It returns an error when another save
// to p is running.
func (t *fcPrefixTable) beginSave(p string) error {
	key := fcPrefixKey(p)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.saving[key] {
		return fmt.Errorf("another save to %v is running", p)
	}
	t.saving[key] = true

	return nil
}

// endSave removes the record of beginSave.
func (t *fcPrefixTable) endSave(p string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.saving, fcPrefixKey(p))
}

// lock takes the write or read lock of prefix p, and returns the function that
// releases it. The function can run more than one time.
func (t *fcPrefixTable) lock(p string, write bool) func() {
	key := fcPrefixKey(p)

	t.mu.Lock()
	l := t.locks[key]
	if l == nil {
		l = &fcPrefixLock{}
		t.locks[key] = l
	}
	l.refs++
	t.mu.Unlock()

	if write {
		l.Lock()
	} else {
		l.RLock()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if write {
				l.Unlock()
			} else {
				l.RUnlock()
			}

			t.mu.Lock()
			if l.refs--; l.refs == 0 {
				delete(t.locks, key)
			}
			t.mu.Unlock()
		})
	}
}

// fcSnapshotDevices are the device counts of a snapshot.
type fcSnapshotDevices struct {
	Disks   int  `json:"disks"`
	RODisks int  `json:"ro_disks"`
	Scratch int  `json:"scratch"`
	NICs    int  `json:"nics"`
	Vsock   int  `json:"vsock"`
	Entropy int  `json:"entropy"`
	PCI     bool `json:"pci"`
}

func (d fcSnapshotDevices) String() string {
	return fmt.Sprintf("%v (%d ro), %d scratch, %v, %d vsock, %d entropy, pci %v",
		fcPlural(d.Disks, "disk"), d.RODisks, d.Scratch, fcPlural(d.NICs, "NIC"), d.Vsock, d.Entropy, d.PCI)
}

// fcSnapshotDrive is one drive of a snapshot. Source is the image of a
// read-only drive, which vm save does not copy. File is the name of the file
// that vm save wrote for a writable drive.
type fcSnapshotDrive struct {
	ID       string `json:"id"`
	JailPath string `json:"jail_path"`
	ReadOnly bool   `json:"read_only"`
	Source   string `json:"source,omitempty"`
	File     string `json:"file,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// fcSnapshotInfo is the content of <prefix>.json, which vm save writes last.
type fcSnapshotInfo struct {
	VMType             string            `json:"vm_type"`
	FirecrackerVersion string            `json:"firecracker_version"`
	SnapshotVersion    string            `json:"snapshot_version"`
	Arch               string            `json:"arch"`
	HostCPUModel       string            `json:"host_cpu_model"`
	HostKernel         string            `json:"host_kernel"`
	VCPUs              uint64            `json:"vcpus"`
	MemoryMiB          uint64            `json:"memory_mib"`
	Devices            fcSnapshotDevices `json:"devices"`
	NICMACs            []string          `json:"nic_macs"`
	Drives             []fcSnapshotDrive `json:"drives"`
	MemoryRegions      []memRegion       `json:"memory_regions,omitempty"`
	Created            time.Time         `json:"created"`
}

// fcSnapshotHost holds the facts about a host and its Firecracker binary that
// decide whether a snapshot loads.
type fcSnapshotHost struct {
	Arch     string
	CPUModel string
	Kernel   string

	Version    fcVersion
	VersionErr error

	// SnapshotVersion is the snapshot format version that the binary writes
	// and loads.
	SnapshotVersion    fcVersion
	SnapshotVersionErr error
}

// fcRestore is a snapshot that a launch loads.
type fcRestore struct {
	prefix  string
	state   string
	mem     string
	scratch string   // the saved scratch disk, or empty
	macs    []string // the NIC MACs of the snapshot
	unlock  func()   // releases the read lock of the prefix
}

// release releases the read lock of the prefix of r.
func (r *fcRestore) release() {
	if r.unlock != nil {
		r.unlock()
	}
}

// fcSave is one vm save after its snapshot.
type fcSave struct {
	job   *fcExport
	info  fcSnapshotInfo
	files []fcSaveFile // in rename order, without <prefix>.json
}

// fcSaveFile is one file of a vm save: src moves or copies to dst.
type fcSaveFile struct {
	src    string
	dst    string
	move   bool
	frozen *fcFrozenDisk
}

var (
	fcHostIDOnce sync.Once
	fcHostCPU    string
	fcHostKernel string

	fcSnapshotFormats sync.Map // resolved path -> *fcSnapshotFormatEntry
)

// fcSnapshotFormatEntry caches the snapshot format of one binary. It keeps
// only a success, and only for the same file size and mtime.
type fcSnapshotFormatEntry struct {
	mu    sync.Mutex
	ok    bool
	stamp fcFileStamp
	v     fcVersion
}

func fcSnapshotHostDefault(fcBin string) fcSnapshotHost {
	fcHostIDOnce.Do(func() {
		data, _ := os.ReadFile("/proc/cpuinfo")
		fcHostCPU = fcCPUModel(string(data))

		var u unix.Utsname
		if err := unix.Uname(&u); err == nil {
			fcHostKernel = unix.ByteSliceToString(u.Release[:])
		}
	})

	h := fcSnapshotHost{Arch: fcArchName(fcMemDumpArch), CPUModel: fcHostCPU, Kernel: fcHostKernel}

	b := fcBinary(fcBin)
	h.Version, h.VersionErr = b.Version, b.Err
	h.SnapshotVersion, h.SnapshotVersionErr = fcSnapshotFormat(fcBin)

	return h
}

// fcSnapshotFormat returns the snapshot format version of the Firecracker
// binary fcBin from `fcBin --snapshot-version`. It runs the binary again
// after a failure or when the file changes.
func fcSnapshotFormat(fcBin string) (fcVersion, error) {
	p := fcResolve(fcBin)

	v, _ := fcSnapshotFormats.LoadOrStore(p, &fcSnapshotFormatEntry{})
	e := v.(*fcSnapshotFormatEntry)
	e.mu.Lock()
	defer e.mu.Unlock()

	stamp, ok := fcStampOf(p)
	if e.ok && ok && e.stamp.equal(stamp) {
		return e.v, nil
	}
	e.ok = false

	out, err := fcRunSnapshotVersion(p)
	if err != nil {
		return fcVersion{}, fmt.Errorf("%v --snapshot-version failed: %v", fcBin, err)
	}
	ver, err := parseFCVersion(out)
	if err != nil {
		return fcVersion{}, fmt.Errorf("%v --snapshot-version: %v", fcBin, err)
	}

	e.ok, e.stamp, e.v = ok, stamp, ver

	return ver, nil
}

// fcCPUModel returns the CPU model in the content of /proc/cpuinfo: the model
// name on x86_64, else the CPU implementer and part.
func fcCPUModel(cpuinfo string) string {
	var impl, part string

	for _, line := range strings.Split(cpuinfo, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		switch v = strings.TrimSpace(v); strings.TrimSpace(k) {
		case "model name":
			return v
		case "CPU implementer":
			if impl == "" {
				impl = v
			}
		case "CPU part":
			if part == "" {
				part = v
			}
		}
	}

	if impl == "" && part == "" {
		return ""
	}

	return fmt.Sprintf("implementer %v part %v", impl, part)
}

// fcArchName returns the uname name of a GOARCH value.
func fcArchName(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}

	return goarch
}

// fcSnapshotPrefix returns the path prefix of the snapshot p. A relative
// prefix is relative to the files directory (-filepath), as for vm save.
func fcSnapshotPrefix(p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(*f_iomBase, p)
	}

	return filepath.Clean(p)
}

// fcSaveSuffix returns the suffix of the file that vm save writes for disk i:
// .hdd for disk 0, .hdd<i> for the other disks, and .scratch for the scratch
// disk.
func fcSaveSuffix(i int, scratch bool) string {
	switch {
	case scratch:
		return ".scratch"
	case i == 0:
		return ".hdd"
	}

	return fmt.Sprintf(".hdd%d", i)
}

// fcSnapshotDevicesOf returns the device counts of a VM with configuration
// cfg.
func fcSnapshotDevicesOf(cfg VMConfig) fcSnapshotDevices {
	d := fcSnapshotDevices{
		Disks: len(cfg.Disks),
		NICs:  len(cfg.Networks),
		PCI:   cfg.FirecrackerPCI,
	}

	for _, dc := range cfg.Disks {
		if dc.ReadOnly {
			d.RODisks++
		}
	}
	if size, _ := fcParseSize(cfg.FirecrackerScratch); size > 0 {
		d.Scratch = 1
	}
	if cfg.Backchannel {
		d.Vsock = 1
	}
	if cfg.FirecrackerEntropy {
		d.Entropy = 1
	}

	return d
}

// fcSnapshotDrives returns the drives of a VM with configuration cfg, in
// order, without the file names.
func fcSnapshotDrives(cfg VMConfig) []fcSnapshotDrive {
	var res []fcSnapshotDrive

	for i, dc := range cfg.Disks {
		id := fcDriveID(i, fcDisk{})
		res = append(res, fcSnapshotDrive{ID: id, JailPath: "/" + id, ReadOnly: dc.ReadOnly})
	}

	if size, _ := fcParseSize(cfg.FirecrackerScratch); size > 0 {
		id := fcDriveID(len(cfg.Disks), fcDisk{Scratch: true})
		res = append(res, fcSnapshotDrive{ID: id, JailPath: "/" + id, Size: size})
	}

	return res
}

func fcSameDrives(a, b []fcSnapshotDrive) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].ID != b[i].ID || a[i].JailPath != b[i].JailPath || a[i].ReadOnly != b[i].ReadOnly || a[i].Size != b[i].Size {
			return false
		}
	}

	return true
}

func fcDriveList(drives []fcSnapshotDrive) string {
	var res []string

	for _, d := range drives {
		s := d.JailPath
		if d.ReadOnly {
			s += " (ro)"
		}
		if d.Size > 0 {
			s += fmt.Sprintf(" (%d bytes)", d.Size)
		}
		res = append(res, s)
	}

	return "[" + strings.Join(res, ", ") + "]"
}

// snapshotInfo returns the sidecar of a save of the VM to dst on host.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) snapshotInfo(dst string, host fcSnapshotHost) fcSnapshotInfo {
	cfg := VMConfig{BaseConfig: vm.BaseConfig, KVMConfig: vm.KVMConfig, FirecrackerConfig: vm.FirecrackerConfig}

	info := fcSnapshotInfo{
		VMType:             fcSnapshotType,
		FirecrackerVersion: host.Version.String(),
		Arch:               host.Arch,
		HostCPUModel:       host.CPUModel,
		HostKernel:         host.Kernel,
		VCPUs:              vm.VCPUs,
		MemoryMiB:          vm.Memory,
		Devices:            fcSnapshotDevicesOf(cfg),
		NICMACs:            []string{},
		Drives:             []fcSnapshotDrive{},
	}

	if host.SnapshotVersionErr == nil {
		info.SnapshotVersion = host.SnapshotVersion.String()
	} else {
		log.Warn("vm %v: the snapshot does not record its format version, so a restore needs firecracker %v: %v", vm.ID, host.Version, host.SnapshotVersionErr)
	}

	for _, nic := range vm.Networks {
		info.NICMACs = append(info.NICMACs, nic.MAC)
	}

	size, _ := fcParseSize(vm.FirecrackerScratch)
	base := filepath.Base(dst)

	for i, d := range vm.disks {
		id := fcDriveID(i, d)
		drive := fcSnapshotDrive{ID: id, JailPath: "/" + id, ReadOnly: d.ReadOnly}

		switch {
		case d.ReadOnly && i < len(vm.Disks):
			drive.Source = fcResolve(vm.Disks[i].Path)
		case d.Scratch:
			drive.File = base + fcSaveSuffix(i, true)
			drive.Size = size
		default:
			drive.File = base + fcSaveSuffix(i, false)
		}

		info.Drives = append(info.Drives, drive)
	}

	if fcMemDumpArch == "amd64" {
		if regions, err := fcMemoryRegions(vm.Memory); err == nil {
			info.MemoryRegions = regions
		}
	}

	return info
}

// Save starts a vm save of a running or paused VM to the path prefix dst. It
// pauses a running VM, has Firecracker write a full snapshot, and takes frozen
// copies of the writable disks. The VM stays paused, as a KVM VM does after vm
// save, and vm start resumes it. Save returns before minimega writes
// <dst>.state, <dst>.mem, the disk files, and <dst>.json, which vm save with
// no arguments reports.
func (vm *FirecrackerVM) Save(dst string) error {
	// HasClient takes a ron lock, so it runs without vm.lock.
	vm.lock.Lock()
	cc, uuid, fcBin := vm.cc, vm.UUID, vm.fcBin
	err := vm.checkSaveState()
	vm.lock.Unlock()
	if err != nil {
		return err
	}

	if s, ok := cc.(interface{ HasClient(string) bool }); ok && s.HasClient(uuid) {
		log.Warn("vm %v: the save resets the vsock connections of the guest, so cc commands, file transfers, and tunnels in progress fail", vm.ID)
	}

	host := fcSnapshotHostFn(fcBin)

	vm.lock.Lock()
	s, err := vm.startSave(dst, host)
	vm.lock.Unlock()
	if err != nil {
		return err
	}

	go fcFinishSave(s)

	return nil
}

// checkSaveState returns an error when the VM cannot be saved.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) checkSaveState() error {
	switch vm.State {
	case VM_RUNNING, VM_PAUSED:
	default:
		return fmt.Errorf("vm %v is %v: vm save needs a vm in running or paused", vm.Name, vm.State)
	}
	if vm.client == nil {
		return vmNotRunning(vm.Name)
	}
	if vm.clone != nil {
		return fmt.Errorf("vm %v is a clone without its identity (clone_identity %q): vm save needs clone_identity ok, or a relaunch with vm kill and vm start", vm.Name, vm.Tags[fcCloneTag])
	}

	return nil
}

// startSave records a save of the VM in fcSaves and takes the snapshot.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) startSave(dst string, host fcSnapshotHost) (*fcSave, error) {
	if err := vm.checkSaveState(); err != nil {
		return nil, err
	}
	if host.VersionErr != nil {
		return nil, fmt.Errorf("unable to get firecracker version: %v", host.VersionErr)
	}

	job := &fcExport{
		ID:        vm.ID,
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Dst:       dst,
		Format:    "snapshot",
		status:    fcExportActive,
	}

	prev, err := fcSaves.start(job)
	if err != nil {
		return nil, fmt.Errorf("vm %v already has a vm save in progress", vm.Name)
	}
	if err := fcPrefixes.beginSave(dst); err != nil {
		fcSaves.restore(job, prev)
		return nil, err
	}

	s, err := vm.snapshotForSave(job, vm.snapshotInfo(dst, host))
	if err != nil {
		fcPrefixes.endSave(dst)
		fcSaves.restore(job, prev)
		return nil, err
	}

	return s, nil
}

// snapshotForSave pauses a running VM, writes a full snapshot to /out in the
// jail, moves the snapshot files into the instance directory, and takes a
// frozen copy of each writable disk. The VM stays paused. When a step fails,
// it resumes a VM that was running.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) snapshotForSave(job *fcExport, info fcSnapshotInfo) (*fcSave, error) {
	dir := fcInstanceDir(vm.ID)

	vm.exportSeq++
	n := vm.exportSeq

	statePath, memPath := "/out/save.state", "/out/save.mem"
	hostState, hostMem := vm.jail.HostPath(statePath), vm.jail.HostPath(memPath)

	s := &fcSave{job: job, info: info}
	running := vm.State == VM_RUNNING

	fail := func(err error) (*fcSave, error) {
		os.Remove(hostState)
		os.Remove(hostMem)
		if err2 := s.release(); err2 != nil {
			log.Error("vm %v: remove save files: %v", vm.ID, err2)
		}

		if running {
			if err2 := vm.client.PatchVMState("Resumed"); err2 != nil {
				return nil, vm.setErrorf("unable to resume: %v", err2)
			}
		}

		return nil, err
	}

	var start time.Time
	if running {
		if err := vm.client.PatchVMState("Paused"); err != nil {
			return nil, fmt.Errorf("unable to pause: %v", err)
		}
		start = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), fcMemDumpTimeout(vm.Memory))
	err := fcSaveSnapshot(ctx, vm.client, statePath, memPath)
	cancel()
	if err != nil {
		return fail(fmt.Errorf("unable to create snapshot: %v", err))
	}

	// A relaunch removes the jail, so the files move to the instance
	// directory.
	for _, f := range []struct{ src, ext string }{{hostState, ".state"}, {hostMem, ".mem"}} {
		p := filepath.Join(dir, fmt.Sprintf("save-%d%s", n, f.ext))
		if err := fcMemDumpMove(f.src, p, nil); err != nil {
			return fail(fmt.Errorf("unable to move snapshot file: %v", err))
		}
		s.files = append(s.files, fcSaveFile{src: p, dst: job.Dst + f.ext, move: true})
	}

	for i, d := range vm.disks {
		if d.ReadOnly {
			continue
		}

		vm.exportSeq++
		f, err := fcFreezeDisk(d, dir, vm.ID, vm.exportSeq)
		if err != nil {
			return fail(fmt.Errorf("unable to copy disk %v: %v", i, err))
		}

		s.files = append(s.files, fcSaveFile{
			src:    f.Path,
			dst:    job.Dst + fcSaveSuffix(i, d.Scratch),
			move:   f.dmName == "",
			frozen: &f,
		})
	}

	if running {
		vm.setState(VM_PAUSED)
		log.Info("vm %v: vm save paused the VM after %v; vm start resumes it", vm.ID, time.Since(start))
	}

	return s, nil
}

// release removes the files and host objects that s holds.
func (s *fcSave) release() error {
	var errs []error

	for _, f := range s.files {
		if f.frozen != nil {
			if err := f.frozen.release(); err != nil {
				errs = append(errs, err)
			}
		} else if err := os.Remove(f.src); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// fcFinishSave writes the files of s, then releases the files that s holds.
func fcFinishSave(s *fcSave) {
	err := fcWriteSave(s)

	if err2 := s.release(); err2 != nil {
		log.Error("vm %v: remove save files: %v", s.job.ID, err2)
	}
	fcPrefixes.endSave(s.job.Dst)

	if err != nil {
		log.Error("vm %v: vm save to %v failed: %v", s.job.ID, s.job.Dst, err)
		s.job.finish(fcExportFailed)
		return
	}

	log.Info("vm %v: saved to %v", s.job.ID, s.job.Dst)
	s.job.finish(fcExportCompleted)
}

// fcWriteSave writes each file of s and the sidecar to a temporary name next
// to its destination, syncs it, and renames it into place. It renames the
// sidecar last, so a reader that finds <dst>.json finds a complete snapshot.
// A new save to the same prefix replaces the files, and a restored VM keeps
// the old memory file that it maps. The renames wait for the restores that
// read the prefix.
func fcWriteSave(s *fcSave) error {
	dst := s.job.Dst
	suffix := ".tmp-" + fcRandomSuffix()

	var tmps []string
	fail := func(err error) error {
		for _, p := range tmps {
			os.Remove(p)
		}
		return err
	}

	if err := fcWriteSaveTemps(s, suffix, &tmps); err != nil {
		return fail(err)
	}

	unlock := fcPrefixes.lock(dst, true)
	defer unlock()

	sidecar := dst + ".json"
	sidecarTmp := sidecar + suffix

	// Without the old sidecar, the files of two saves never look like one
	// snapshot.
	if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	for _, f := range s.files {
		if err := fcSaveRename(f.dst+suffix, f.dst); err != nil {
			return fail(err)
		}
	}
	if err := fcSaveRename(sidecarTmp, sidecar); err != nil {
		return fail(err)
	}

	return fcSyncPath(filepath.Dir(dst))
}

// fcWriteSaveTemps writes each file of s and the sidecar to its destination
// with suffix, syncs it, and appends its name to tmps.
func fcWriteSaveTemps(s *fcSave, suffix string, tmps *[]string) error {
	fcLaunchSem.Acquire()
	defer fcLaunchSem.Release()

	sizes := make([]int64, len(s.files))
	var total int64
	for i, f := range s.files {
		size, err := fcFileSize(f.src)
		if err != nil {
			return err
		}
		sizes[i] = size
		total += size
	}

	var done int64
	for i, f := range s.files {
		tmp := f.dst + suffix
		*tmps = append(*tmps, tmp)

		progress := func(p float64) {
			if total > 0 {
				s.job.progress((float64(done) + p*float64(sizes[i])/100) * 100 / float64(total))
			}
		}

		var err error
		if f.move {
			err = fcMemDumpMove(f.src, tmp, progress)
		} else {
			err = fcSparseCopyProgress(f.src, tmp, progress)
		}

		// The VMM wrote the snapshot files as the VM uid.
		if err == nil {
			err = fcChown(tmp, os.Geteuid(), os.Getegid())
		}
		if err == nil {
			err = os.Chmod(tmp, 0600)
		}
		if err == nil {
			err = fcSyncPath(tmp)
		}
		if err != nil {
			return fmt.Errorf("write %v: %v", f.dst, err)
		}

		done += sizes[i]
	}

	s.info.Created = time.Now().UTC()
	data, err := json.MarshalIndent(s.info, "", "    ")
	if err != nil {
		return err
	}

	sidecar := s.job.Dst + ".json"
	sidecarTmp := sidecar + suffix
	*tmps = append(*tmps, sidecarTmp)
	if err := fcWriteSynced(sidecarTmp, append(data, '\n')); err != nil {
		return fmt.Errorf("write %v: %v", sidecar, err)
	}

	return nil
}

// fcSavePoll is the interval at which fcWaitSave checks a vm save. Tests
// replace it.
var fcSavePoll = 100 * time.Millisecond

// fcWaitSave waits until the vm save of the VM with ID id ends. It returns an
// error when the save failed or when the VM has no save.
func fcWaitSave(id int) error {
	for {
		fcSaves.mu.Lock()
		e := fcSaves.m[id]
		fcSaves.mu.Unlock()

		if e == nil {
			return fmt.Errorf("vm %v has no vm save", id)
		}

		switch status, _ := e.get(); status {
		case fcExportCompleted:
			return nil
		case fcExportFailed:
			return fmt.Errorf("vm save of vm %v to %v failed, see the minimega log", e.Name, e.Dst)
		}

		time.Sleep(fcSavePoll)
	}
}

// fcFileSize returns the size of a file or a block device.
func fcFileSize(p string) (int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if fi.Mode().IsRegular() {
		return fi.Size(), nil
	}

	return f.Seek(0, io.SeekEnd)
}

// fcSyncPath syncs the file or directory p to disk.
func fcSyncPath(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// fcWriteSynced creates the file p with data and mode 0600, and syncs it.
func fcWriteSynced(p string, data []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// fcReadSnapshotInfo reads <prefix>.json.
func fcReadSnapshotInfo(prefix string) (fcSnapshotInfo, error) {
	var info fcSnapshotInfo

	data, err := os.ReadFile(prefix + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return info, fmt.Errorf("snapshot %v is not a Firecracker snapshot: %v.json does not exist", prefix, prefix)
	} else if err != nil {
		return info, fmt.Errorf("snapshot %v: %v", prefix, err)
	}

	if err := json.Unmarshal(data, &info); err != nil {
		return info, fmt.Errorf("snapshot %v: %v.json: %v", prefix, prefix, err)
	}

	return info, nil
}

// fcCheckSnapshotVersion returns an error when the Firecracker binary of host
// cannot load the snapshot format of info. The major versions must be the
// same, and the snapshot minor version must not be higher. When a format
// version is unknown, the Firecracker versions must be the same.
func fcCheckSnapshotVersion(info fcSnapshotInfo, host fcSnapshotHost) error {
	if host.VersionErr != nil {
		return fmt.Errorf("unable to get firecracker version: %v", host.VersionErr)
	}

	snap, snapErr := parseFCVersion(info.SnapshotVersion)
	if snapErr == nil && host.SnapshotVersionErr == nil {
		have := host.SnapshotVersion
		if snap.Major != have.Major || snap.Minor > have.Minor {
			return fmt.Errorf("the snapshot format is %v (firecracker %v), but firecracker %v loads format %v: the major versions must be the same, and the snapshot minor version must not be higher", snap, info.FirecrackerVersion, host.Version, have)
		}
		return nil
	}

	reason := host.SnapshotVersionErr
	if snapErr != nil {
		reason = errors.New("the snapshot does not record it")
	}

	made, err := parseFCVersion(info.FirecrackerVersion)
	if err != nil || made != host.Version {
		return fmt.Errorf("the snapshot format version is unknown (%v), so the restore needs firecracker %v, which made the snapshot, not %v", reason, info.FirecrackerVersion, host.Version)
	}

	return nil
}

// fcCheckSnapshot checks the sidecar info of the snapshot at prefix against
// cfg and host. It returns a warning for each difference that does not stop
// the restore.
func fcCheckSnapshot(prefix string, info fcSnapshotInfo, cfg VMConfig, host fcSnapshotHost) ([]string, error) {
	if info.VMType != fcSnapshotType {
		return nil, fmt.Errorf("snapshot %v is not a Firecracker snapshot: %v.json has vm_type %q", prefix, prefix, info.VMType)
	}

	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if cfg.KernelPath == "" {
		fail("firecracker-snapshot needs vm config kernel: after a guest reboot, a restored VM boots that kernel")
	}
	if len(cfg.Injects) > 0 {
		fail("vm config inject does not apply to a restore: clear vm config inject")
	}

	if info.Arch != host.Arch {
		fail("snapshot %v is for %v, but this host is %v", prefix, info.Arch, host.Arch)
	}
	if err := fcCheckSnapshotVersion(info, host); err != nil {
		fail("snapshot %v: %v", prefix, err)
	}
	if info.HostCPUModel != host.CPUModel || info.HostKernel != host.Kernel {
		fail("snapshot %v was made on a host with CPU %q and kernel %v, but this host has CPU %q and kernel %v: restore on the same kind of host", prefix, info.HostCPUModel, info.HostKernel, host.CPUModel, host.Kernel)
	}

	if info.VCPUs != cfg.VCPUs {
		fail("snapshot %v has %d vCPUs, but vm config vcpus is %d", prefix, info.VCPUs, cfg.VCPUs)
	}
	if info.MemoryMiB != cfg.Memory {
		fail("snapshot %v has %d MiB of memory, but vm config memory is %d", prefix, info.MemoryMiB, cfg.Memory)
	}

	if want := fcSnapshotDevicesOf(cfg); info.Devices != want {
		fail("snapshot %v has %v, but vm config gives %v", prefix, info.Devices, want)
	} else if want := fcSnapshotDrives(cfg); !fcSameDrives(info.Drives, want) {
		fail("snapshot %v has the drives %v, but vm config gives %v", prefix, fcDriveList(info.Drives), fcDriveList(want))
	} else if len(info.NICMACs) != info.Devices.NICs {
		fail("snapshot %v has %d NIC MACs for %v", prefix, len(info.NICMACs), fcPlural(info.Devices.NICs, "NIC"))
	} else if _, err := fcSnapshotMACs(info.NICMACs); err != nil {
		fail("snapshot %v: %v", prefix, err)
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	var warnings []string
	for i, dc := range cfg.Disks {
		if dc.ReadOnly {
			if src := info.Drives[i].Source; src != "" && fcResolve(dc.Path) != src {
				warnings = append(warnings, fmt.Sprintf("vm config disk %d is %v, but the snapshot used %v", i, dc.Path, src))
			}
			continue
		}

		if want := prefix + fcSaveSuffix(i, false); fcResolve(dc.Path) != fcResolve(want) {
			warnings = append(warnings, fmt.Sprintf("vm config disk %d is %v, not %v: a restore needs the disk that vm save wrote with the snapshot", i, dc.Path, want))
		}
	}

	return warnings, nil
}

// fcSnapshotMACs returns the NIC MACs of a sidecar in the lowercase form of
// net.HardwareAddr. It returns an error when one is not a 6-byte MAC.
func fcSnapshotMACs(macs []string) ([]string, error) {
	res := make([]string, 0, len(macs))

	for i, m := range macs {
		hw, err := net.ParseMAC(m)
		if err == nil && len(hw) != 6 {
			err = errors.New("not a 6-byte MAC")
		}
		if err != nil {
			return nil, fmt.Errorf("NIC %d has the MAC %q: %v", i, m, err)
		}
		res = append(res, hw.String())
	}

	return res, nil
}

// checkRestore reads and checks the snapshot of firecracker-snapshot for a
// launch with cfg and the Firecracker binary fcBin. It gives the NICs the MACs
// of the snapshot, because the restored guest keeps them. A clone keeps its
// configured MACs, and its identity sets them in the guest. The restore holds
// the read lock of the prefix until r.unlock runs.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) checkRestore(cfg VMConfig, fcBin string) (*fcRestore, error) {
	prefix := fcSnapshotPrefix(vm.FirecrackerSnapshotPath)

	unlock := fcPrefixes.lock(prefix, false)
	r, err := vm.readRestore(prefix, cfg, fcBin)
	if err != nil {
		unlock()
		return nil, err
	}
	r.unlock = unlock

	return r, nil
}

// readRestore reads and checks the snapshot at prefix for checkRestore.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) readRestore(prefix string, cfg VMConfig, fcBin string) (*fcRestore, error) {
	info, err := fcReadSnapshotInfo(prefix)
	if err != nil {
		return nil, err
	}

	warnings, err := fcCheckSnapshot(prefix, info, cfg, fcSnapshotHostFn(fcBin))
	for _, w := range warnings {
		log.Warn("vm %v: %v", vm.Name, w)
	}
	if err != nil {
		return nil, err
	}

	macs, err := fcSnapshotMACs(info.NICMACs)
	if err != nil {
		return nil, fmt.Errorf("snapshot %v: %v", prefix, err)
	}

	r := &fcRestore{prefix: prefix, state: prefix + ".state", mem: prefix + ".mem", macs: macs}
	if info.Devices.Scratch > 0 {
		r.scratch = prefix + fcSaveSuffix(0, true)
	}

	for _, p := range []string{r.state, r.mem, r.scratch} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("snapshot %v: %v", prefix, err)
		}
	}

	if r.scratch != "" {
		want, _ := fcParseSize(vm.FirecrackerScratch)
		if fi, err := os.Stat(r.scratch); err == nil && fi.Size() != want {
			return nil, fmt.Errorf("snapshot %v: %v is %d bytes, but firecracker-scratch is %d bytes", prefix, r.scratch, fi.Size(), want)
		}
	}

	if vm.FirecrackerClone {
		return r, nil
	}

	for i, mac := range macs {
		if !strings.EqualFold(vm.Networks[i].MAC, mac) {
			log.Info("vm %v: NIC %d gets MAC %v from snapshot %v in place of %v", vm.Name, i, mac, prefix, vm.Networks[i].MAC)
		}
		vm.Networks[i].MAC = mac
	}

	return r, nil
}

// fcSnapGroups holds the shared group of each snapshot that VMs restore.
var fcSnapGroups = newFCSnapGroups()

// fcSnapGroup is the group of the VMs that restore one snapshot. A gid of -1
// means that the files did not get the group, so each VM gets ACL entries.
type fcSnapGroup struct {
	gid   int
	files []string
	refs  int
}

// fcSnapGroupTable gives each snapshot that VMs restore a gid in
// fcSnapGIDRange: the highest gid that no other snapshot in use and no host
// account or group has. The gid is free again when no VM uses the snapshot.
type fcSnapGroupTable struct {
	mu       sync.Mutex
	m        map[string]*fcSnapGroup // real path of <prefix>.mem -> group
	reserved map[int]bool            // host IDs in fcSnapGIDRange
}

func newFCSnapGroups() *fcSnapGroupTable {
	return &fcSnapGroupTable{m: map[string]*fcSnapGroup{}, reserved: map[int]bool{}}
}

// fcSnapGIDRange returns the range [lo, hi) of the snapshot gids: the fcIDRange
// gids below the uid base, without gid 0.
func fcSnapGIDRange() (uint64, uint64) {
	hi := uint64(*f_firecrackerUIDBase)
	if hi <= fcIDRange {
		return 1, hi
	}

	return hi - fcIDRange, hi
}

// reserve keeps the host IDs ids out of the snapshot gids.
func (t *fcSnapGroupTable) reserve(ids []uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range ids {
		t.reserved[int(id)] = true
	}
}

// freeGID returns the highest free gid in fcSnapGIDRange.
//
// LOCK: Assumes t.mu is held.
func (t *fcSnapGroupTable) freeGID() (int, error) {
	used := map[int]bool{}
	for _, g := range t.m {
		used[g.gid] = true
	}

	lo, hi := fcSnapGIDRange()
	for gid := int(hi) - 1; gid >= int(lo); gid-- {
		if !used[gid] && !t.reserved[gid] {
			return gid, nil
		}
	}

	return 0, fmt.Errorf("no free gid for the snapshot group in %v-%v", lo, int(hi)-1)
}

// acquire adds a VM to the group of the snapshot files state and mem, and
// gives the files to root:<gid> with mode 0640. It returns the key of the group
// and its gid, or -1 when the files cannot get the group.
func (t *fcSnapGroupTable) acquire(state, mem string) (string, int, error) {
	key := fcResolve(mem)

	t.mu.Lock()
	defer t.mu.Unlock()

	g := t.m[key]
	if g == nil {
		gid, err := t.freeGID()
		if err != nil {
			return "", 0, err
		}

		g = &fcSnapGroup{gid: gid, files: []string{state, mem}}
		if err := fcShareSnapshot(g.files, gid); err != nil {
			fcUnshareSnapshot(g.files)
			log.Warn("unable to give %v and %v to group %v, so each restore of them adds an ACL entry for its uid, and ext4 holds only about 500 ACL entries on a file: %v", state, mem, gid, err)
			g.gid = -1
		}
		t.m[key] = g
	} else if g.gid >= 0 {
		// A vm save to the same prefix replaces the files.
		if err := fcShareSnapshot([]string{state, mem}, g.gid); err != nil {
			return "", 0, err
		}
	}

	g.refs++

	return key, g.gid, nil
}

// release removes a VM from the group key. After the last VM, it gives the
// files back to root:root with mode 0600, and the gid is free.
func (t *fcSnapGroupTable) release(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	g := t.m[key]
	if g == nil {
		return
	}
	if g.refs--; g.refs > 0 {
		return
	}
	delete(t.m, key)

	if g.gid >= 0 {
		if err := fcUnshareSnapshot(g.files); err != nil {
			log.Error("snapshot group %v: %v", g.gid, err)
		}
	}
}

// fcShareSnapshot gives the snapshot files to root:<gid> with mode 0640.
func fcShareSnapshot(files []string, gid int) error {
	for _, p := range files {
		if err := fcChown(p, os.Geteuid(), gid); err != nil {
			return err
		}
		if err := os.Chmod(p, 0640); err != nil {
			return err
		}
	}

	return nil
}

// fcUnshareSnapshot gives the snapshot files back to root:root with mode 0600.
// A missing file is not an error.
func fcUnshareSnapshot(files []string) error {
	var errs []error

	for _, p := range files {
		err := os.Chmod(p, 0600)
		if err == nil {
			err = fcChown(p, os.Geteuid(), os.Getegid())
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// grantSnapshotRead gives the VM read access to the state and memory files of
// r, and returns the gid of its VMM. The VMs that restore one snapshot share a
// group that can read its files. When the files cannot get the group, the VM
// uid gets an ACL entry on each file, and the gid is the VM uid. Flush
// releases the access.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) grantSnapshotRead(r *fcRestore) (int, error) {
	// A relaunch after a failed restore gets the access again.
	vm.revokeSnapshotRead()

	key, gid, err := fcSnapGroups.acquire(r.state, r.mem)
	if err != nil {
		return 0, fmt.Errorf("snapshot %v: %v", r.prefix, err)
	}
	vm.snapGroup = key

	if gid >= 0 {
		return gid, nil
	}

	uid := fcVMUID(vm.ID)
	for _, p := range []string{r.state, r.mem} {
		if out, err := fcRunCmd("setfacl", "-m", fmt.Sprintf("u:%d:r", uid), p); err != nil {
			return 0, fmt.Errorf("setfacl %v: %v: %v", p, strings.TrimSpace(out), err)
		}
		vm.snapACLs = append(vm.snapACLs, p)
	}

	return uid, nil
}

// revokeSnapshotRead removes the ACL entries of grantSnapshotRead and removes
// the VM from its snapshot group.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) revokeSnapshotRead() {
	uid := fcVMUID(vm.ID)

	for _, p := range vm.snapACLs {
		if out, err := fcRunCmd("setfacl", "-x", fmt.Sprintf("u:%d", uid), p); err != nil {
			log.Error("vm %v: setfacl -x %v: %v: %v", vm.ID, p, strings.TrimSpace(out), err)
		}
	}
	vm.snapACLs = nil

	if vm.snapGroup != "" {
		fcSnapGroups.release(vm.snapGroup)
		vm.snapGroup = ""
	}
}

// configureRestore loads the snapshot r into a new VMM, which leaves the VM
// paused. Only the logger and the serial output can precede the load. The
// load does not set clock_realtime: a failed load ends the VMM, and minimega
// cannot know the clock source of the guest.
func (vm *FirecrackerVM) configureRestore(c *fcClient, r *fcRestore) error {
	if err := c.PutLogger("/firecracker.log", "Info"); err != nil {
		return err
	}
	if err := c.PutSerial("/console.fifo"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), fcMemDumpTimeout(vm.Memory))
	defer cancel()

	if err := c.LoadSnapshot(ctx, fcRestoreState, fcRestoreMem, false); err != nil {
		return fmt.Errorf("unable to load snapshot %v: %v", r.prefix, err)
	}

	return nil
}

// fcRestoreScratch copies the scratch disk src that vm save wrote to the
// scratch disk of a VM in instanceDir. uid owns the copy.
func fcRestoreScratch(instanceDir, src string, uid int) (fcDisk, error) {
	p := filepath.Join(instanceDir, "scratch.raw")

	err := errors.New("no reflink")
	if fcReflinkOK() {
		err = fcCloneFile(src, p)
	}
	if err != nil {
		if err := fcFullCopy(src, p); err != nil {
			os.Remove(p)
			return fcDisk{}, fmt.Errorf("copy scratch disk %v: %v", src, err)
		}
	}

	if err := fcChown(p, uid, uid); err != nil {
		return fcDisk{}, fmt.Errorf("chown %v: %v", p, err)
	}

	return fcDisk{HostPath: p, Scratch: true, CacheType: fcCacheType("")}, nil
}

// writeFirecrackerRestore writes the namespace save commands that restore the
// Firecracker VM vm from the snapshot that vm save wrote to the prefix dst.
// With useIOM, the paths are relative to the files directory, and the disks
// use file: paths, which iomeshage fetches.
func writeFirecrackerRestore(w io.Writer, vm VM, dst string, useIOM bool) error {
	fmt.Fprintf(w, "clear vm config\n")

	if err := vm.WriteConfig(w); err != nil {
		return err
	}

	disks, err := vm.Info("disks")
	if err != nil {
		return err
	}

	var diskConfigs DiskConfigs
	if disks != "" {
		for i, spec := range strings.Split(disks, " ") {
			dc, err := ParseDiskConfig(spec, false)
			if err != nil {
				return err
			}

			if !dc.ReadOnly {
				p := dst + fcSaveSuffix(i, false)
				if useIOM {
					rel, _ := filepath.Rel(*f_iomBase, p)
					p = IOM_HELPER_MATCH + rel
				}
				dc.Path = p
			}

			diskConfigs = append(diskConfigs, *dc)
		}
	}

	snap := dst
	if useIOM {
		snap, _ = filepath.Rel(*f_iomBase, dst)
	}

	fmt.Fprintf(w, "clear vm config inject\n")
	fmt.Fprintf(w, "vm config firecracker-snapshot %v\n", snap)
	if len(diskConfigs) > 0 {
		fmt.Fprintf(w, "vm config disks %s\n", diskConfigs.String())
	}
	fmt.Fprintf(w, "vm launch %v %q\n\n", vm.GetType(), vm.GetName())

	return nil
}
