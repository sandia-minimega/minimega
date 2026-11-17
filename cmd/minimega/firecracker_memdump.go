// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// Guest memory layout of Firecracker v1.17 on x86_64, from
// src/vmm/src/arch/x86_64/layout.rs.
const (
	// MMIO32_MEM_START
	fcMMIO32Start = 3 << 30

	// MMIO32_MEM_SIZE
	fcMMIO32Size = 1 << 30

	// MMIO64_MEM_START
	fcMMIO64Start = 256 << 30

	// MMIO64_MEM_SIZE
	fcMMIO64Size = 256 << 30

	// PAST_64BITS_MMIO_SIZE, the size of the memory past the 64-bit MMIO gap
	fcPast64Size = 512 << 30
)

// fcMemDumpArch is the host architecture. Tests replace it.
var fcMemDumpArch = runtime.GOARCH

// fcMemDumpSnapshot creates a full snapshot through c. Tests replace it.
var fcMemDumpSnapshot = func(ctx context.Context, c *fcClient, statePath, memPath string) error {
	return c.CreateSnapshot(ctx, statePath, memPath)
}

// fcMemDumps holds the latest memory dump of each Firecracker VM, by VM ID.
var fcMemDumps = &fcExportRegistry{m: map[int]*fcExport{}}

// fcMemDumpInfo is the content of the JSON file next to a raw memory dump.
type fcMemDumpInfo struct {
	Format             string      `json:"format"`
	Arch               string      `json:"arch"`
	FirecrackerVersion string      `json:"firecracker_version"`
	VCPUs              uint64      `json:"vcpus"`
	MemoryMiB          uint64      `json:"memory_mib"`
	Regions            []memRegion `json:"regions"`
}

// fcMemDump is one memory dump after its snapshot.
type fcMemDump struct {
	job     *fcExport
	mem     string
	regions []memRegion
	info    fcMemDumpInfo
}

// fcMemoryRegions returns the guest memory regions of a Firecracker VM with
// memMiB of memory on an x86_64 host, as arch_memory_regions of Firecracker
// v1.17 computes them. Offset is the position of each region in the snapshot
// memory file, which holds the regions back to back. It returns an error for a
// size that the layout does not cover. The layout does not depend on the PCI
// transport. minimega does not enable memory hotplug, so the layout has no
// hotplug region.
func fcMemoryRegions(memMiB uint64) ([]memRegion, error) {
	const limit = fcMMIO32Start + (fcMMIO64Start - fcMMIO32Start - fcMMIO32Size) + fcPast64Size

	if memMiB == 0 {
		return nil, errors.New("memory size is 0")
	}
	if memMiB > limit>>20 {
		return nil, fmt.Errorf("memory dumps of firecracker VMs support at most %d MiB of memory", limit>>20)
	}

	var regions []memRegion
	var off uint64

	add := func(start, size uint64) {
		regions = append(regions, memRegion{Start: start, Size: size, Offset: off})
		off += size
	}

	size := memMiB << 20

	low := min(size, fcMMIO32Start)
	add(0, low)
	size -= low

	if size > 0 {
		mid := min(size, fcMMIO64Start-(fcMMIO32Start+fcMMIO32Size))
		add(fcMMIO32Start+fcMMIO32Size, mid)
		size -= mid
	}

	if size > 0 {
		add(fcMMIO64Start+fcMMIO64Size, size)
	}

	return regions, nil
}

// fcMemDumpTimeout returns how long a snapshot of memMiB of memory can take:
// one minute, plus 30 seconds for each GiB.
func fcMemDumpTimeout(memMiB uint64) time.Duration {
	return time.Minute + time.Duration(memMiB)*30*time.Second/1024
}

