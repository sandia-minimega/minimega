package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/nbd"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// #include "linux/fs.h"
import "C"

type DiskInfo struct {
	Name        string
	Format      string
	VirtualSize int64
	DiskSize    int64
	BackingFile string
	InUse       bool
}

// diskInfo return information about the disk.
func diskInfo(image string) (DiskInfo, error) {
	info := DiskInfo{}

	out, err := processWrapper("qemu-img", "info", image, "--output=json")
	if err != nil {
		return info, fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	jsonOut := map[string]interface{}{}
	err = json.Unmarshal([]byte(out), &jsonOut)
	if err != nil {
		return info, fmt.Errorf("[image %s] %v", image, err)
	}

	info, err = parseQemuImg(jsonOut)
	if err != nil {
		return info, fmt.Errorf("[image %s] %v", image, err)
	}
	info.InUse, err = checkDiskInUse(image)
	if err != nil {
		return info, fmt.Errorf("could not check if image in use: %w", err)
	}

	return info, nil
}

// diskChainInfo returns info about this disk and all backing disks
func diskChainInfo(image string) ([]DiskInfo, error) {
	infos := []DiskInfo{}

	out, err := processWrapper("qemu-img", "info", image, "--output=json", "--backing-chain")
	if err != nil {
		// qemu-img returns nothing if it has an error reading a backing image.
		// Instead log error and get details for just this image
		if strings.Contains(out, "Could not open") && !strings.Contains(out, image) {
			log.Warn("%s", fmt.Sprintf("[image %s] returning just image details. Error getting backing image details: %v",
				image, out))
			single, err2 := diskInfo(image)
			if err2 != nil {
				return infos, err2
			}
			infos = append(infos, single)
			return infos, nil
		}
		return infos, fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	jsonOut := []map[string]interface{}{}
	err = json.Unmarshal([]byte(out), &jsonOut)

	if err != nil || len(jsonOut) == 0 {
		return infos, fmt.Errorf("[image %s] %v", image, err)
	}

	for _, d := range jsonOut {
		info, err := parseQemuImg(d)
		if err != nil {
			return infos, fmt.Errorf("[image %s] %v", image, err)
		}
		info.InUse, err = checkDiskInUse(image)
		if err != nil {
			return infos, fmt.Errorf("could not check if image in use: %w", err)
		}

		infos = append(infos, info)
	}

	return infos, nil
}

func parseQemuImg(j map[string]interface{}) (DiskInfo, error) {
	info := DiskInfo{}

	val, ok := j["filename"]
	if !ok {
		return info, fmt.Errorf("missing key 'filename'")
	}
	info.Name = val.(string)

	val, ok = j["format"]
	if !ok {
		return info, fmt.Errorf("missing key 'format'")
	}
	info.Format = val.(string)

	val, ok = j["virtual-size"]
	if !ok {
		return info, fmt.Errorf("missing key 'virtual-size'")
	}
	info.VirtualSize = int64(val.(float64))

	val, ok = j["actual-size"]
	if !ok {
		return info, fmt.Errorf("missing key 'actual-size'")
	}
	info.DiskSize = int64(val.(float64))

	// may be absolute or relative depending on creation. Want which it is to be shown to user
	if backing, ok := j["backing-filename"]; ok {
		info.BackingFile = backing.(string)
	}

	return info, nil
}

func checkDiskInUse(path string) (bool, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return false, fmt.Errorf("error stating file: %w", err)
	}

	locks, err := os.ReadFile("/proc/locks")
	if err != nil {
		return false, fmt.Errorf("error reading /proc/locks: %w", err)
	}

	return strings.Contains(string(locks), strconv.FormatUint(stat.Ino, 10)), nil
}

// diskCreate creates a new disk image, dst, of given size/format.
func diskCreate(format, dst, size string) error {
	out, err := processWrapper("qemu-img", "create", "-f", format, dst, size)
	if err != nil {
		log.Error("diskCreate: %v", out)
		return err
	}
	return nil
}

// diskSnapshot creates a new image, dst, using src as the backing image.
func diskSnapshot(src, dst string) error {
	if !strings.HasPrefix(src, *f_iomBase) {
		log.Warn("minimega expects backing images to be in the files directory")
	}

	info, err := diskInfo(src)
	if err != nil {
		return fmt.Errorf("[image %s] error getting info: %v", src, err)
	}

	backing := src

	if !*f_absSnapshot {
		var err error

		backing, err = filepath.Rel(filepath.Dir(dst), src)
		if err != nil {
			return fmt.Errorf("[image %s] error getting src backing image relative to dst: %v", src, err)
		}
	}

	out, err := processWrapper("qemu-img", "create", "-f", "qcow2", "-b", backing, "-F", info.Format, dst)
	if err != nil {
		return fmt.Errorf("[image %s] %v: %v", src, out, err)
	}

	return nil
}

func diskCommit(image string) error {
	out, err := processWrapper("qemu-img", "commit", "-d", image)
	if err != nil {
		return fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	return nil
}

func diskRebase(image, backing string, unsafe bool) error {
	args := []string{"qemu-img", "rebase"}

	if backing != "" {
		if !strings.HasPrefix(backing, *f_iomBase) {
			log.Warn("minimega expects backing images to be in the files directory")
		}

		parent := backing

		if !*f_absSnapshot {
			var err error

			parent, err = filepath.Rel(filepath.Dir(image), backing)
			if err != nil {
				return fmt.Errorf("[image %s] error getting parent backing relative to dst: %v", backing, err)
			}
		}

		backingInfo, err := diskInfo(backing)
		if err != nil {
			return fmt.Errorf("[image %s] error getting info for backing file: %v", image, err)
		}

		args = append(args, "-b", parent, "-F", backingInfo.Format)
	} else { // rebase as independent image
		args = append(args, "-b", "")
	}

	if unsafe {
		args = append(args, "-u")
	}

	args = append(args, image)

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	return nil
}

func diskResize(image, size string) error {
	out, err := processWrapper("qemu-img", "resize", "--shrink", image, size)
	if err != nil {
		return fmt.Errorf("[image %s] %v: %v", image, out, err)
	}

	return nil
}

// mountedImage tracks the state of an image that has been connected via nbd
// and mounted onto the host filesystem. Close() undoes the mount/connect, in
// reverse order, and removes the temporary mount point.
type mountedImage struct {
	image   string
	nbdPath string
	device  string // device (or partition) that was mounted
	mntDir  string
	mounted bool
}

// mountImage loads nbd, connects image, waits for/selects the desired
// partition, and mounts it at a newly created temporary directory using the
// provided mount args. If readOnly is set, the image is exported read-only
// over nbd so that the mount cannot modify it. mountArgsFn is called with the
// device path to mount and should return one or more candidate sets of
// `mount` arguments; each candidate is tried in turn (allowing callers to fall
// back to alternative mount options, e.g. to work around mild filesystem
// corruption). Callers must call Close() on the returned mountedImage once
// done.
func mountImage(image, partition string, readOnly bool, mountArgsFn func(path string) [][]string) (*mountedImage, error) {
	// Load nbd
	if err := nbd.Modprobe(); err != nil {
		return nil, err
	}

	// create a tmp mount point
	mntDir, err := os.MkdirTemp(*f_base, "dstImg")
	if err != nil {
		return nil, err
	}
	log.Debug("temporary mount point: %v", mntDir)

	mi := &mountedImage{image: image, mntDir: mntDir}

	connect := nbd.ConnectImage
	if readOnly {
		connect = nbd.ConnectImageReadOnly
	}

	nbdPath, err := connect(image)
	if err != nil {
		os.Remove(mntDir)
		return nil, err
	}
	mi.nbdPath = nbdPath

	path := nbdPath

	f, err := os.Open(nbdPath)
	if err != nil {
		mi.Close()
		return nil, err
	}
	defer f.Close()

	// close the nbd device before Close() disconnects it
	fail := func(err error) (*mountedImage, error) {
		f.Close()
		mi.Close()
		return nil, err
	}

	// decide whether to mount partition or raw disk
	if partition != "none" {
		// keep rereading partitions and waiting for them to show up for a bit
		timeoutTime := time.Now().Add(10 * time.Second)
		for i := 1; ; i++ {
			if time.Now().After(timeoutTime) {
				return fail(fmt.Errorf("[image %s] no partitions found on image", image))
			}

			// tell kernel to reread partitions
			syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), C.BLKRRPART, 0)

			_, err = os.Stat(nbdPath + "p1")
			if err == nil {
				log.Info("partitions detected after %d attempt(s)", i)
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		// default to first partition if there is only one partition
		if partition == "" {
			_, err = os.Stat(nbdPath + "p2")
			if err == nil {
				return fail(fmt.Errorf("[image %s] please specify a partition; multiple found", image))
			}

			partition = "1"
		}

		path = nbdPath + "p" + partition

		// check desired partition exists
		for i := 1; i <= 5; i++ {
			_, err = os.Stat(path)
			if err != nil {
				err = fmt.Errorf("[image %s] desired partition %s not found", image, partition)

				time.Sleep(time.Duration(i*100) * time.Millisecond)
				continue
			}

			log.Info("desired partition %s found in image %s", partition, image)
			break
		}

		if err != nil {
			return fail(err)
		}
	}

	mi.device = path

	// we use mount(8), because the mount syscall (mount(2)) requires we
	// populate the fstype field, which we don't know
	var failures []string

	for _, args := range mountArgsFn(path) {
		args = append(args, mntDir)
		log.Debug("mount args: %v", args)

		var out string
		var mountErr error

		for i := 1; i <= 5; i++ {
			out, mountErr = processWrapper(args...)
			if mountErr == nil {
				mi.mounted = true
				return mi, nil
			}
			log.Debug("mount attempt %d failed: %v", i, mountErr)
			time.Sleep(time.Duration(i*100) * time.Millisecond)
		}

		log.Debug("mount attempt with args %v failed: %v, %v", args, out, mountErr)
		failures = append(failures, fmt.Sprintf("%v: %v: %v", args, out, mountErr))
	}

	return fail(fmt.Errorf("[image %s] unable to mount %s: %s", image, path, strings.Join(failures, "; ")))
}

// Close unmounts the filesystem (if mounted), disconnects the nbd device (if
// connected), and removes the temporary mount point. Errors are logged rather
// than returned since Close is expected to be deferred.
func (mi *mountedImage) Close() {
	if mi.mounted {
		if err := syscall.Unmount(mi.mntDir, 0); err != nil {
			log.Error("unmount failed: %v", err)
		}
	}

	if mi.nbdPath != "" {
		if err := nbd.DisconnectDevice(mi.nbdPath); err != nil {
			log.Error("nbd disconnect failed: %v", err)
		}
	}

	if err := os.Remove(mi.mntDir); err != nil {
		log.Error("rm mount dir failed: %v", err)
	}
}

// writableMountArgs returns candidate mount option sets for read-write
// mounting: the user-supplied options (or -w if there are none), followed by
// the ntfs-3g driver for NTFS images.
func writableMountArgs(options []string) func(path string) [][]string {
	return func(path string) [][]string {
		args := []string{"mount", "-w", path}
		if len(options) != 0 {
			args = append(append([]string{"mount"}, options...), path)
		}

		return [][]string{
			args,
			{"mount", "-t", "ntfs-3g", path},
		}
	}
}

// readOnlyMountArgs returns candidate mount option sets for read-only
// mounting. Each candidate starts with the user-supplied options (if any). The
// first candidate adds nothing else; the others add fallbacks intended to cope
// with mild filesystem corruption, such as skipping journal replay. Every
// candidate ends with -r so that user options cannot make the mount writable.
func readOnlyMountArgs(options []string) func(path string) [][]string {
	return func(path string) [][]string {
		fallbacks := [][]string{
			// normal read-only mount
			nil,
			// ext2/3/4: mount without replaying the journal, in case the
			// journal itself is corrupt or replaying it would otherwise fail
			{"-o", "noload"},
			// xfs: skip log recovery
			{"-o", "norecovery"},
		}

		// ntfs: force mount even if marked dirty/unclean. Skip it when the
		// user selected a filesystem type, since it would replace that type.
		if !hasMountType(options) {
			fallbacks = append(fallbacks, []string{"-t", "ntfs-3g", "-o", "force"})
		}

		var candidates [][]string
		for _, fallback := range fallbacks {
			args := append([]string{"mount"}, options...)
			args = append(args, fallback...)
			args = append(args, "-r", path)
			candidates = append(candidates, args)
		}

		return candidates
	}
}

// hasMountType returns true if the mount(8) options select a filesystem type.
func hasMountType(options []string) bool {
	for _, opt := range options {
		if strings.HasPrefix(opt, "-t") || opt == "--types" || strings.HasPrefix(opt, "--types=") {
			return true
		}
	}

	return false
}

// diskInject injects files into or deletes files from a disk image.
// dst/partition specify the image and the partition number. for injecting
// files, pairs is the dst/src filepaths. for deleting files, paths is the
// comma-separated list of filepaths to delete. options can be used to supply
// mount arguments.
func diskInject(dst, partition string, pairs map[string]string, options []string, delete bool, paths []string) error {
	mi, err := mountImage(dst, partition, false, writableMountArgs(options))
	if err != nil {
		return err
	}
	defer mi.Close()

	if delete {
		// delete the file paths from mntDir.
		for _, path := range paths {
			mntPath := filepath.Join(mi.mntDir, path)
			if _, err := os.Stat(mntPath); os.IsNotExist(err) {
				log.Warn("[image %s] path does not exist to delete: %v", dst, path)
			} else {
				err := os.RemoveAll(mntPath)
				if err != nil {
					return fmt.Errorf("[image %s] error deleting '%s': %v", dst, path, err)
				}
			}
		}
	} else {
		// copy files/folders into mntDir
		for target, source := range pairs {
			dir := filepath.Dir(filepath.Join(mi.mntDir, target))
			os.MkdirAll(dir, 0o775)

			out, err := processWrapper("cp", "-fr", source, filepath.Join(mi.mntDir, target))
			if err != nil {
				return fmt.Errorf("[image %s] %v: %v", dst, out, err)
			}
		}
	}

	// explicitly flush buffers
	out, err := processWrapper("blockdev", "--flushbufs", mi.device)
	if err != nil {
		return fmt.Errorf("[image %s] unable to flush: %v %v", dst, out, err)
	}

	return nil
}

// diskExtract extracts files/directories from a disk image onto the host
// filesystem. src/partition specify the image and the partition number.
// pairs maps a destination path on the host to the source path within the
// image. options can be used to supply mount arguments. The image is always
// mounted read-only; a handful of mount fallbacks are attempted to cope with
// mild filesystem corruption.
func diskExtract(src, partition string, pairs map[string]string, options []string) error {
	mi, err := mountImage(src, partition, true, readOnlyMountArgs(options))
	if err != nil {
		return err
	}
	defer mi.Close()

	dsts := make([]string, 0, len(pairs))
	for dst := range pairs {
		dsts = append(dsts, dst)
	}
	sort.Strings(dsts)

	// check every source before copying anything
	sources := map[string]string{}
	for _, dst := range dsts {
		source := pairs[dst]
		if source == "" {
			return fmt.Errorf("[image %s] empty source path for destination '%s'", src, dst)
		}

		mntPath, err := resolveInRoot(mi.mntDir, source)
		if err != nil {
			return fmt.Errorf("[image %s] error accessing '%s' in image: %v", src, source, err)
		}

		if _, err := os.Lstat(mntPath); err != nil {
			return fmt.Errorf("[image %s] error accessing '%s' in image: %v", src, source, err)
		}

		if err := checkExtractable(mi.mntDir, mntPath); err != nil {
			return fmt.Errorf("[image %s] cannot extract '%s': %v", src, source, err)
		}

		sources[dst] = mntPath
	}

	// copy files/folders out of mntDir
	for _, dst := range dsts {
		dir := filepath.Dir(dst)
		if err := os.MkdirAll(dir, 0o775); err != nil {
			return fmt.Errorf("[image %s] error creating destination directory '%s': %v", src, dir, err)
		}

		// -r copies symlinks as symlinks and does not follow them.
		// --remove-destination stops writes through existing host symlinks.
		out, err := processWrapper("cp", "-r", "--remove-destination", sources[dst], dst)
		if err != nil {
			return fmt.Errorf("[image %s] %v: %v", src, out, err)
		}
	}

	return nil
}

// maxSymlinks limits the number of symlinks that resolveInRoot follows.
const maxSymlinks = 255

// resolveInRoot resolves path as if root were the filesystem root and returns
// the resulting host path. It follows symlinks in all but the final path
// component, and in the final component too if path ends with a slash.
// Absolute symlink targets and ".." components cannot go above root, so the
// result is always within root.
func resolveInRoot(root, path string) (string, error) {
	resolved := "/"
	remaining := path
	links := 0

	for {
		var comp string
		var more bool
		comp, remaining, more = strings.Cut(strings.TrimLeft(remaining, "/"), "/")

		switch comp {
		case "":
			return filepath.Join(root, resolved), nil
		case ".":
			continue
		case "..":
			resolved = filepath.Dir(resolved)
			continue
		}

		next := filepath.Join(resolved, comp)

		if !more {
			return filepath.Join(root, next), nil
		}

		info, err := os.Lstat(filepath.Join(root, next))
		if err != nil {
			return "", err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			if !info.IsDir() {
				return "", fmt.Errorf("'%s' is not a directory", next)
			}

			resolved = next
			continue
		}

		links++
		if links > maxSymlinks {
			return "", fmt.Errorf("too many levels of symbolic links in '%s'", path)
		}

		target, err := os.Readlink(filepath.Join(root, next))
		if err != nil {
			return "", err
		}

		if filepath.IsAbs(target) {
			resolved = "/"
		}
		remaining = target + "/" + remaining
	}
}

// checkExtractable walks path, without following symlinks, and returns an
// error if it contains anything other than regular files, directories, and
// symlinks. This prevents extraction from recreating guest device nodes,
// FIFOs, or sockets on the host. Errors name entries relative to root.
func checkExtractable(root, path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		t := d.Type()
		if t.IsRegular() || t.IsDir() || t&fs.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}

		return fmt.Errorf("'/%s' is a special file (%v)", rel, t)
	})
}

