package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	backing, err := backingName(dst, src, *f_absSnapshot)
	if err != nil {
		return fmt.Errorf("[image %s] error getting src backing image relative to dst: %v", src, err)
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

		parent, err := backingName(image, backing, *f_absSnapshot)
		if err != nil {
			return fmt.Errorf("[image %s] error getting parent backing relative to dst: %v", backing, err)
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

// backingName returns the backing file name to write in image for the backing
// file backing. The name is relative to the directory of image unless abs is
// true.
func backingName(image, backing string, abs bool) (string, error) {
	if abs {
		return backing, nil
	}

	rel, err := filepath.Rel(filepath.Dir(image), backing)
	if err == nil {
		if i := strings.IndexAny(rel, ":/"); i >= 0 && rel[i] == ':' {
			// QEMU reads a name with a colon before the first slash as a protocol
			rel = "./" + rel
		}
	}

	return rel, err
}

// savedDiskBacking returns the path of the saved disk image and the path of
// its backing file to give to diskRebase, and whether the stored backing name
// is different from the name that diskRebase writes. For a drive-backup
// target, QEMU writes the backing name that the VM overlay resolves to, such as
// "<base>/<id>/../files/x.qcow2". That name does not open after "vm flush"
// removes the instance directory. The clean paths are used when they identify
// the same files as QEMU does. If a symlink makes them different, the paths
// with resolved symlinks are used.
func savedDiskBacking(image, stored string, abs bool) (string, string, bool, error) {
	// as in QEMU, a ":" before the first "/" makes the name a protocol or
	// "json:" name and not a path, so leave it as it is
	if i := strings.IndexAny(stored, ":/"); stored == "" || (i >= 0 && stored[i] == ':') {
		return "", "", false, nil
	}

	realImage, err := realPath(image)
	if err != nil {
		return "", "", false, err
	}
	dir := filepath.Dir(realImage)

	target, err := realPath(joinBacking(dir, stored))
	if err != nil {
		return "", "", false, err
	}

	cleanImage, err := filepath.Abs(image)
	if err != nil {
		return "", "", false, err
	}
	backing := filepath.Clean(joinBacking(filepath.Dir(cleanImage), stored))

	name, err := backingName(cleanImage, backing, abs)
	if err == nil {
		gotImage, err1 := realPath(cleanImage)
		gotBacking, err2 := realPath(joinBacking(dir, name))
		if err1 == nil && err2 == nil && gotImage == realImage && gotBacking == target {
			return cleanImage, backing, name != stored, nil
		}
	}

	name, err = backingName(realImage, target, abs)
	if err != nil {
		return "", "", false, err
	}

	return realImage, target, name != stored, nil
}

// joinBacking returns the path that QEMU opens for the backing file name of an
// image in dir. QEMU does not clean the path.
func joinBacking(dir, name string) string {
	if filepath.IsAbs(name) {
		return name
	}

	return dir + string(filepath.Separator) + name
}

// realPath returns p as an absolute path whose directory has no symlinks. It
// does not clean p first because, as in the OS, a ".." after a symlink goes to
// the parent of the symlink target. The last element of p does not change.
func realPath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		p = wd + string(filepath.Separator) + p
	}

	i := strings.LastIndexByte(p, filepath.Separator)
	dir, err := filepath.EvalSymlinks(p[:i+1])
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, p[i+1:]), nil
}

