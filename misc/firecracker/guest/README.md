# Minimal Firecracker guest

These files make a small Linux guest for minimega Firecracker VMs. The guest
has a static BusyBox, an init script, and a static miniccc. All VMs boot from
one shared read-only root image.

| File | Use |
|---|---|
| `build-rootfs.sh` | Builds `rootfs.squashfs` and `rootfs.ext4` from the files below. |
| `init` | PID 1 of the guest (BusyBox `sh`). |
| `udhcpc.script` | The event script that applies a DHCP lease. |
| `kernel/minimal-6.18.fragment` | Kconfig fragment for a small 6.18 guest kernel. |
| `build-kernel.sh` | Builds `vmlinux` from Firecracker's 6.18 guest config and the fragment. |

## Build the root image

```bash
misc/firecracker/guest/build-rootfs.sh out/
```

The script needs Docker. It runs an Alpine container (`ALPINE_IMAGE`, default
`alpine:3.22.2`, pinned by digest) and installs `busybox-static`, `squashfs-tools`, and
`e2fsprogs` in it. On Alpine Linux as root, it uses the host instead.

Options:

- `--miniccc <path>` uses this miniccc binary. Without it, the script builds a
  static miniccc from this repository with `go build` and `CGO_ENABLED=0`.
- `--busybox <path>` uses this static BusyBox binary instead of the Alpine
  package.

The script stops when BusyBox or miniccc is not a static binary. It writes
`rootfs.squashfs` (`SQUASHFS_COMP`, default `zstd`) and `rootfs.ext4` to the
output directory, and prints their sizes. The ext4 image has no journal and is
for read-only use.

## The init script

`init` does these steps:

1. It mounts `/proc`, `/sys`, `/dev`, and `/dev/pts`.
2. It mounts a 16 MiB tmpfs on `/tmp`, `/run`, and `/var`. The root stays
   read-only.
3. It sets `/proc/sys/kernel/ctrl-alt-del` to 0. Ctrl-Alt-Del then sends
   SIGINT to PID 1.
4. It sets the host name and the network from the kernel command line.
5. It starts miniccc again each time miniccc stops.
6. It prints `init: ready` on the console.
7. It reaps orphan processes.

On SIGINT or SIGTERM, `init` stops all processes, runs `sync`, and runs
`reboot -f`. The Firecracker VMM then stops with exit code 0. `vm shutdown`
uses this path.

`init` reads these kernel command line parameters:

| Parameter | Use |
|---|---|
| `mm.hostname=<name>` | Host name of the guest. |
| `mm.ip=<address/prefix>` | Static address for `eth0`. |
| `mm.gw=<address>` | Default gateway for `mm.ip`. |
| `mm.dns=<address>` | Name server in `/run/resolv.conf`. |
| `mm.cc=<host:port>` | TCP address of the miniccc server. |

The kernel `ip=` parameter also sets the address of `eth0`. When `eth0` has
no address from `ip=` or `mm.ip`, `init` starts `udhcpc` in the background.

With `mm.cc`, miniccc connects over TCP. Without `mm.cc`, miniccc uses
`-family vsock -port 9002` when `/dev/vsock` exists. minimega listens on vsock
port 9002 for each VM with `vm config backchannel true`.

## Build the kernel

```bash
misc/firecracker/guest/build-kernel.sh ~/src/linux-6.18 out/ \
  --fc-config firecracker/resources/guest_configs/microvm-kernel-ci-x86_64-6.18.config
```

The script copies the Firecracker config into the kernel tree and merges the
fragment with `scripts/kconfig/merge_config.sh`. Then it runs
`make olddefconfig`. When `olddefconfig` drops a fragment option, the script
prints the option and stops. Otherwise it builds `vmlinux` and writes
`vmlinux` and `config` to the output directory. Set `JOBS` to change the
number of build jobs.

The fragment removes module support, IPv6, netfilter, BPF, audit, debug
information, and memory hotplug. It keeps virtio-net, virtio-blk, vsock, the
balloon driver, VMGenID, the virtio random number generator, the serial
console, `ip=`, squashfs, ext4, overlayfs, tmpfs, and devtmpfs.

> [!IMPORTANT]
> The Firecracker CI kernel does not have the i8042 and AT keyboard drivers.
> `vm shutdown` sends Ctrl-Alt-Del through the i8042 controller, so it needs
> this kernel.

## Boot the guest

```text
vm config kernel /path/to/vmlinux
vm config append root=/dev/vda ro rootfstype=squashfs quiet i8042.noaux i8042.nomux i8042.nopnp i8042.dumbkbd
vm config disk /path/to/rootfs.squashfs,ro
vm config firecracker-scratch 64M
vm config memory 64
vm launch firecracker guest[1-10]
vm start all
```

- For the ext4 image, use `rootfstype=ext4` and `rootfs.ext4,ro`.
- The `i8042` parameters make the keyboard controller probe shorter.
- Use 64 to 96 MiB of memory. With this kernel, the guest uses about 28 MiB
  after boot.
- The `ro` disk option attaches the image read-only. minimega copies the
  image into its disk cache one time. Then it hard-links the cached copy into
  the jail of each VM.
- The host page cache holds one copy of the image for all VMs. minimega makes
  no copy, loop device, or device-mapper target for each VM. The `snapshot`
  setting does not apply to a disk with `ro`.
- The `firecracker-scratch` line is optional. It adds an empty writable disk,
  `/dev/vdb` here, after the other disks. Use it only for a VM that must keep
  data, for example as an overlayfs upper layer.
- minimega creates the scratch disk as a sparse file at the first launch. The
  file uses only the blocks that the guest writes. `vm reset-disk` makes the
  scratch disk empty again.
- `vm config inject` needs a writable disk 0. Do not use it with a `ro` disk 0.
- minimega reads the Firecracker log through a named pipe. It keeps at most
  4 MiB of it in `firecracker.log` in the VM instance directory.
- minimega does not configure Firecracker metrics, so Firecracker writes no
  metrics file.