// parseInjectPairs parses a list of strings containing src:dst pairs into a
// map of where the dst is the key and src is the value. We build the map this
// way so that one source file can be written to multiple destinations and so
// that we can detect and return an error if the user tries to inject two files
// with the same destination.
func parseInjectPairs(files []string) (map[string]string, error) {
	pairs := map[string]string{}

	// parse inject pairs
	for _, arg := range files {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			return nil, errors.New("malformed command; expected src:dst pairs")
		}

		if pairs[parts[1]] != "" {
			return nil, fmt.Errorf("destination appears twice: `%v`", parts[1])
		}

		pairs[parts[1]] = parts[0]
		log.Debug("inject pair: %v, %v", parts[0], parts[1])
	}

	return pairs, nil
}

// parseFiles parses the files argument passed into the 'disk inject ...'
// command. if options are used, the files argument gets turned into a
// minicli.Command StringArg, otherwise it gets turned into a ListArg.
// this function returns either a paths string slice if 'delete' is part
// of the original command, or a pairs string map.
func parseFiles(files interface{}, delete bool) (map[string]string, []string, error) {
	var pairs map[string]string
	var paths []string
	var err error
	switch v := files.(type) {
	case []string:
		if delete {
			if sliceContainsString(v, ",") {
				paths = strings.Split(v[0], ",")
			} else {
				paths = []string{v[0]} // single file
			}
		} else {
			pairs, err = parseInjectPairs(v)
		}
	case string:
		if delete {
			if strings.Contains(v, ",") {
				paths = strings.Split(v, ",")
			} else {
				paths = []string{v} // single file
			}
		} else {
			pairs, err = parseInjectPairs([]string{v})
		}
	default:
		return nil, nil, errors.New("error parsing files: unknown type")
	}

	return pairs, paths, err
}
