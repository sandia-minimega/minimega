// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// Tests replace these.
var (
	fcRunCmd = processWrapper

	fcReflinkOK = func() bool {
		return fcCheckDir(*f_firecrackerDir).Reflink
	}

	fcDMUsable = func() bool {
		if _, err := os.Stat("/dev/mapper/control"); err != nil {
			return false
		}
		for _, b := range []string{"dmsetup", "losetup"} {
			if _, err := exec.LookPath(b); err != nil {
				return false
			}
		}
		return true
	}

	fcCloneFile = fcReflinkCopy
	fcCacheFill = fcFillCache
)

// fcSemaphore limits the number of Firecracker VMs that do slow launch work,
// such as conversions and full copies, at the same time.
type fcSemaphore struct {
	once sync.Once
	ch   chan struct{}
	size func() int // the limit; nil means fcLaunchParallel
}

func (s *fcSemaphore) Acquire() {
	s.once.Do(func() {
		size := fcLaunchParallel
		if s.size != nil {
			size = s.size
		}
		s.ch = make(chan struct{}, size())
	})
	s.ch <- struct{}{}
}

func (s *fcSemaphore) Release() {
	<-s.ch
}

// fcLaunchSem is sized by -firecracker-launch-parallel. It is not reentrant.
var fcLaunchSem fcSemaphore

// fcImage is one image of a `qemu-img info --backing-chain` result.
type fcImage struct {
	Filename            string `json:"filename"`
	Format              string `json:"format"`
	VirtualSize         int64  `json:"virtual-size"`
	BackingFilename     string `json:"backing-filename"`
	FullBackingFilename string `json:"full-backing-filename"`
}

type fcDiskMode int

const (
	fcDiskCached fcDiskMode = iota
	fcDiskOverlay
	fcDiskConvert
)

// fcDiskChain returns the backing chain of src, src first. It runs qemu-img
// again only when src or an image in its chain changes.
func fcDiskChain(src string) ([]fcImage, error) {
	if chain, ok := fcCache.chains.get(src); ok {
		return append([]fcImage(nil), chain...), nil
	}

	before, statErr := fcStatSource(src)

	out, err := fcRunCmd("qemu-img", "info", "--output=json", "--backing-chain", src)
	if err != nil {
		return nil, fmt.Errorf("qemu-img info %v: %v: %v", src, strings.TrimSpace(out), err)
	}

	chain, err := fcParseChain(src, out)
	if err != nil {
		return nil, err
	}

	if statErr == nil {
		var deps []string
		for _, img := range chain[1:] {
			p := img.Filename
			if !filepath.IsAbs(p) {
				p = filepath.Join(filepath.Dir(src), p)
			}
			deps = append(deps, p)
		}
		fcCache.chains.put(src, before, append([]fcImage(nil), chain...), deps...)
	}

	return chain, nil
}

func fcParseChain(src, out string) ([]fcImage, error) {
	var chain []fcImage
	if err := json.Unmarshal([]byte(out), &chain); err != nil {
		return nil, fmt.Errorf("qemu-img info %v: %v", src, err)
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("qemu-img info %v: no images", src)
	}

	return chain, nil
}

// fcPlanDisk chooses how to build a raw disk from a backing chain. For a
// cached disk or an overlay, it also returns the image to cache.
func fcPlanDisk(chain []fcImage) (fcDiskMode, fcImage, error) {
	switch {
	case len(chain) == 0:
		return 0, fcImage{}, errors.New("empty backing chain")
	case len(chain) == 1:
		return fcDiskCached, chain[0], nil
	case len(chain) == 2 && chain[0].Format == "qcow2" && (chain[1].Format == "raw" || chain[1].Format == "qcow2"):
		base := chain[1]
		if chain[0].FullBackingFilename != "" {
			base.Filename = chain[0].FullBackingFilename
		}
		if !filepath.IsAbs(base.Filename) {
			base.Filename = filepath.Join(filepath.Dir(chain[0].Filename), base.Filename)
		}
		return fcDiskOverlay, base, nil
	}

	return fcDiskConvert, fcImage{}, nil
}