// fixSavedDiskBacking rebases the saved disk image, without a data copy, so
// that its backing file name does not use the VM instance directory.
func fixSavedDiskBacking(image string) error {
	info, err := diskInfo(image)
	if err != nil {
		return fmt.Errorf("saved disk %v: %w", image, err)
	}

	path, backing, rebase, err := savedDiskBacking(image, info.BackingFile, *f_absSnapshot)
	if err != nil {
		return fmt.Errorf("saved disk %v: %w", image, err)
	}
	if !rebase {
		return nil
	}

	log.Debug("rebasing saved disk %v on %v", path, backing)

	if err := diskRebase(path, backing, true); err != nil {
		return fmt.Errorf("saved disk %v: %w", path, err)
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

// diskInject injects files into or deletes files from a disk image.
// dst/partition specify the image and the partition number. for injecting
// files, pairs is the dst/src filepaths. for deleting files, paths is the
// comma-separated list of filepaths to delete. options can be used to supply
// mount arguments.
func diskInject(dst, partition string, pairs map[string]string, options []string, delete bool, paths []string) error {
	// Load nbd
	if err := nbd.Modprobe(); err != nil {
		return err
	}

	// create a tmp mount point
	mntDir, err := os.MkdirTemp(*f_base, "dstImg")
	if err != nil {
		return err
	}
	log.Debug("temporary mount point: %v", mntDir)
	defer func() {
		if err := os.Remove(mntDir); err != nil {
			log.Error("rm mount dir failed: %v", err)
		}
	}()

	nbdPath, err := nbd.ConnectImage(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := nbd.DisconnectDevice(nbdPath); err != nil {
			log.Error("nbd disconnect failed: %v", err)
		}
	}()

	path := nbdPath

	f, err := os.Open(nbdPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// decide whether to mount partition or raw disk
	if partition != "none" {
		// keep rereading partitions and waiting for them to show up for a bit
		timeoutTime := time.Now().Add(10 * time.Second)
		for i := 1; ; i++ {
			if time.Now().After(timeoutTime) {
				return fmt.Errorf("[image %s] no partitions found on image", dst)
			}

			// tell kernel to reread partitions
			syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), C.BLKRRPART, 0)

			_, err = os.Stat(nbdPath + "p1")
			if err == nil {
				log.Debug("partitions detected after %d attempt(s)", i)
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		// default to first partition if there is only one partition
		if partition == "" {
			_, err = os.Stat(nbdPath + "p2")
			if err == nil {
				return fmt.Errorf("[image %s] please specify a partition; multiple found", dst)
			}

			partition = "1"
		}

		path = nbdPath + "p" + partition

		// check desired partition exists
		for i := 1; i <= 5; i++ {
			_, err = os.Stat(path)
			if err != nil {
				err = fmt.Errorf("[image %s] desired partition %s not found", dst, partition)

				time.Sleep(time.Duration(i*100) * time.Millisecond)
				continue
			}

			log.Info("desired partition %s found in image %s", partition, dst)
			break
		}

		if err != nil {
			return err
		}
	}

	// we use mount(8), because the mount syscall (mount(2)) requires we
	// populate the fstype field, which we don't know
	args := []string{"mount"}
	if len(options) != 0 {
		args = append(args, options...)
		args = append(args, path, mntDir)
	} else {
		args = []string{"mount", "-w", path, mntDir}
	}
	log.Debug("mount args: %v", args)

	var out string
	mounted := false
	for i := 1; i <= 5; i++ {
		out, err = processWrapper(args...)
		if err == nil {
			mounted = true
			break
		}
		log.Debug("mount attempt %d failed: %v", i, err)
		time.Sleep(time.Duration(i*100) * time.Millisecond)
	}

	if !mounted {
		log.Debug("standard mount failed, trying ntfs-3g: %v, %v", out, err)
		// check that ntfs-3g is installed
		out, err := processWrapper("ntfs-3g", "--version")
		if err != nil {
			log.Error("ntfs-3g not found, ntfs images unwriteable")
			return fmt.Errorf("[image %s] ntfs-3g not found %v: %v", dst, out, err)
		}

		// mount with ntfs-3g
		out, err = processWrapper("mount", "-o", "ntfs-3g", path, mntDir)
		if err != nil {
			log.Error("failed to mount partition")
			return fmt.Errorf("[image %s] %v: %v", dst, out, err)
		}
	}
	defer func() {
		if err := syscall.Unmount(mntDir, 0); err != nil {
			log.Error("unmount failed: %v", err)
		}
	}()

	if delete {
		// delete the file paths from mntDir.
		for _, path := range paths {
			mntPath := filepath.Join(mntDir, path)
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
			dir := filepath.Dir(filepath.Join(mntDir, target))
			os.MkdirAll(dir, 0o775)

			out, err := processWrapper("cp", "-fr", source, filepath.Join(mntDir, target))
			if err != nil {
				return fmt.Errorf("[image %s] %v: %v", dst, out, err)
			}
		}
	}

	// explicitly flush buffers
	out, err = processWrapper("blockdev", "--flushbufs", path)
	if err != nil {
		return fmt.Errorf("[image %s] unable to flush: %v %v", dst, out, err)
	}

	return nil
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
