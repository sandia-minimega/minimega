package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// mountedImage is an image connected via nbd and mounted on the host.
type mountedImage struct {
	nbdPath string
	device  string // device (or partition) that was mounted
	mntDir  string
	mounted bool
}

// mountImage connects image via nbd and mounts the partition at a temporary
// directory, trying each candidate set of mount options. Callers must Close it.
func mountImage(image, partition string, readOnly bool, options []string) (_ *mountedImage, err error) {
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

	mi := &mountedImage{mntDir: mntDir}

	defer func() {
		if err != nil {
			mi.Close()
		}
	}()

	connect, candidates := nbd.ConnectImage, writableMountOptions(options)
	if readOnly {
		connect, candidates = nbd.ConnectImageReadOnly, readOnlyMountOptions(options)
	}

	mi.nbdPath, err = connect(image)
	if err != nil {
		return nil, err
	}

	mi.device, err = selectPartition(mi.nbdPath, partition)
	if err != nil {
		return nil, fmt.Errorf("[image %s] %w", image, err)
	}

	if err := mountFirst(mi.device, mntDir, candidates); err != nil {
		return nil, fmt.Errorf("[image %s] %w", image, err)
	}

	mi.mounted = true
	return mi, nil
}

// mountFirst mounts device at mntDir with the first candidate set of mount
// options that works.
func mountFirst(device, mntDir string, candidates [][]string) error {
	var errs []error
	for _, opts := range candidates {
		err := mountDevice(device, mntDir, opts)
		if err == nil {
			return nil
		}

		errs = append(errs, err)
	}

	return fmt.Errorf("unable to mount %s: %w", device, errors.Join(errs...))
}

// selectPartition returns the device to mount ("none" means the whole disk).
func selectPartition(nbdPath, partition string) (string, error) {
	// decide whether to mount partition or raw disk
	if partition == "none" {
		return nbdPath, nil
	}

	if err := waitForPartitions(nbdPath); err != nil {
		return "", err
	}

	// default to first partition if there is only one partition
	if partition == "" {
		if _, err := os.Stat(nbdPath + "p2"); err == nil {
			return "", errors.New("please specify a partition; multiple found")
		}

		partition = "1"
	}

	path := nbdPath + "p" + partition

	// check desired partition exists
	for i := 1; i <= 5; i++ {
		if _, err := os.Stat(path); err == nil {
			log.Info("desired partition %s found", partition)
			return path, nil
		}

		time.Sleep(time.Duration(i*100) * time.Millisecond)
	}

	return "", fmt.Errorf("desired partition %s not found", partition)
}

