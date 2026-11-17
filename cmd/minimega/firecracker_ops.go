// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Status values of an export, the same values that vm save reports.
const (
	fcExportActive    = "active"
	fcExportCompleted = "completed"
	fcExportFailed    = "failed"
)

// fcExportHeader is the header of the vm export-disk progress table, the same
// header as the vm save progress table.
var fcExportHeader = []string{"id", "name", "status", "complete (%)"}

// fcConvertQcow2 writes the raw image src to dst as qcow2. Tests replace it.
var fcConvertQcow2 = fcConvertQcow2Default

// fcExport is one vm export-disk job.
type fcExport struct {
	ID        int
	Name      string
	Namespace string
	Dst       string
	Format    string

	mu       sync.Mutex
	status   string
	complete float64
}

func (e *fcExport) get() (string, float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.status, e.complete
}

func (e *fcExport) progress(p float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if p > 100 {
		p = 100
	}
	if p > e.complete {
		e.complete = p
	}
}

func (e *fcExport) finish(status string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.status = status
	if status == fcExportCompleted {
		e.complete = 100
	}
}

// fcExportRegistry holds the latest export of each Firecracker VM, by VM ID.
type fcExportRegistry struct {
	mu sync.Mutex
	m  map[int]*fcExport
}

var fcExports = &fcExportRegistry{m: map[int]*fcExport{}}

// start records e as the export of its VM. It returns the export that e
// replaces, and an error when that export is still active.
func (r *fcExportRegistry) start(e *fcExport) (*fcExport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev := r.m[e.ID]
	if prev != nil {
		if status, _ := prev.get(); status == fcExportActive {
			return nil, fmt.Errorf("vm %v already has an export-disk in progress", e.Name)
		}
	}
	r.m[e.ID] = e

	return prev, nil
}

// restore puts prev back in place of e.
func (r *fcExportRegistry) restore(e, prev *fcExport) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.m[e.ID] != e {
		return
	}
	if prev == nil {
		delete(r.m, e.ID)
	} else {
		r.m[e.ID] = prev
	}
}

// remove removes the export of VM id, unless it is still active. An active
// export keeps its row, so vm export-disk shows how it ends.
func (r *fcExportRegistry) remove(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e := r.m[id]; e != nil {
		if status, _ := e.get(); status == fcExportActive {
			return
		}
	}
	delete(r.m, id)
}

// table returns one progress row for each export of a VM in namespace.
func (r *fcExportRegistry) table(namespace string) [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var ids []int
	for id, e := range r.m {
		if e.Namespace == namespace {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)

	var res [][]string
	for _, id := range ids {
		e := r.m[id]
		status, complete := e.get()
		res = append(res, []string{
			strconv.Itoa(e.ID),
			e.Name,
			status,
			fmt.Sprintf("%.2f", complete),
		})
	}

	return res
}

// Shutdown sends Ctrl-Alt-Del to the guest and returns. With reboot=k, the
// guest reboot ends the VMM and the VM goes to QUIT.
func (vm *FirecrackerVM) Shutdown() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State != VM_RUNNING || vm.client == nil {
		return vmNotRunning(vm.Name)
	}

	log.Info("sending ctrl-alt-del to VM: %v", vm.ID)
	if err := vm.client.Action("SendCtrlAltDel"); err != nil {
		return fmt.Errorf("unable to shut down vm %v: %v", vm.Name, err)
	}

	return nil
}

// ResetDisk releases the disks of a VM in QUIT or ERROR and builds them again
// from the VM configuration, as the first launch did. The next start boots
// the new disks.
func (vm *FirecrackerVM) ResetDisk() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if !vm.Snapshot {
		return errors.New("vm reset-disk needs snapshot true")
	}
	if vm.State != VM_QUIT && vm.State != VM_ERROR {
		return fmt.Errorf("vm %v is %v: vm reset-disk needs a vm in quit or error", vm.Name, vm.State)
	}
	if !vm.dirReady {
		return fmt.Errorf("vm %v has no instance directory", vm.Name)
	}

	disk := fcGetStages().disk
	disk.acquire()
	defer disk.release()

	// A VMM can stay alive in ERROR, for example after a failed resume.
	vm.stopVMM()

	log.Info("resetting disks of VM: %v", vm.ID)

	// Keep a disk that fails to release, so that a later reset-disk or
	// flush tries again.
	var errs []error
	var stale []fcDisk
	for i, d := range append(vm.disks, vm.stale...) {
		if err := fcReleaseDiskFn(d); err != nil {
			errs = append(errs, fmt.Errorf("disk %v: %v", i, err))
			stale = append(stale, d)
		}
	}
	vm.disks, vm.stale = nil, stale
	vm.injected = false

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("unable to release disks: %v", err)
	}

	return vm.prepareDisks()
}

// fcFrozenDisk is a copy of a disk that does not change while it is exported.
type fcFrozenDisk struct {
	Path string // the file or device to read

	files   []string
	dmName  string
	cowLoop string
	baseKey string
}