// fcSourceID identifies one version of a source image.
type fcSourceID struct {
	Path  string
	Size  int64
	Mtime int64
	Inode uint64
}

func fcStatSource(src string) (fcSourceID, error) {
	p := fcResolve(src)

	fi, err := os.Stat(p)
	if err != nil {
		return fcSourceID{}, err
	}

	id := fcSourceID{Path: p, Size: fi.Size(), Mtime: fi.ModTime().UnixNano()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		id.Inode = uint64(st.Ino)
	}

	return id, nil
}

// fcStatCacheMax is the maximum number of entries in a statCache.
const fcStatCacheMax = 1024

// statCache keeps one value for each path. A value is valid while the path
// and each file that the value depends on keep the same fcSourceID.
type statCache[V any] struct {
	mu sync.Mutex
	m  map[string]statCacheEntry[V]
}

type statCacheEntry[V any] struct {
	files []string
	ids   []fcSourceID
	v     V
}

func newStatCache[V any]() *statCache[V] {
	return &statCache[V]{m: map[string]statCacheEntry[V]{}}
}

// get returns the value of path when no file that it depends on changed.
func (c *statCache[V]) get(path string) (V, bool) {
	c.mu.Lock()
	e, ok := c.m[path]
	c.mu.Unlock()

	var zero V
	if !ok {
		return zero, false
	}

	for i, f := range e.files {
		if id, err := fcStatSource(f); err != nil || id != e.ids[i] {
			return zero, false
		}
	}

	return e.v, true
}

// put keeps v for path. v also depends on deps. before is the fcSourceID of
// path from before v was made. put keeps nothing when path changed since then,
// or when it cannot read a file.
func (c *statCache[V]) put(path string, before fcSourceID, v V, deps ...string) {
	files := append([]string{path}, deps...)
	ids := make([]fcSourceID, len(files))
	for i, f := range files {
		id, err := fcStatSource(f)
		if err != nil {
			return
		}
		ids[i] = id
	}
	if ids[0] != before {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.m[path]; !ok && len(c.m) >= fcStatCacheMax {
		for k := range c.m {
			delete(c.m, k)
			break
		}
	}
	c.m[path] = statCacheEntry[V]{files: files, ids: ids, v: v}
}

func fcCacheKey(id fcSourceID) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%d", id.Path, id.Size, id.Mtime, id.Inode)))
	return hex.EncodeToString(sum[:])[:16]
}

func fcCacheDir() string {
	return filepath.Join(fcResolve(*f_firecrackerDir), "cache")
}

// fcCacheMeta is the content of <key>.json.
type fcCacheMeta struct {
	Source  string    `json:"source"`
	Size    int64     `json:"size"`
	Mtime   int64     `json:"mtime"`
	Inode   uint64    `json:"inode"`
	Format  string    `json:"format"`
	Created time.Time `json:"created"`
}

type fcInflight struct {
	done chan struct{}
	err  error
}

type fcBaseLoop struct {
	dev  string
	refs int
}

// fcCacheState is the in-memory state of the cache: fills in progress, the VMs
// that use each entry, and the read-only loop device of each entry.
type fcCacheState struct {
	mu       sync.Mutex
	inflight map[string]*fcInflight
	users    map[string]map[int]int
	bases    map[string]*fcBaseLoop

	// chains keeps the backing chain of each source image.
	chains *statCache[[]fcImage]
}

func newFCCacheState() *fcCacheState {
	return &fcCacheState{
		inflight: map[string]*fcInflight{},
		users:    map[string]map[int]int{},
		bases:    map[string]*fcBaseLoop{},
		chains:   newStatCache[[]fcImage](),
	}
}

var fcCache = newFCCacheState()

