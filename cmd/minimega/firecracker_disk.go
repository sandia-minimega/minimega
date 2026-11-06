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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
}

func (s *fcSemaphore) Acquire() {
	s.once.Do(func() {
		n := *f_firecrackerLaunchParallel
		if n <= 0 {
			n = runtime.NumCPU()
		}
		s.ch = make(chan struct{}, n)
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

// fcDiskChain returns the backing chain of src, src first.
func fcDiskChain(src string) ([]fcImage, error) {
	out, err := fcRunCmd("qemu-img", "info", "--output=json", "--backing-chain", src)
	if err != nil {
		return nil, fmt.Errorf("qemu-img info %v: %v: %v", src, strings.TrimSpace(out), err)
	}

	return fcParseChain(src, out)
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
}

func newFCCacheState() *fcCacheState {
	return &fcCacheState{
		inflight: map[string]*fcInflight{},
		users:    map[string]map[int]int{},
		bases:    map[string]*fcBaseLoop{},
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

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if err := fcCopyData(in, out, size); err != nil {
		out.Close()
		return fmt.Errorf("copy %v to %v: %v", src, dst, err)
	}
	if err := out.Truncate(size); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

func fcCopyData(in, out *os.File, size int64) error {
	buf := make([]byte, 64<<10)

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

			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
		}

		off = end
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
	CacheType string

	vmID         int
	key          string
	baseAttached bool
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

func fcCacheType(cache string) string {
	if cache == "unsafe" {
		return "Unsafe"
	}
	return "Writeback"
}

// fcPrepareDisk builds disk index of VM vmID from dc in instanceDir. The VMM
// runs as uid.
func fcPrepareDisk(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (d fcDisk, err error) {
	d = fcDisk{CacheType: fcCacheType(dc.Cache), vmID: vmID}

	if dc.Interface != "" && dc.Interface != "virtio" {
		log.Warn("firecracker always uses virtio-blk: ignoring interface %v of disk %v", dc.Interface, dc.Path)
	}

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
	d.cowLoop = strings.TrimSpace(out)

	name := fcDMName(d.vmID, index)
	if out, err := fcRunCmd("dmsetup", "create", name, "--table", fcDMSnapshotTable(size, baseDev, d.cowLoop)); err != nil {
		return fmt.Errorf("dmsetup create %v: %v: %v", name, strings.TrimSpace(out), err)
	}
	d.dmName = name

	d.HostPath = filepath.Join("/dev/mapper", name)
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

	return n, errors.Join(errs...)
}