// MemoryDump starts a dump of the guest memory to dst in format elf or raw.
// It pauses a running VM while Firecracker writes a full snapshot, then
// resumes it. It returns before minimega writes dst, which vm memory-dump
// with no arguments reports.
func (vm *FirecrackerVM) MemoryDump(dst, format string) error {
	if format != "elf" && format != "raw" {
		return fmt.Errorf("invalid memory dump format: %v", format)
	}

	// HasClient takes a ron lock, so it runs without vm.lock.
	vm.lock.Lock()
	cc, uuid := vm.cc, vm.UUID
	vm.lock.Unlock()

	if s, ok := cc.(interface{ HasClient(string) bool }); ok && s.HasClient(uuid) {
		log.Warn("vm %v: the memory dump resets the vsock connections of the guest, so cc commands, file transfers, and tunnels in progress fail", vm.ID)
	}

	vm.lock.Lock()
	d, err := vm.startMemoryDump(dst, format)
	vm.lock.Unlock()
	if err != nil {
		return err
	}

	go fcFinishMemoryDump(d)

	return nil
}

// startMemoryDump records a dump of the VM in fcMemDumps and writes the
// snapshot memory file of the dump.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) startMemoryDump(dst, format string) (*fcMemDump, error) {
	switch vm.State {
	case VM_RUNNING, VM_PAUSED:
	default:
		return nil, fmt.Errorf("vm %v is %v: vm memory-dump needs a vm in running or paused", vm.Name, vm.State)
	}
	if vm.client == nil {
		return nil, vmNotRunning(vm.Name)
	}
	if fcMemDumpArch != "amd64" {
		return nil, fmt.Errorf("memory dumps of firecracker VMs need an x86_64 host, not %v", fcMemDumpArch)
	}

	regions, err := fcMemoryRegions(vm.Memory)
	if err != nil {
		return nil, err
	}

	job := &fcExport{
		ID:        vm.ID,
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Dst:       dst,
		Format:    format,
		status:    fcExportActive,
	}

	info := fcMemDumpInfo{
		Format:    format,
		Arch:      "x86_64",
		VCPUs:     vm.VCPUs,
		MemoryMiB: vm.Memory,
		Regions:   regions,
	}

	prev, err := fcMemDumps.start(job)
	if err != nil {
		return nil, fmt.Errorf("vm %v already has a memory-dump in progress", vm.Name)
	}

	if format == "raw" {
		v, err := vm.client.GetVersion()
		if err != nil {
			fcMemDumps.restore(job, prev)
			return nil, fmt.Errorf("unable to get firecracker version: %v", err)
		}
		info.FirecrackerVersion = v
	}

	mem, err := vm.snapshotMemory()
	if err != nil {
		fcMemDumps.restore(job, prev)
		return nil, err
	}

	return &fcMemDump{job: job, mem: mem, regions: regions, info: info}, nil
}

// snapshotMemory pauses a running VM, writes a full snapshot to /out in the
// jail, and resumes the VM. It moves the memory file into the instance
// directory, removes the state file, and returns the path of the memory file.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) snapshotMemory() (string, error) {
	vm.exportSeq++
	dst := filepath.Join(fcInstanceDir(vm.ID), fmt.Sprintf("memdump-%d.mem", vm.exportSeq))

	statePath, memPath := "/out/dump.state", "/out/dump.mem"
	hostMem := vm.jail.HostPath(memPath)
	defer os.Remove(vm.jail.HostPath(statePath))

	running := vm.State == VM_RUNNING

	var start time.Time
	if running {
		if err := vm.client.PatchVMState("Paused"); err != nil {
			return "", fmt.Errorf("unable to pause: %v", err)
		}
		start = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), fcMemDumpTimeout(vm.Memory))
	err := fcMemDumpSnapshot(ctx, vm.client, statePath, memPath)
	cancel()

	if running {
		if err2 := vm.client.PatchVMState("Resumed"); err2 != nil {
			os.Remove(hostMem)
			return "", vm.setErrorf("unable to resume: %v", err2)
		}
		log.Info("vm %v: memory-dump paused the VM for %v", vm.ID, time.Since(start))
	}

	if err != nil {
		os.Remove(hostMem)
		return "", fmt.Errorf("unable to create snapshot: %v", err)
	}

	if err := fcMemDumpMove(hostMem, dst, nil); err != nil {
		os.Remove(hostMem)
		return "", fmt.Errorf("unable to move memory file: %v", err)
	}

	return dst, nil
}