// release removes the host objects and files of f.
func (f fcFrozenDisk) release() error {
	var errs []error

	run := func(args ...string) {
		if out, err := fcRunCmd(args...); err != nil {
			errs = append(errs, fmt.Errorf("%v: %v: %v", strings.Join(args, " "), strings.TrimSpace(out), err))
		}
	}

	if f.dmName != "" {
		run("dmsetup", "remove", f.dmName)
	}
	if f.cowLoop != "" {
		run("losetup", "-d", f.cowLoop)
	}
	if f.baseKey != "" {
		if err := fcCache.releaseBase(f.baseKey); err != nil {
			errs = append(errs, err)
		}
	}
	for _, p := range f.files {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ExportDisk starts an export of disk 0 to dst in format raw or qcow2. It
// takes a frozen copy of the disk, and pauses a running VM only for that
// copy. For a running VM the copy must be fast: a device-mapper snapshot or a
// reflink copy. It returns before the conversion to dst, which vm export-disk with
// no arguments reports.
func (vm *FirecrackerVM) ExportDisk(dst, format string) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch vm.State {
	case VM_RUNNING, VM_PAUSED, VM_QUIT:
	default:
		return fmt.Errorf("vm %v is %v: vm export-disk needs a vm in running, paused, or quit", vm.Name, vm.State)
	}
	if len(vm.disks) == 0 || vm.disks[0].Scratch {
		return fmt.Errorf("vm %v has no disk", vm.Name)
	}

	e := &fcExport{
		ID:        vm.ID,
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Dst:       dst,
		Format:    format,
		status:    fcExportActive,
	}

	prev, err := fcExports.start(e)
	if err != nil {
		return err
	}

	frozen, err := vm.freezeDisk()
	if err != nil {
		fcExports.restore(e, prev)
		return err
	}

	go fcFinishExport(e, frozen)

	return nil
}

// freezeDisk takes a frozen copy of disk 0, and pauses the VM for the copy
// when it runs.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) freezeDisk() (fcFrozenDisk, error) {
	vm.exportSeq++
	n := vm.exportSeq

	running := vm.State == VM_RUNNING
	if running && vm.client == nil {
		return fcFrozenDisk{}, vmNotRunning(vm.Name)
	}
	if running && vm.disks[0].dmName == "" && !fcReflinkOK() {
		return fcFrozenDisk{}, fmt.Errorf("vm %v is running, and export-disk cannot make a fast copy of its disk: the disk is not a device-mapper snapshot and %v does not support reflink. Pause or stop the VM first", vm.Name, *f_firecrackerDir)
	}

	var start time.Time
	if running {
		if err := vm.client.PatchVMState("Paused"); err != nil {
			return fcFrozenDisk{}, fmt.Errorf("unable to pause: %v", err)
		}
		start = time.Now()
	}

	f, err := fcFreezeDiskCopy(vm.disks[0], fcInstanceDir(vm.ID), vm.ID, n, !running)

	if running {
		if err2 := vm.client.PatchVMState("Resumed"); err2 != nil {
			if err == nil {
				if err3 := f.release(); err3 != nil {
					log.Error("vm %v: remove frozen disk: %v", vm.ID, err3)
				}
			}
			return fcFrozenDisk{}, vm.setErrorf("unable to resume: %v", err2)
		}
		log.Info("vm %v: export-disk paused the VM for %v", vm.ID, time.Since(start))
	}

	return f, err
}

// fcFreezeDisk copies disk d of VM vmID into instanceDir. A device-mapper
// disk gets a copy of its copy-on-write file in a new snapshot of the same
// base. A file gets a reflink copy, or a sparse copy without reflink.
func fcFreezeDisk(d fcDisk, instanceDir string, vmID, n int) (fcFrozenDisk, error) {
	return fcFreezeDiskCopy(d, instanceDir, vmID, n, true)
}

// fcFreezeDiskCopy is fcFreezeDisk. With slow false, it returns an error
// instead of a sparse copy when the reflink copy fails.
func fcFreezeDiskCopy(d fcDisk, instanceDir string, vmID, n int, slow bool) (fcFrozenDisk, error) {
	if d.dmName != "" {
		return fcFreezeDM(d, instanceDir, vmID, n)
	}

	dst := filepath.Join(instanceDir, fmt.Sprintf("export-%d.raw", n))
	f := fcFrozenDisk{Path: dst, files: []string{dst}}

	if fcReflinkOK() {
		err := fcCloneFile(d.HostPath, dst)
		if err == nil {
			return f, nil
		}
		if !slow {
			os.Remove(dst)
			return fcFrozenDisk{}, fmt.Errorf("reflink copy of %v failed, and a running VM needs a fast copy: %v. Pause or stop the VM first", d.HostPath, err)
		}
		log.Warn("vm %v: reflink copy failed, using a sparse copy: %v", vmID, err)
	}

	if err := fcSparseCopy(d.HostPath, dst); err != nil {
		os.Remove(dst)
		return fcFrozenDisk{}, err
	}

	return f, nil
}

