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
			log.Warn(fmt.Sprintf("[image %s] returning just image details. Error getting backing image details: %v",
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

	// If there is a backing file, than we should not try to create a new
	// snapshot. Rather we will log a message and copy that previous backing file
	// to the new destination. Both preserving the original backing file, and the
	// disk used with the backing file.
	if info.BackingFile != "" {
		// We first will ensure that the backing file exists
		if _, err := os.Stat(info.BackingFile); os.IsNotExist(err) {
			return fmt.Errorf("[image %s] backing file does not exist: %s", src, info.BackingFile)
		}

		// Now we can copy the backing file to the destination
		out, err := processWrapper("cp", src, dst)
		if err != nil {
			return fmt.Errorf("[image %s] error copying to destination: %v: %v", src, out, err)
		}
		log.Info("Copied existing snapshot file from %s to %s", src, dst)
		return nil
	}

	relSrc, err := filepath.Rel(filepath.Dir(dst), src)
	if err != nil {
		return fmt.Errorf("[image %s] error getting src relative to dst: %v", src, err)
	}

	out, err := processWrapper("qemu-img", "create", "-f", "qcow2", "-b", relSrc, "-F", info.Format, dst)

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
		relBacking, err := filepath.Rel(filepath.Dir(image), backing)
		if err != nil {
			return fmt.Errorf("[image %s] error getting backing relative to dst: %v", backing, err)
		}
		backingInfo, err := diskInfo(backing)
		if err != nil {
			return fmt.Errorf("[image %s] error getting info for backing file: %v", image, err)
		}
		args = append(args, "-b", relBacking, "-F", backingInfo.Format)
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
		timeoutTime := time.Now().Add(5 * time.Second)
		for i := 1; ; i++ {
			if time.Now().After(timeoutTime) {
				return fmt.Errorf("[image %s] no partitions found on image", dst)
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

	_, err = processWrapper(args...)
	if err != nil {
		// check that ntfs-3g is installed
		_, err = processWrapper("ntfs-3g", "--version")
		if err != nil {
			log.Error("ntfs-3g not found, ntfs images unwriteable")
		}

		// mount with ntfs-3g
		out, err := processWrapper("mount", "-o", "ntfs-3g", path, mntDir)
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
			os.MkdirAll(dir, 0775)

			out, err := processWrapper("cp", "-fr", source, filepath.Join(mntDir, target))
			if err != nil {
				return fmt.Errorf("[image %s] %v: %v", dst, out, err)
			}
		}
	}

	// explicitly flush buffers
	out, err := processWrapper("blockdev", "--flushbufs", path)
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