// get returns the cache entry for src, filling it once when it is missing,
// and records that VM vmID uses it.
func (c *fcCacheState) get(src, format string, vmID int) (string, string, error) {
	id, err := fcStatSource(src)
	if err != nil {
		return "", "", fmt.Errorf("cache %v: %v", src, err)
	}

	key := fcCacheKey(id)
	dir := fcCacheDir()
	path := filepath.Join(dir, key+".raw")

	for {
		c.mu.Lock()
		if f, ok := c.inflight[key]; ok {
			c.mu.Unlock()
			<-f.done
			if f.err != nil {
				return "", "", f.err
			}
			continue
		}

		if _, err := os.Stat(path); err == nil {
			c.addUser(key, vmID)
			c.mu.Unlock()
			return key, path, nil
		}

		f := &fcInflight{done: make(chan struct{})}
		c.inflight[key] = f
		c.mu.Unlock()

		f.err = fcFillEntry(dir, key, id, format)

		c.mu.Lock()
		delete(c.inflight, key)
		if f.err == nil {
			c.addUser(key, vmID)
		}
		c.mu.Unlock()
		close(f.done)

		return key, path, f.err
	}
}

// LOCK: Assumes c.mu is held.
func (c *fcCacheState) addUser(key string, vmID int) {
	if c.users[key] == nil {
		c.users[key] = map[int]int{}
	}
	c.users[key][vmID]++
}

func (c *fcCacheState) removeUser(key string, vmID int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.users[key] == nil {
		return
	}
	if c.users[key][vmID]--; c.users[key][vmID] <= 0 {
		delete(c.users[key], vmID)
	}
	if len(c.users[key]) == 0 {
		delete(c.users, key)
	}
}

// inUse returns the number of VMs that use key.
func (c *fcCacheState) inUse(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.users[key])
}

// attachBase returns the read-only loop device of the entry, and attaches it
// on first use.
func (c *fcCacheState) attachBase(key, path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if b, ok := c.bases[key]; ok {
		b.refs++
		return b.dev, nil
	}

	out, err := fcRunCmd("losetup", "--find", "--show", "--read-only", path)
	if err != nil {
		return "", fmt.Errorf("attach %v: %v: %v", path, strings.TrimSpace(out), err)
	}

	dev := strings.TrimSpace(out)
	c.bases[key] = &fcBaseLoop{dev: dev, refs: 1}

	return dev, nil
}

// releaseBase drops a reference to the loop device of the entry, and detaches
// it at zero.
func (c *fcCacheState) releaseBase(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.bases[key]
	if !ok {
		return nil
	}
	if b.refs--; b.refs > 0 {
		return nil
	}

	delete(c.bases, key)
	if out, err := fcRunCmd("losetup", "-d", b.dev); err != nil {
		return fmt.Errorf("detach %v: %v: %v", b.dev, strings.TrimSpace(out), err)
	}

	return nil
}

func fcRandomSuffix() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// fcFillEntry writes <key>.raw and <key>.json. It writes to temporary files
// and renames them into place.
func fcFillEntry(dir, key string, id fcSourceID, format string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cache: %v", err)
	}

	suffix := ".tmp-" + fcRandomSuffix()
	tmp := filepath.Join(dir, key+".raw"+suffix)

	log.Info("caching %v (%v) as %v", id.Path, format, key)

	fcLaunchSem.Acquire()
	err := fcCacheFill(id.Path, tmp, format)
	fcLaunchSem.Release()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cache %v: %v", id.Path, err)
	}

	meta, err := json.Marshal(fcCacheMeta{
		Source:  id.Path,
		Size:    id.Size,
		Mtime:   id.Mtime,
		Inode:   id.Inode,
		Format:  format,
		Created: time.Now(),
	})
	if err != nil {
		os.Remove(tmp)
		return err
	}

	metaTmp := filepath.Join(dir, key+".json"+suffix)
	if err := os.WriteFile(metaTmp, meta, 0600); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cache %v: %v", id.Path, err)
	}
	if err := os.Rename(metaTmp, filepath.Join(dir, key+".json")); err != nil {
		os.Remove(tmp)
		os.Remove(metaTmp)
		return fmt.Errorf("cache %v: %v", id.Path, err)
	}

	if err := os.Rename(tmp, filepath.Join(dir, key+".raw")); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cache %v: %v", id.Path, err)
	}

	return nil
}

// fcFillCache writes a raw copy of src to dst.
func fcFillCache(src, dst, format string) error {
	if format == "raw" {
		return fcSparseCopy(src, dst)
	}

	if out, err := fcRunCmd("qemu-img", "convert", "-O", "raw", src, dst); err != nil {
		return fmt.Errorf("qemu-img convert %v: %v: %v", src, strings.TrimSpace(out), err)
	}

	return nil
}

func fcReflinkCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		out.Close()
		return fmt.Errorf("reflink %v to %v: %v", src, dst, err)
	}

	return out.Close()
}

// fcSparseCopy copies src to dst and leaves holes where src has holes or
// zero blocks.
func fcSparseCopy(src, dst string) error {
	return fcSparseCopyProgress(src, dst, nil)
}

// fcSparseCopyProgress is fcSparseCopy that also accepts a block device as
// src. When progress is not nil, it gets the percent of src that is copied.
func fcSparseCopyProgress(src, dst string, progress func(float64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	if !fi.Mode().IsRegular() {
		if size, err = in.Seek(0, io.SeekEnd); err != nil {
			return err
		}
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if err := fcCopyData(in, out, size, progress); err != nil {
		out.Close()
		return fmt.Errorf("copy %v to %v: %v", src, dst, err)
	}
	if err := out.Truncate(size); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

func fcCopyData(in, out *os.File, size int64, progress func(float64)) error {
	buf := make([]byte, 64<<10)

	report := func(pos int64) {
		if progress != nil && size > 0 {
			progress(float64(pos) * 100 / float64(size))
		}
	}

	for off := int64(0); off < size; {
		data, err := in.Seek(off, unix.SEEK_DATA)
		if err != nil {
			if errors.Is(err, unix.ENXIO) {
				return nil
			}
			if !errors.Is(err, unix.EINVAL) {
				return err
			}
			data = off
		}

		end, err := in.Seek(data, unix.SEEK_HOLE)
		if err != nil {
			end = size
		}

		for pos := data; pos < end; {
			n := int64(len(buf))
			if end-pos < n {
				n = end - pos
			}

			m, err := in.ReadAt(buf[:n], pos)
			if m > 0 && !fcAllZero(buf[:m]) {
				if _, err := out.WriteAt(buf[:m], pos); err != nil {
					return err
				}
			}
			pos += int64(m)
			report(pos)

			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}

		off = end
		report(off)
	}

	return nil
}

func fcAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// fcDisk is a disk that is ready for one Firecracker VM.
type fcDisk struct {
	HostPath  string
	IsBlock   bool
	ReadOnly  bool
	Scratch   bool
	CacheType string

	vmID         int
	key          string
	baseAttached bool
	cowFile      string
	cowLoop      string
	dmName       string
	aclPath      string
	aclUID       int
}

// fcDMName returns the device-mapper name of disk index of VM vmID.
func fcDMName(vmID, index int) string {
	return fmt.Sprintf("mm-fc-%s-%d-%d", fcInstance, vmID, index)
}

// fcDMSnapshotTable returns a persistent snapshot table with 4 KiB chunks.
func fcDMSnapshotTable(size int64, base, cow string) string {
	return fmt.Sprintf("0 %d snapshot %s %s P 8", size/512, base, cow)
}

// fcDMNodeTimeout is how long fcDMNode waits for a /dev/mapper node.
const fcDMNodeTimeout = 5 * time.Second

// fcDMNode makes the /dev/mapper node of the device-mapper target name, waits
// for it, and returns its path. Without udev, for example in a container,
// dmsetup create can return before the node exists.
func fcDMNode(name string) (string, error) {
	p := filepath.Join("/dev/mapper", name)

	if out, err := fcRunCmd("dmsetup", "mknodes", name); err != nil {
		log.Warn("dmsetup mknodes %v: %v: %v", name, strings.TrimSpace(out), err)
	}

	if err := fcWaitDev(p, fcDMNodeTimeout); err != nil {
		return "", fmt.Errorf("dmsetup create %v: %v", name, err)
	}

	return p, nil
}

func fcCacheType(cache string) string {
	if cache == "unsafe" {
		return "Unsafe"
	}
	return "Writeback"
}

// fcPrepareDisk builds disk index of VM vmID from dc in instanceDir. The VMM
// runs as uid. A read-only disk is the cache entry itself, which all VMs
// share, and snapshot does not apply to it.
func fcPrepareDisk(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (d fcDisk, err error) {
	d = fcDisk{CacheType: fcCacheType(dc.Cache), vmID: vmID}

	defer func() {
		if err != nil {
			if err2 := fcReleaseDisk(d); err2 != nil {
				log.Warn("clean up disk %v: %v", dc.Path, err2)
			}
			d = fcDisk{}
		}
	}()

	src := fcResolve(dc.Path)

	chain, err := fcDiskChain(src)
	if err != nil {
		return d, err
	}

	if dc.ReadOnly {
		// The cache key covers only src, so a change to a backing file would
		// not be seen.
		if len(chain) != 1 {
			return d, fmt.Errorf("firecracker needs an image without a backing file for a disk with ro: %v", dc.Path)
		}

		d.key, d.HostPath, err = fcCache.get(src, chain[0].Format, vmID)
		if err != nil {
			return d, err
		}

		// The VMM reads the hard link in the jail as the VM uid.
		if err := os.Chmod(d.HostPath, 0444); err != nil {
			return d, err
		}
		d.ReadOnly = true

		return d, nil
	}

	if !snapshot {
		if len(chain) != 1 || chain[0].Format != "raw" {
			return d, fmt.Errorf("firecracker needs a raw image when snapshot is false: %v", dc.Path)
		}

		if out, err := fcRunCmd("setfacl", "-m", fmt.Sprintf("u:%d:rw", uid), src); err != nil {
			return d, fmt.Errorf("setfacl %v: %v: %v", src, strings.TrimSpace(out), err)
		}
		d.aclPath, d.aclUID = src, uid
		d.HostPath = src

		return d, nil
	}

	dst := filepath.Join(instanceDir, fmt.Sprintf("disk-%d.raw", index))

	mode, base, err := fcPlanDisk(chain)
	if err != nil {
		return d, err
	}

	switch mode {
	case fcDiskCached:
		d.key, d.HostPath, err = fcCache.get(src, base.Format, vmID)
		if err != nil {
			return d, err
		}

		if fcReflinkOK() {
			if err := fcCloneFile(d.HostPath, dst); err != nil {
				return d, err
			}
		} else if fcDMUsable() {
			err = fcSnapshotDisk(&d, instanceDir, index)
			return d, err
		} else if err := fcFullCopy(d.HostPath, dst); err != nil {
			return d, err
		}
	case fcDiskOverlay:
		var cached string
		d.key, cached, err = fcCache.get(base.Filename, base.Format, vmID)
		if err != nil {
			return d, err
		}

		if fcReflinkOK() {
			err = fcCloneFile(cached, dst)
		} else {
			err = fcFullCopy(cached, dst)
		}
		if err != nil {
			return d, err
		}

		if err := fcCommitOverlay(src, dst, instanceDir, index); err != nil {
			return d, err
		}
	case fcDiskConvert:
		fcLaunchSem.Acquire()
		out, err := fcRunCmd("qemu-img", "convert", "-O", "raw", src, dst)
		fcLaunchSem.Release()
		if err != nil {
			return d, fmt.Errorf("qemu-img convert %v: %v: %v", src, strings.TrimSpace(out), err)
		}
	}

	if err := fcChown(dst, uid, uid); err != nil {
		return d, fmt.Errorf("chown %v: %v", dst, err)
	}
	d.HostPath = dst

	return d, nil
}

// fcPrepareScratch creates an empty sparse scratch disk of size bytes in
// instanceDir. uid owns it.
func fcPrepareScratch(instanceDir string, size int64, uid int) (fcDisk, error) {
	p := filepath.Join(instanceDir, "scratch.raw")

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fcDisk{}, fmt.Errorf("create scratch disk: %v", err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return fcDisk{}, fmt.Errorf("create scratch disk: %v", err)
	}
	if err := f.Close(); err != nil {
		return fcDisk{}, fmt.Errorf("create scratch disk: %v", err)
	}

	if err := fcChown(p, uid, uid); err != nil {
		return fcDisk{}, fmt.Errorf("chown %v: %v", p, err)
	}

	return fcDisk{HostPath: p, Scratch: true, CacheType: fcCacheType("")}, nil
}

// fcDriveID returns the drive ID of disk i of a VM. It is also the name of
// the disk in the jail.
func fcDriveID(i int, d fcDisk) string {
	if d.Scratch {
		return "scratch"
	}
	return fmt.Sprintf("disk%d", i)
}

// fcParseSize parses a size such as 4096, 64M, or 1G. The K, M, G, and T
// suffixes are powers of 1024. An empty size is 0.
func fcParseSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	num, mult := s, int64(1)
	switch strings.ToUpper(s[len(s)-1:]) {
	case "K":
		mult = 1 << 10
	case "M":
		mult = 1 << 20
	case "G":
		mult = 1 << 30
	case "T":
		mult = 1 << 40
	}
	if mult > 1 {
		num = s[:len(s)-1]
	}

	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 0 || n > math.MaxInt64/mult {
		return 0, fmt.Errorf("invalid size %q: use a number of bytes with an optional K, M, G, or T suffix", s)
	}

	n *= mult
	if n%512 != 0 {
		return 0, fmt.Errorf("invalid size %q: the size must be a multiple of 512 bytes", s)
	}

	return n, nil
}

func validFCScratch(_ VMConfig, s string) error {
	_, err := fcParseSize(s)
	return err
}

// fcCheckDiskOptions returns an error when disks use an option that VMs of
// type t do not support.
func fcCheckDiskOptions(t VMType, disks DiskConfigs) error {
	if t == FIRECRACKER {
		return nil
	}

	for _, d := range disks {
		if d.ReadOnly {
			return errors.New("disk option ro is supported only for firecracker VMs")
		}
	}

	return nil
}

func fcFullCopy(src, dst string) error {
	fcLaunchSem.Acquire()
	defer fcLaunchSem.Release()

	return fcSparseCopy(src, dst)
}

// fcCommitOverlay writes the changes in overlay into the raw file dst.
func fcCommitOverlay(overlay, dst, instanceDir string, index int) error {
	ov := filepath.Join(instanceDir, fmt.Sprintf("disk-%d.overlay.qcow2", index))
	defer os.Remove(ov)

	fcLaunchSem.Acquire()
	defer fcLaunchSem.Release()

	if err := fcSparseCopy(overlay, ov); err != nil {
		return err
	}

	for _, args := range [][]string{
		{"qemu-img", "rebase", "-u", "-F", "raw", "-b", dst, ov},
		{"qemu-img", "commit", ov},
	} {
		if out, err := fcRunCmd(args...); err != nil {
			return fmt.Errorf("%v %v %v: %v: %v", args[0], args[1], overlay, strings.TrimSpace(out), err)
		}
	}

	return nil
}

// fcSnapshotDisk gives d a device-mapper snapshot of its cache entry, with a
// sparse copy-on-write file in the instance directory.
func fcSnapshotDisk(d *fcDisk, instanceDir string, index int) error {
	cached := d.HostPath
	d.HostPath = ""

	fi, err := os.Stat(cached)
	if err != nil {
		return err
	}
	size := fi.Size()

	baseDev, err := fcCache.attachBase(d.key, cached)
	if err != nil {
		return err
	}
	d.baseAttached = true

	// Leave room for the snapshot metadata.
	cow := filepath.Join(instanceDir, fmt.Sprintf("disk-%d.cow", index))
	f, err := os.OpenFile(cow, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create %v: %v", cow, err)
	}
	if err := f.Truncate(size + size/128 + 1<<20); err != nil {
		f.Close()
		return fmt.Errorf("create %v: %v", cow, err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	out, err := fcRunCmd("losetup", "--find", "--show", cow)
	if err != nil {
		return fmt.Errorf("attach %v: %v: %v", cow, strings.TrimSpace(out), err)
	}
	d.cowFile = cow
	d.cowLoop = strings.TrimSpace(out)

	name := fcDMName(d.vmID, index)
	if out, err := fcRunCmd("dmsetup", "create", name, "--table", fcDMSnapshotTable(size, baseDev, d.cowLoop)); err != nil {
		return fmt.Errorf("dmsetup create %v: %v: %v", name, strings.TrimSpace(out), err)
	}
	d.dmName = name

	p, err := fcDMNode(name)
	if err != nil {
		return err
	}
	d.HostPath = p
	d.IsBlock = true

	return nil
}

// fcReleaseDisk removes the host objects of d: the device-mapper snapshot, its
// loop devices, its use of the cache entry, and the ACL entry. The instance
// directory cleanup removes the files.
func fcReleaseDisk(d fcDisk) error {
	var errs []error

	run := func(args ...string) {
		if out, err := fcRunCmd(args...); err != nil {
			errs = append(errs, fmt.Errorf("%v: %v: %v", strings.Join(args, " "), strings.TrimSpace(out), err))
		}
	}

	if d.dmName != "" {
		run("dmsetup", "remove", d.dmName)
	}
	if d.cowLoop != "" {
		run("losetup", "-d", d.cowLoop)
	}
	if d.baseAttached {
		if err := fcCache.releaseBase(d.key); err != nil {
			errs = append(errs, err)
		}
	}
	if d.key != "" {
		fcCache.removeUser(d.key, d.vmID)
	}
	if d.aclPath != "" {
		run("setfacl", "-x", fmt.Sprintf("u:%d", d.aclUID), d.aclPath)
	}

	return errors.Join(errs...)
}

// fcCacheEntry is one entry of the cache, for disk cache.
type fcCacheEntry struct {
	Key      string
	Source   string
	Format   string
	Size     int64
	Created  time.Time
	InUse    int
	Attached bool
}

// fcCacheList returns the cache entries, sorted by key.
func fcCacheList() ([]fcCacheEntry, error) {
	fcCache.mu.Lock()
	defer fcCache.mu.Unlock()

	return fcCache.list(fcCacheDir())
}

// LOCK: Assumes c.mu is held.
func (c *fcCacheState) list(dir string) ([]fcCacheEntry, error) {
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var res []fcCacheEntry
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".raw") || strings.Contains(name, ".tmp-") || f.IsDir() {
			continue
		}

		fi, err := f.Info()
		if err != nil {
			continue
		}

		e := fcCacheEntry{
			Key:     strings.TrimSuffix(name, ".raw"),
			Size:    fi.Size(),
			Created: fi.ModTime(),
		}

		if data, err := os.ReadFile(filepath.Join(dir, e.Key+".json")); err == nil {
			var meta fcCacheMeta
			if err := json.Unmarshal(data, &meta); err == nil {
				e.Source, e.Format, e.Created = meta.Source, meta.Format, meta.Created
			}
		}

		e.InUse = len(c.users[e.Key])
		_, e.Attached = c.bases[e.Key]

		res = append(res, e)
	}

	sort.Slice(res, func(i, j int) bool { return res[i].Key < res[j].Key })

	return res, nil
}

// fcCacheClear removes the cache entries that no VM uses and that have no
// loop device. It returns the number of entries that it removed.
func fcCacheClear() (int, error) {
	fcCache.mu.Lock()
	defer fcCache.mu.Unlock()

	dir := fcCacheDir()

	entries, err := fcCache.list(dir)
	if err != nil {
		return 0, err
	}

	var n int
	var errs []error
	for _, e := range entries {
		if e.InUse > 0 || e.Attached || fcCache.inflight[e.Key] != nil {
			continue
		}

		if err := os.Remove(filepath.Join(dir, e.Key+".raw")); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Key+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		n++
	}

	if err := fcCache.removeOrphans(dir); err != nil {
		errs = append(errs, err)
	}

	return n, errors.Join(errs...)
}

// removeOrphans removes the temporary files of interrupted cache fills, and
// each <key>.json that has no <key>.raw, when no fill of the key runs.
//
// LOCK: Assumes c.mu is held.
func (c *fcCacheState) removeOrphans(dir string) error {
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	var errs []error
	for _, f := range files {
		name := f.Name()
		key, _, _ := strings.Cut(name, ".")
		if f.IsDir() || c.inflight[key] != nil {
			continue
		}

		orphan := strings.Contains(name, ".tmp-")
		if name == key+".json" {
			_, err := os.Lstat(filepath.Join(dir, key+".raw"))
			orphan = errors.Is(err, os.ErrNotExist)
		}
		if !orphan {
			continue
		}

		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
