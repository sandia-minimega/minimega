# Building images with vmbetter

vmbetter builds minimal Debian-based guest images from a short configuration
file. One build produces whichever artifact you ask for: a kernel and initrd
pair (the default), a bootable disk image, a live ISO, or a plain root
filesystem directory for containers. This page explains which image type boots
which kind of VM, how a configuration file is put together, what every flag
does, and how to turn an existing Docker or LXC filesystem into a minimega
container.

You need vmbetter when you want a reproducible, scriptable guest that starts
fast and already contains miniccc, and you would rather not click through an
installer. If you need a full distribution or Windows installed from its own
media, see [Creating VMs from install media](newvm.md). To work with disk
images once they exist, see [Disk images and the disk API](disk-images.md).

## Image types and which VM they boot

minimega runs two kinds of Linux guest, and they need different artifacts.
[Virtual machine types](vmtypes.md) covers the VM types in full; the short
version is:

- **KVM** guests are full virtual machines with their own kernel. They boot
  from a disk image (`vm config disks`), a CD image (`vm config cdrom`), or a
  kernel and initrd pair (`vm config kernel` and `vm config initrd`). A kernel
  and initrd pair runs entirely from RAM, starts in a second or two, and never
  touches a disk, which makes it ideal for hundreds of identical, disposable
  nodes. A disk image is what you want when the guest must persist state, look
  like an installed machine, or hold more than fits comfortably in an initrd.