func fcFreezeDM(d fcDisk, instanceDir string, vmID, n int) (fcFrozenDisk, error) {
	f := fcFrozenDisk{}

	fail := func(err error) (fcFrozenDisk, error) {
		if err2 := f.release(); err2 != nil {
			log.Warn("vm %v: remove frozen disk: %v", vmID, err2)
		}
		return fcFrozenDisk{}, err
	}

	// Write the dirty pages of the snapshot to the copy-on-write file.
	if out, err := fcRunCmd("blockdev", "--flushbufs", d.HostPath); err != nil {
		return fail(fmt.Errorf("blockdev --flushbufs %v: %v: %v", d.HostPath, strings.TrimSpace(out), err))
	}

	cached := filepath.Join(fcCacheDir(), d.key+".raw")
	fi, err := os.Stat(cached)
	if err != nil {
		return fail(err)
	}

	cow := filepath.Join(instanceDir, fmt.Sprintf("export-%d.cow", n))
	f.files = append(f.files, cow)
	if err := fcSparseCopy(d.cowFile, cow); err != nil {
		return fail(err)
	}

	base, err := fcCache.attachBase(d.key, cached)
	if err != nil {
		return fail(err)
	}
	f.baseKey = d.key

	out, err := fcRunCmd("losetup", "--find", "--show", cow)
	if err != nil {
		return fail(fmt.Errorf("attach %v: %v: %v", cow, strings.TrimSpace(out), err))
	}
	f.cowLoop = strings.TrimSpace(out)

	name := fmt.Sprintf("mm-fc-%s-%d-x%d", fcInstance, vmID, n)
	if out, err := fcRunCmd("dmsetup", "create", name, "--table", fcDMSnapshotTable(fi.Size(), base, f.cowLoop)); err != nil {
		return fail(fmt.Errorf("dmsetup create %v: %v: %v", name, strings.TrimSpace(out), err))
	}
	f.dmName = name

	if f.Path, err = fcDMNode(name); err != nil {
		return fail(err)
	}

	return f, nil
}

// fcFinishExport writes the frozen disk to the destination of e, then
// removes the frozen disk.
func fcFinishExport(e *fcExport, f fcFrozenDisk) {
	err := fcWriteExport(e, f.Path)

	if err2 := f.release(); err2 != nil {
		log.Error("vm %v: remove frozen disk: %v", e.ID, err2)
	}

	if err != nil {
		log.Error("vm %v: export-disk to %v failed: %v", e.ID, e.Dst, err)
		e.finish(fcExportFailed)
		return
	}

	log.Info("vm %v: exported disk to %v", e.ID, e.Dst)
	e.finish(fcExportCompleted)
}

// fcWriteExport converts src to a temporary file and links it to the
// destination of e.
func fcWriteExport(e *fcExport, src string) error {
	fcLaunchSem.Acquire()
	defer fcLaunchSem.Release()

	tmp := e.Dst + ".tmp-" + fcRandomSuffix()

	var err error
	if e.Format == "raw" {
		err = fcSparseCopyProgress(src, tmp, e.progress)
	} else {
		err = fcConvertQcow2(src, tmp, e.progress)
	}
	if err == nil {
		err = os.Chmod(tmp, 0644)
	}
	// A link, unlike a rename, fails when another export made e.Dst first.
	if err == nil {
		if err = os.Link(tmp, e.Dst); errors.Is(err, os.ErrExist) {
			err = fmt.Errorf("%v already exists", e.Dst)
		}
	}
	os.Remove(tmp)

	return err
}

// fcConvertQcow2Default runs qemu-img convert -p and reports the percent
// that qemu-img prints.
func fcConvertQcow2Default(src, dst string, progress func(float64)) error {
	cmd := exec.Command("qemu-img", "convert", "-p", "-f", "raw", "-O", "qcow2", src, dst)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("qemu-img convert %v: %v", src, err)
	}

	fcReadProgress(out, progress)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("qemu-img convert %v: %v: %v", src, strings.TrimSpace(stderr.String()), err)
	}

	return nil
}

var fcProgressRE = regexp.MustCompile(`\((\d+(?:\.\d+)?)/100%\)`)

// fcReadProgress reads qemu-img -p output, such as "    (42.00/100%)\r",
// until EOF, and calls progress with each percent.
func fcReadProgress(r io.Reader, progress func(float64)) {
	s := bufio.NewScanner(r)
	s.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for s.Scan() {
		m := fcProgressRE.FindStringSubmatch(s.Text())
		if m == nil {
			continue
		}
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			progress(v)
		}
	}

	// Drain the rest so that qemu-img does not block on a full pipe.
	io.Copy(io.Discard, r)
}