// waitForPartitions tells the kernel to read the partition table of nbdPath
// again until the first partition shows up, for at most 10 seconds.
func waitForPartitions(nbdPath string) error {
	f, err := os.Open(nbdPath)
	if err != nil {
		return err
	}
	defer f.Close()

	timeoutTime := time.Now().Add(10 * time.Second)
	for i := 1; ; i++ {
		if time.Now().After(timeoutTime) {
			return errors.New("no partitions found on image")
		}

		syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), C.BLKRRPART, 0)

		if _, err := os.Stat(nbdPath + "p1"); err == nil {
			log.Debug("partitions detected after %d attempt(s)", i)
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// mountDevice mounts device at mntDir, retrying a few times.
func mountDevice(device, mntDir string, options []string) error {
	// we use mount(8), because the mount syscall (mount(2)) requires we
	// populate the fstype field, which we don't know
	args := append([]string{"mount"}, options...)
	args = append(args, device, mntDir)
	log.Debug("mount args: %v", args)

	var out string
	var err error

	for i := 1; i <= 5; i++ {
		out, err = processWrapper(args...)
		if err == nil {
			return nil
		}

		log.Debug("mount attempt %d failed: %v", i, err)
		time.Sleep(time.Duration(i*100) * time.Millisecond)
	}

	return fmt.Errorf("%v: %v: %w", args, strings.TrimSpace(out), err)
}

// Close unmounts, disconnects nbd, and removes the mount point, logging errors.
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

// writableMountOptions returns the user options (or -w), then ntfs-3g.
func writableMountOptions(options []string) [][]string {
	if len(options) == 0 {
		options = []string{"-w"}
	}

	return [][]string{
		options,
		{"-t", "ntfs-3g"},
	}
}

// readOnlyMountOptions returns the user options plus recovery fallbacks, each
// ending with -r so user options cannot make the mount writable.
func readOnlyMountOptions(options []string) [][]string {
	fallbacks := [][]string{
		nil,
		{"-o", "noload"},     // ext2/3/4: skip journal replay
		{"-o", "norecovery"}, // xfs: skip log recovery
	}

	// ntfs: force mount even if dirty, unless the user picked a type
	if !hasMountType(options) {
		fallbacks = append(fallbacks, []string{"-t", "ntfs-3g", "-o", "force"})
	}

	var candidates [][]string
	for _, fallback := range fallbacks {
		candidates = append(candidates, slices.Concat(options, fallback, []string{"-r"}))
	}

	return candidates
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
	mi, err := mountImage(dst, partition, false, options)
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

// diskExtract copies files out of image src; pairs maps host dst to image src.
func diskExtract(src, partition string, pairs map[string]string, options []string) error {
	mi, err := mountImage(src, partition, true, options)
	if err != nil {
		return err
	}
	defer mi.Close()

	if err := extractFiles(mi.mntDir, pairs); err != nil {
		return fmt.Errorf("[image %s] %w", src, err)
	}

	return nil
}

// extractFiles copies pairs out of root, checking every source before copying any.
func extractFiles(root string, pairs map[string]string) error {
	dsts := slices.Sorted(maps.Keys(pairs))

	sources := map[string]string{}
	for _, dst := range dsts {
		if pairs[dst] == "" {
			return fmt.Errorf("empty source path for destination '%s'", dst)
		}

		mntPath, err := extractSource(root, pairs[dst])
		if err != nil {
			return err
		}

		sources[dst] = mntPath
	}

	for _, dst := range dsts {
		dir := filepath.Dir(dst)
		if err := os.MkdirAll(dir, 0o775); err != nil {
			return fmt.Errorf("error creating destination directory '%s': %w", dir, err)
		}

		// -r keeps links; --remove-destination avoids writing through host links
		out, err := processWrapper("cp", "-r", "--remove-destination", sources[dst], dst)
		if err != nil {
			return fmt.Errorf("%v: %w", strings.TrimSpace(out), err)
		}
	}

	return nil
}

// extractSource returns the host path of source in the image at root, if it
// exists and has no special files.
func extractSource(root, source string) (string, error) {
	mntPath, err := resolveInRoot(root, source)
	if err != nil {
		return "", fmt.Errorf("error accessing '%s' in image: %w", source, err)
	}

	if _, err := os.Lstat(mntPath); err != nil {
		return "", fmt.Errorf("error accessing '%s' in image: %w", source, err)
	}

	if err := checkExtractable(root, mntPath); err != nil {
		return "", fmt.Errorf("cannot extract '%s': %w", source, err)
	}

	return mntPath, nil
}

// maxSymlinks limits the symbolic links that resolveInRoot follows, so that a
// link loop fails.
const maxSymlinks = 255

// resolveInRoot resolves path with root as "/", so it cannot escape root.
// The final component is followed only if path ends with a slash.
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
			// filepath.Dir("/") is "/", so ".." cannot leave root
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

		// resolve the link target in place of the link; an absolute
		// target starts again at root
		if filepath.IsAbs(target) {
			resolved = "/"
		}
		remaining = target + "/" + remaining
	}
}

// checkExtractable rejects device files, FIFOs, and sockets under path.
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