- **Container** guests share the host kernel. All they need is a root
  filesystem directory containing an `init` program (`vm config filesystem`).
  minimega creates the namespaces and cgroups itself, so no container engine
  runs on the host, and the same `vm config`, networking, `cc`, and capture
  commands apply to containers and KVM guests alike. Because there is no
  layered image format and no registry, a container image is just a directory
  tree, which vmbetter produces directly and which you can also extract from
  Docker or LXC (see [below](#container-root-filesystems-from-docker-or-lxc)).

vmbetter produces all of these from the same configuration, selected by a
flag:

| Flag | Output in the current directory | Boot it with |
|---|---|---|
| (none) | `<name>.kernel` and `<name>.initrd` | `vm config kernel` and `vm config initrd` |
| `-disk` | `<name>.qc2` (or the `-format` you chose) | `vm config disks` |
| `-iso` | `<name>.iso` | `vm config cdrom` |
| `-rootfs` | `<name>_rootfs/` directory | `vm config filesystem` |

`<name>` is the basename of the configuration file without its extension
unless you override it with `-O`.

For kernel and initrd builds the entire filesystem is packed into the initrd
and the kernel runs the overlay's `/init` as PID 1. Disk and ISO builds boot
through the distribution's own initramfs from `/boot`, so those images should
contain a normal init system; `minimal_ubuntu.conf` installs the `init` package
and regenerates the initramfs in its `postbuild` for exactly this reason.

## Installing vmbetter and its dependencies

The `.deb` and `.rpm` packages install vmbetter as `/opt/minimega/bin/vmbetter`
together with the example configurations in
`/opt/minimega/misc/vmbetter_configs/`. A source build (`scripts/build.bash` or
`scripts/all.bash`) puts it in `bin/vmbetter`. The Docker image contains the
binary but not the build toolchain below, so run vmbetter on a host.

vmbetter must run as root: it calls `debootstrap` and `chroot`, and disk builds
attach the image to an NBD device. It checks for every external tool at
start-up and logs the ones it cannot find. On Debian or Ubuntu:

```bash
$ sudo apt install debootstrap extlinux syslinux-common squashfs-tools genisoimage qemu-utils
```

Every build needs `debootstrap`, `chroot`, `cp`, and `bash`. Disk images also
need `qemu-img`, `qemu-nbd`, `sfdisk`, `mkfs.ext3`, `mount`, `dd`, `extlinux`,
and syslinux's `mbr.bin`. ISO images need `mksquashfs` and `genisoimage` plus
an isolinux directory (the repository ships one in `misc/isolinux/`).

## Configuration files

A configuration file is a list of `key = "value"` lines. Values are
double-quoted strings, or backtick-quoted raw strings when they span several
lines, and `//` starts a comment. There are exactly four keys:

| Key | Meaning |
|---|---|
| `parents` | Space-separated list of configuration files to include first. Parents are read recursively, so their packages, overlays, and postbuild commands apply before this file's own. A parent that is not found relative to the current directory is looked up relative to the file that names it. |
| `packages` | Space-separated Debian package names, passed to `debootstrap --include`. The key may appear any number of times; the lists are concatenated and debootstrap resolves dependencies. |
| `overlay` | A directory whose contents are copied over the root of the new filesystem after debootstrap finishes. A relative path is relative to the configuration file. Overlays apply parents first, so a child's files overwrite a parent's. Symlinks inside the overlay are followed when copying. |
| `postbuild` | A shell script run with `bash` inside a chroot of the new filesystem, with `/proc` and `/dev` mounted, after all overlays are in place. Parent scripts run first. |

A key can be guarded by a build-constraint comment in the style of Go source.
The key that follows applies only when every listed tag is present in the
`-constraints` flag (default `debian,amd64`); a `!tag` excludes it:

```text
// +build debian amd64
packages = "linux-headers-amd64 linux-image-amd64"

// +build ubuntu
packages = "linux-headers-generic linux-image-generic"
```

`vmbetter -dry-run <config>` prints the fully resolved configuration and exits,
which is the quickest way to see what a chain of parents produces.

### The shipped configurations

`misc/vmbetter_configs/` holds the configurations the project itself uses,
each with a matching `<name>_overlay/` directory:

- `default.conf` is the base almost everything inherits from: a kernel, a DHCP
  client, an SSH server, `vim`, and the usual network tools. Its `postbuild`
  allows password-less root logins on the console and over SSH and cleans the
  apt cache. Its overlay holds an `/init` script that mounts `/proc`,
  `/sys`, and `/dev`, starts udev, brings up `eth0` with DHCP, starts `sshd`,
  and opens a shell on the console, plus a minimal `/etc/ssh/sshd_config`
  that permits password-less root logins.
- `miniccc.conf` swaps in an `/init` that also loads the virtio and common
  network drivers, creates `/dev/virtio-ports/<name>` symlinks, and starts
  `/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /miniccc.log`. The
  overlay's `miniccc` is a symlink to `../../../bin/miniccc`, which vmbetter
  follows when it copies the overlay, so the binary comes from `bin/` of a
  source build (`scripts/build.bash`) or from the package's
  `/opt/minimega/bin/`. In a fresh clone that has not been built the link
  dangles and the build stops with a `cp` error.
- `minirouter.conf` adds `dnsmasq` and `bird` and starts `minirouter`, for use
  with the [router API](router.md). Its overlay links `miniccc` and
  `minirouter` into `bin/` in the same way.
- `miniccc_container.conf` and `minirouter_container.conf` are the container
  equivalents. Their `/init` brings up `veth0` instead of `eth0` and starts
  `miniccc -family unix -parent /cc`, because a container reaches minimega
  over a UNIX socket that minimega creates at `/cc` in the container's root.
  `minirouter_container_overlay` also carries a `preinit` script that enables
  IP forwarding, which has to happen before the container is isolated.
- `host.conf`, `ccc_host.conf`, `carnac_host.conf`, the `*_buildbot.conf`
  files, and `miniception.conf` build images for nodes that run minimega
  itself.
- `bro.conf` inherits `default.conf` directly and adds `miniccc_overlay` and
  its own `bro_overlay` to run the Bro network monitor (the package that
  preceded Zeek). The `bro` package is not in current Debian releases, so
  treat it as an example rather than a working build.
- `minimal_ubuntu.conf` shows how to build an Ubuntu disk image instead of a
  Debian initrd; see [Ubuntu builds](#ubuntu-builds) below.

### Writing your own

Put a new file next to the shipped ones and inherit from one of them. This
example adds two packages to the miniccc image and sets a message of the day:

```text title="misc/vmbetter_configs/sensor.conf"
parents = "miniccc.conf"

packages = "tshark iperf3"

overlay = "sensor_overlay"

postbuild = `
	echo "sensor built with vmbetter on $(date)" > /etc/motd
`
```

The overlay is an ordinary directory tree rooted at `/`. A file at
`sensor_overlay/etc/iperf3.conf` lands at `/etc/iperf3.conf` in the image. To
change what runs at boot, copy `miniccc_overlay/init` into `sensor_overlay/`
and edit it; because child overlays are copied last, your `init` replaces the
parent's. Keep `init` executable, since it becomes PID 1.

## Building

Run vmbetter from the repository root (or the package's `/opt/minimega`) so
that relative overlay paths and the default `-isolinux` directory resolve:

```bash
$ cd /opt/minimega
$ sudo bin/vmbetter -level info -branch bookworm misc/vmbetter_configs/miniccc.conf
```

This writes `miniccc.kernel` and `miniccc.initrd` to the current directory.
Copy them into minimega's files directory so they can be fetched across a
cluster, then boot them:

```minimega
minimega$ vm config kernel /tmp/minimega/files/miniccc.kernel
minimega$ vm config initrd /tmp/minimega/files/miniccc.initrd
minimega$ vm launch kvm node[1-4]
minimega$ vm start all
```

A disk image and a live ISO use the same configuration:

```bash
$ sudo bin/vmbetter -disk -size 4G -branch bookworm misc/vmbetter_configs/miniccc.conf
$ sudo bin/vmbetter -iso -branch bookworm misc/vmbetter_configs/miniccc.conf
```

### Flags

| Flag | Default | Effect |
|---|---|---|
| `-branch` | `testing` | Debian suite passed to debootstrap. Name a release (`bookworm`, `trixie`) or use `stable` for repeatable builds. |
| `-mirror` | `http://ftp.us.debian.org/debian` | Package mirror. |
| `-constraints` | `debian,amd64` | Comma-separated tags matched against `// +build` lines. |
| `-debootstrap-append` | (none) | Extra arguments placed before debootstrap's own, for example `"--components=main,universe"`. |
| `-disk` | off | Build a disk image instead of a kernel and initrd. |
| `-size` | `1G` | Disk image size, in `qemu-img` notation, when `-disk` is set. |
| `-format` | `qcow2` | Disk image format: `qcow`, `qcow2`, `raw`, or `vmdk`. |
| `-mbr` | `/usr/lib/syslinux/mbr/mbr.bin` | Master boot record written to the disk image. Override it if your syslinux package installs `mbr.bin` elsewhere. |
| `-iso` | off | Build a live ISO. The `live-boot` package is added automatically. |
| `-isolinux` | `misc/isolinux/` | Directory containing `isolinux.bin`, `ldlinux.c32`, and `isolinux.cfg`, relative to the current directory. |
| `-rootfs` | off | Copy the finished filesystem to `<name>_rootfs/` for use as a container filesystem. |
| `-O` | config basename | Output name. |
| `-1` | off | Stop after stage one and copy the build tree to `<name>_stage1/`. |
| `-2 <dir>` | (none) | Run stage two on an existing stage-one directory. Cannot be combined with `-1`. |
| `-noclean` | off | Keep the temporary build directory under `/tmp`. |
| `-dry-run` | off | Print the resolved configuration and exit. |
| `-level` | `error` | Log level: `debug`, `info`, `warn`, `error`, or `fatal`. `info` shows debootstrap's progress. |
| `-logfile` | (none) | Also write the log to a file. |
| `-v` | `true` | Log to standard error. |

### What a build does

1. Reads the configuration and its parents. With `-iso`, adds `live-boot`.
2. Runs `debootstrap --variant=minbase --include=<packages> <branch> <build dir> <mirror>`.
3. Copies each overlay into the build directory, parents first. This is the
   end of stage one; with `-1` the tree is copied to `<name>_stage1/` and
   vmbetter stops.
4. Runs each `postbuild` script in a chroot, parents first.
5. Produces the target. A kernel and initrd build packs the tree with `cpio`
   and `gzip` and copies `/boot/vmlinu*` out. A disk build creates the image
   with `qemu-img`, attaches it with `qemu-nbd`, writes one bootable ext3
   partition, copies the tree in, and installs `extlinux` with a
   `root=/dev/sda1` entry and the MBR. An ISO build squashes the tree with
   `mksquashfs` and runs `genisoimage` with the isolinux files. A rootfs build
   copies the tree to `<name>_rootfs/`.
6. Removes the temporary build directory unless `-noclean` was given or the
   build ran from a stage-one directory with `-2`.

### Multi-stage builds

When a build needs manual work between installing packages and packaging, split
it in two. `-1` performs debootstrap and the overlays and leaves the tree in
`<name>_stage1/`:

```bash
$ sudo bin/vmbetter -1 -branch bookworm misc/vmbetter_configs/miniccc.conf
$ ls miniccc_stage1
```

Enter the tree with `chroot`, make your changes, and leave:

```bash
$ sudo chroot miniccc_stage1
# echo "built by hand" > /etc/motd
# exit
```

Then finish with `-2`, naming the stage-one directory and the original
configuration. The postbuild scripts run at this point, so they run again on
every `-2` invocation, and the stage directory is left in place so you can
iterate:

```bash
$ sudo bin/vmbetter -2 miniccc_stage1 misc/vmbetter_configs/miniccc.conf
```

### Ubuntu builds

vmbetter can debootstrap Ubuntu as well. Point `-mirror` at an Ubuntu archive,
name an Ubuntu release with `-branch`, pass `-constraints ubuntu` so the
`// +build ubuntu` kernel line in `default.conf` is chosen, and enable the
extra components with `-debootstrap-append`. On a Debian host install the
`ubuntu-keyring` package first so debootstrap can verify the archive.
`minimal_ubuntu.conf` is a self-contained example that builds a disk image:

```bash
$ sudo bin/vmbetter -level info -disk -size 5G -branch noble \
    -mirror http://us.archive.ubuntu.com/ubuntu/ \
    -debootstrap-append "--components=main,universe,restricted,multiverse" \
    misc/vmbetter_configs/minimal_ubuntu.conf
```

The comment at the top of `minimal_ubuntu.conf` still shows `-branch bionic`;
substitute a current release codename.

### The vmbetter.bash wrapper

`misc/vmbetter.bash` in the repository wraps the builds the project uses. It
expects a source build in `bin/`, pins `-branch stable`, the US Debian mirror,
and `-level info`, and takes one or more target names:

```bash
$ sudo bash misc/vmbetter.bash miniccc minirouter minicccfs minirouterfs
```

`miniccc`, `minirouter`, `bro`, and `miniception` build kernel and initrd
pairs. `minicccfs` and `minirouterfs` build the container configurations with
`-rootfs`, then `tar` and `gzip` the directory into `<name>.tar.gz`, which is
the form `vm config filesystem tar:<name>.tar.gz` consumes (see
[File management](file.md)). `ccc_host`, `carnac_host`, and the buildbot
targets build cluster-node images, and their `_ubuntu` variants build the same
configurations on Ubuntu.

Be aware that the script's Ubuntu path is stale: it hard-codes `-branch xenial`
(Ubuntu 16.04) and the `us.archive.ubuntu.com` mirror. Edit the
`vmbetter_ubuntu` function to a current release before using those targets, or
call vmbetter directly as shown above.

## Container root filesystems from Docker or LXC

vmbetter's `-rootfs` output is one way to get a container filesystem. Any
Linux root filesystem works, provided it meets a few expectations, so you can
also start from a Docker image or an LXC template.

### What minimega expects of a container filesystem

`vm config filesystem` names a directory that is a root filesystem: it must
contain `/dev`, `/proc`, and `/sys` directories, which minimega mounts over
with a tmpfs `/dev` (plus `/dev/shm` and `/dev/pts`), `proc`, and a read-only
`sysfs`. minimega creates `/dev/null`, `zero`, `full`, `tty`, `random`, and
`urandom`, and links `/dev/fd`, `/dev/stdin`, `/dev/stdout`, and `/dev/stderr`
to `/proc/self`. Anything the image placed under `/dev` is hidden by the
tmpfs.

`vm config init` (default `/init`) names the program minimega executes as
PID 1, with optional arguments. It must be executable and must not exit, or
the container stops. Use a shell script: minimega does not set up the cgroup
and mount layout systemd expects, so distribution images whose `/sbin/init` is
systemd need a script that starts the services you want directly.

`vm config preinit` names an optional program that runs as root before the
container is confined: after the namespaces exist and the filesystems are
mounted, but before capabilities are dropped and before the chroot. Use it for
anything that fails inside the container because `/proc/sys` is read-only
there, such as enabling IP forwarding. The container itself runs with a
reduced capability set, so mounting filesystems or loading modules from inside
it does not work.

Network interfaces appear as `veth0`, `veth1`, and so on, in
`vm config networks` order. minimega listens on a UNIX socket at `/cc` in the container's root, so
miniccc is started with `-family unix -parent /cc`. With the default
`vm config snapshot true` the container runs on an overlay and changes are
discarded on `vm flush`; with `snapshot false` it writes into the directory
itself, and minimega refuses to launch a second container on the same
directory unless both are in snapshot mode.

### From a Docker image

Install the packages your guest needs inside a container, export it, and add
an `init`:

```bash
$ docker run -it --name mmbase debian:bookworm bash
# apt-get update && apt-get install -y isc-dhcp-client iproute2 iputils-ping openssh-server
# exit
$ mkdir /tmp/minimega/files/debianfs
$ docker export mmbase | sudo tar -x -C /tmp/minimega/files/debianfs
$ docker rm mmbase
```

Write the `init` script. It runs with an empty environment, so set `PATH`
explicitly:

```sh title="/tmp/minimega/files/debianfs/init"
#!/bin/sh
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

ip link set lo up
ip link set veth0 up
dhclient -v veth0

mkdir -p /run/sshd
/usr/sbin/sshd &

bash
```

Make it executable and launch:

```bash
$ sudo chmod 755 /tmp/minimega/files/debianfs/init
```

```minimega
minimega$ vm config filesystem /tmp/minimega/files/debianfs
minimega$ vm config networks 100
minimega$ vm launch container deb1
minimega$ vm start deb1
```

To distribute the filesystem across a cluster, pack it as a tarball with a
single top-level directory and use the `tar:` prefix:

```bash
$ cd /tmp/minimega/files && sudo tar -czf debianfs.tar.gz debianfs
```

```minimega
minimega$ vm config filesystem tar:debianfs.tar.gz
```

### From an LXC template

The [LXC](https://linuxcontainers.org/lxc/) `download` template fetches a
plain root filesystem. Copy it out of LXC's tree rather than pointing minimega
at it, then add the same `init` as above:

```bash
$ sudo lxc-create -t download -n base -- -d debian -r bookworm -a amd64
$ sudo cp -a /var/lib/lxc/base/rootfs /tmp/minimega/files/lxcfs
```

### Caveats

- Exported images often have no `/etc/resolv.conf`, or a stale one. `dhclient`
  writes one if the DHCP server supplies DNS; otherwise create it yourself.
- `init` starts with no `PATH` and no locale. Set what the image needs.
- Packages installed while `snapshot` is `true` disappear on flush. Launch
  with `snapshot false` to make changes stick, then switch back to snapshot
  mode to share the directory among many containers.

## See also

- [Virtual machine types](vmtypes.md)
- [Creating VMs from install media](newvm.md)
- [Disk images and the disk API](disk-images.md)
- [Command and control](cc.md) for miniccc's flags
- [Routing with minirouter](router.md)
- [File management](file.md) for the `file:` and `tar:` prefixes
- Reference: [`vm config kernel`](../reference/minimega.md#vm-config-kernel),
  [`vm config initrd`](../reference/minimega.md#vm-config-initrd),
  [`vm config filesystem`](../reference/minimega.md#vm-config-filesystem),
  [`vm config init`](../reference/minimega.md#vm-config-init),
  [`vm config preinit`](../reference/minimega.md#vm-config-preinit)
- [Debootstrap](https://wiki.debian.org/Debootstrap) and
  [EXTLINUX](https://wiki.syslinux.org/wiki/index.php?title=EXTLINUX) on the
  tools vmbetter drives