// fcMemDumpMove renames src to dst. When they are on different filesystems,
// it copies src to dst and removes src.
func fcMemDumpMove(src, dst string, progress func(float64)) error {
	err := os.Rename(src, dst)
	if err == nil {
		if progress != nil {
			progress(100)
		}
		return nil
	}
	if !errors.Is(err, unix.EXDEV) {
		return err
	}

	if err := fcSparseCopyProgress(src, dst, progress); err != nil {
		os.Remove(dst)
		return err
	}

	return os.Remove(src)
}

// fcFinishMemoryDump writes the destination of d, then removes the memory
// file of d.
func fcFinishMemoryDump(d *fcMemDump) {
	err := fcWriteMemoryDump(d)

	if err2 := os.Remove(d.mem); err2 != nil && !errors.Is(err2, os.ErrNotExist) {
		log.Error("vm %v: remove memory file: %v", d.job.ID, err2)
	}

	if err != nil {
		log.Error("vm %v: memory-dump to %v failed: %v", d.job.ID, d.job.Dst, err)
		d.job.finish(fcExportFailed)
		return
	}

	log.Info("vm %v: wrote memory dump to %v", d.job.ID, d.job.Dst)
	d.job.finish(fcExportCompleted)
}

// fcWriteMemoryDump writes the destination of d from its memory file: an ELF
// core file, or the memory file itself and a JSON file with the region map.
// It writes temporary files and renames them into place.
func fcWriteMemoryDump(d *fcMemDump) error {
	fcLaunchSem.Acquire()
	defer fcLaunchSem.Release()

	var total uint64
	for _, r := range d.regions {
		total += r.Size
	}

	fi, err := os.Stat(d.mem)
	if err != nil {
		return err
	}
	if uint64(fi.Size()) != total {
		return fmt.Errorf("memory file is %d bytes, but the memory layout of %d MiB has %d bytes", fi.Size(), d.info.MemoryMiB, total)
	}

	dst := d.job.Dst
	suffix := ".tmp-" + fcRandomSuffix()
	tmp := dst + suffix

	tmps := []string{tmp}
	fail := func(err error) error {
		for _, p := range tmps {
			os.Remove(p)
		}
		return err
	}

	renames := [][2]string{{tmp, dst}}

	if d.job.Format == "raw" {
		if err := fcMemDumpMove(d.mem, tmp, d.job.progress); err != nil {
			return fail(err)
		}

		// The VMM wrote the memory file as the VM uid.
		if err := fcChown(tmp, os.Geteuid(), os.Getegid()); err != nil {
			return fail(err)
		}

		data, err := json.MarshalIndent(d.info, "", "    ")
		if err != nil {
			return fail(err)
		}

		jsonTmp := dst + ".json" + suffix
		tmps = append(tmps, jsonTmp)
		if err := os.WriteFile(jsonTmp, append(data, '\n'), 0600); err != nil {
			return fail(err)
		}
		renames = append(renames, [2]string{jsonTmp, dst + ".json"})
	} else if err := fcMemDumpELF(d.mem, tmp, d.regions, d.job.progress); err != nil {
		return fail(err)
	}

	if err := os.Chmod(tmp, 0600); err != nil {
		return fail(err)
	}

	for _, r := range renames {
		if _, err := os.Lstat(r[1]); err == nil {
			return fail(fmt.Errorf("%v already exists", r[1]))
		}
	}
	for _, r := range renames {
		if err := os.Rename(r[0], r[1]); err != nil {
			return fail(err)
		}
	}

	return nil
}

// fcMemDumpELF writes the ELF core file dst from the memory file src.
func fcMemDumpELF(src, dst string, regions []memRegion, progress func(float64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	if err := writeELFCore(out, in, regions, progress); err != nil {
		out.Close()
		return fmt.Errorf("write %v: %v", dst, err)
	}

	return out.Close()
}
