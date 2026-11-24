# Building images with vmbetter

vmbetter builds minimal Debian-based guest images from a short configuration
file. One build produces the artifact that you ask for. The artifact is a kernel
and initrd pair (the default), a bootable disk image, a live ISO, or a plain
root filesystem directory for containers. This page explains which image type
boots which kind of VM. It also explains how the parts of a configuration file
fit together and what every flag does. Last, it shows how to turn an existing
Docker or LXC filesystem into a minimega container.

If you want a reproducible, scriptable guest without an installer to click
through, you need vmbetter. The guest starts fast and already contains miniccc.
If you need a full distribution or Windows installed from its own media, see
[Creating VMs from install media](newvm.md). To work with disk images after they
exist, see [Disk images and the disk API](disk-images.md).

## Image types and which VM they boot

minimega runs two kinds of Linux guest, and they need different artifacts.
[Virtual machine types](vmtypes.md) covers the VM types in full. The short
version is:

- KVM guests are full virtual machines with their own kernel. They boot from a
  disk image (`vm config disks`) or a CD image (`vm config cdrom`). They can
  also boot from a kernel and initrd pair (`vm config kernel` and
  `vm config initrd`). A kernel and initrd pair runs entirely from RAM, starts
  in a second or two, and never touches a disk. This suits hundreds of
  identical, disposable nodes. When the guest must keep its state, look like an
  installed machine, or hold more than fits comfortably in an initrd, use a disk
  image.
- Container guests share the host kernel. They need only a root filesystem
  directory that contains an `init` program (`vm config filesystem`). minimega
  creates the namespaces and cgroups itself, so no container engine runs on the
  host. The same `vm config`, networking, `cc`, and capture commands apply to
  containers and KVM guests alike. Because there is no layered image format and
  no registry, a container image is a directory tree. vmbetter produces this
  tree directly, and you can also extract it from Docker or LXC (see
  [below](#container-root-filesystems-from-docker-or-lxc)).

vmbetter produces all of these from the same configuration. A flag selects the
artifact:

| Flag | Output in the current directory | Boot it with |
|---|---|---|
| (none) | `<name>.kernel` and `<name>.initrd` | `vm config kernel` and `vm config initrd` |
| `-disk` | `<name>.<format>` (`<name>.qcow2` by default) | `vm config disks` |
| `-iso` | `<name>.iso` | `vm config cdrom` |
| `-rootfs` | `<name>_rootfs/` directory | `vm config filesystem` |

Unless you override it with `-O`, `<name>` is the basename of the configuration
file without its extension. With `-O`, `-rootfs` writes a directory with exactly
the name that you give, with no `_rootfs` suffix.

For kernel and initrd builds, vmbetter packs the entire filesystem into the
initrd. The kernel runs the `/init` of the overlay as PID 1. Disk and ISO builds
boot through the initramfs of the distribution from `/boot`. Therefore, those
images must contain a normal init system. For exactly this reason,
`minimal_ubuntu.conf` installs the `init` package and regenerates the initramfs
in its `postbuild`.

## Installing vmbetter and its dependencies

The `.deb` and `.rpm` packages install vmbetter as `/opt/minimega/bin/vmbetter`
together with the example configurations in
`/opt/minimega/misc/vmbetter_configs/`. A source build (`scripts/build.bash` or
`scripts/all.bash`) puts it in `bin/vmbetter`. The Docker image contains the
binary but not the build toolchain below. Run vmbetter on a host.

vmbetter must run as root. It calls `debootstrap` and `chroot`, and disk builds
attach the image to an NBD device. It looks for every external tool at start-up
and logs the ones that it cannot find. On Debian or Ubuntu, run:

```bash
$ sudo apt install debootstrap extlinux syslinux-common squashfs-tools genisoimage qemu-utils
```

Every build needs `debootstrap`, `chroot`, `cp`, and `bash`. Disk images also
need `qemu-img`, `qemu-nbd`, `sfdisk`, `mkfs.ext3`, `mount`, `dd`, `extlinux`,
and syslinux's `mbr.bin`. ISO images need `mksquashfs` and `genisoimage` plus
an isolinux directory. The repository ships one in `misc/isolinux/`, but the
packages do not install it.

## Configuration files

A configuration file is a list of `key = "value"` lines. Values are
double-quoted strings. When a value spans several lines, it is a
backtick-quoted raw string. A `//` starts a comment. There are exactly four
keys:

| Key | Meaning |
|---|---|
| `parents` | Space-separated list of configuration files to include first. Parents are read recursively, so their packages, overlays, and postbuild commands apply before those of this file. If vmbetter does not find a parent relative to the current directory, it looks for the parent relative to the file that names it. |
| `packages` | Space-separated Debian package names, passed to `debootstrap --include`. The key can appear any number of times. vmbetter concatenates the lists, and debootstrap resolves dependencies. |
| `overlay` | A directory whose contents vmbetter copies over the root of the new filesystem after debootstrap finishes. A relative path is relative to the configuration file. Overlays apply parents first, so the files of a child overwrite the files of a parent. When vmbetter copies the overlay, it follows symlinks inside it. |
| `postbuild` | A shell script run with `bash` inside a chroot of the new filesystem, with `/proc` and `/dev` mounted, after all overlays are in place. Parent scripts run first. |

A build-constraint comment in the style of Go source can guard a key. If the
`-constraints` flag (default `debian,amd64`) contains every listed tag, the key
that follows applies. Otherwise, the key does not apply. Do not use `!tag`.
vmbetter does not read `!tag` as "not tag", so a key with a `!tag` constraint
does not apply. For example:

```text
// +build debian amd64
packages = "linux-headers-amd64 linux-image-amd64"

// +build ubuntu
packages = "linux-headers-generic linux-image-generic"
```

`vmbetter -dry-run <config>` prints the fully resolved configuration and exits.
This is the quickest way to see what a chain of parents produces.

### The shipped configurations

`misc/vmbetter_configs/` holds the configurations that the project itself uses,
each with a matching `<name>_overlay/` directory:

- `default.conf` is the base that almost everything inherits from. It contains
  a kernel, a DHCP client, an SSH server, `vim`, and the usual network tools.
  Its `postbuild` allows password-less root logins on the console and over SSH
  and cleans the apt cache. Its overlay holds an `/init` script and a minimal
  `/etc/ssh/sshd_config` that permits password-less root logins. The `/init`
  script mounts `/proc`, `/sys`, and `/dev` and starts udev. It also brings up
  `eth0` with DHCP, starts `sshd`, and opens a shell on the console.
- `miniccc.conf` swaps in an `/init` that also loads the virtio and common
  network drivers and creates `/dev/virtio-ports/<name>` symlinks. This `/init`
  starts `/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /miniccc.log`.
  The `miniccc` file in the overlay is a symlink to `../../../bin/miniccc`. When
  vmbetter copies the overlay, it follows this symlink. The binary comes from
  `bin/` of a source build (`scripts/build.bash`) or from `/opt/minimega/bin/`
  of the package. In a fresh clone that you did not build, the link dangles and
  the build stops with a `cp` error.
- `minirouter.conf` adds `dnsmasq` and `bird` and starts `minirouter`, for use
  with the [router API](router.md). Its overlay links `miniccc` and
  `minirouter` into `bin/` in the same way.
- `miniccc_container.conf` and `minirouter_container.conf` are the container
  equivalents. The `/init` of `miniccc_container` brings up `veth0` with DHCP.
  The `/init` of `minirouter_container` brings up only `lo`. Both start
  `miniccc -family unix -parent /cc`. This is because a container reaches
  minimega over a UNIX socket that minimega creates at `/cc` in the root of the
  container. `minirouter_container_overlay` also carries a `preinit` script that
  enables IP forwarding. This must happen before minimega isolates the
  container.
- `host.conf`, `ccc_host.conf`, `carnac_host.conf`, the `*_buildbot.conf`
  files, and `miniception.conf` build images for nodes that run minimega
  itself.
- `bro.conf` inherits `default.conf` directly and adds `miniccc_overlay` and
  its own `bro_overlay` to run the Bro network monitor (the package that
  preceded Zeek). The `bro` package is not in current Debian releases, so
  treat it as an example rather than a working build.
- `minimal_ubuntu.conf` shows how to build an Ubuntu disk image instead of a
  Debian initrd. See [Ubuntu builds](#ubuntu-builds) below.

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
and edit it. Because vmbetter copies child overlays last, your `init` replaces
the `init` of the parent. Keep `init` executable, because it becomes PID 1.

## Building

Run vmbetter from the repository root (or from `/opt/minimega` for the
package). The relative overlay paths then resolve. The default `-isolinux`
directory resolves only in a source checkout. To build an ISO from a package
install, give `-isolinux` your own directory. This example builds a kernel and an
initrd:

```bash
$ cd /opt/minimega
$ sudo bin/vmbetter -level info -branch bookworm misc/vmbetter_configs/miniccc.conf
```

This writes `miniccc.kernel` and `miniccc.initrd` to the current directory.
Copy them into the files directory of minimega. This lets nodes fetch them
across a cluster. Then boot them. The image unpacks into RAM, so give each VM
3 GB of memory. A VM with the 2 GB default does not boot:

```minimega
minimega$ vm config kernel /tmp/minimega/files/miniccc.kernel
minimega$ vm config initrd /tmp/minimega/files/miniccc.initrd
minimega$ vm config memory 3072
minimega$ vm launch kvm node[1-4]
minimega$ vm start all
```

For a disk image, add `-disk` and `-size`. For a live ISO, add `-iso`. These
images need a configuration that installs an init system, and `miniccc.conf`
lists no init package. [Ubuntu builds](#ubuntu-builds) has a complete disk
example with `minimal_ubuntu.conf`.

### Flags

| Flag | Default | Effect |
|---|---|---|
| `-branch` | `testing` | Debian suite passed to debootstrap. Name a release (`bookworm`, `trixie`) or use `stable` for repeatable builds. |
| `-mirror` | `http://ftp.us.debian.org/debian` | Package mirror. |
| `-constraints` | `debian,amd64` | Comma-separated tags matched against `// +build` lines. |
| `-debootstrap-append` | (none) | Extra arguments placed before debootstrap's own, for example `"--components=main,universe"`. |
| `-disk` | off | Build a disk image instead of a kernel and initrd. |
| `-size` | `1G` | Disk image size in `qemu-img` notation. Used with `-disk`. |
| `-format` | `qcow2` | Disk image format: `qcow`, `qcow2`, `raw`, or `vmdk`. |
| `-mbr` | `/usr/lib/syslinux/mbr/mbr.bin` | Master boot record written to the disk image. If your syslinux package installs `mbr.bin` elsewhere, override this flag. |
| `-iso` | off | Build a live ISO. vmbetter adds the `live-boot` package automatically. |
| `-isolinux` | `misc/isolinux/` | Directory containing `isolinux.bin`, `ldlinux.c32`, and `isolinux.cfg`, relative to the current directory. |
| `-rootfs` | off | Copy the finished filesystem to `<name>_rootfs/` for use as a container filesystem. With `-O`, the directory has exactly the `-O` name. |
| `-O` | configuration basename | Output name. |
| `-1` | off | Stop after stage one and copy the build tree to `<config basename>_stage1/`. This flag ignores `-O`. |
| `-2 <dir>` | (none) | Run stage two on an existing stage-one directory. Cannot be combined with `-1`. |
| `-noclean` | off | Keep the temporary build directory under `/tmp`. |
| `-dry-run` | off | Print the resolved configuration and exit. |
| `-level` | `error` | Log level: `debug`, `info`, `warn`, `error`, or `fatal`. `info` shows debootstrap's progress. |
| `-logfile` | (none) | Also write the log to a file. |
| `-v` | `true` | Log to standard error. |

### What a build does

1. vmbetter reads the configuration and its parents. With `-iso`, it adds
   `live-boot`.
2. vmbetter runs `debootstrap --variant=minbase --include=<packages> <branch> <build dir> <mirror>`.
3. vmbetter copies each overlay into the build directory, parents first. This is
   the end of stage one. With `-1`, vmbetter copies the tree to
   `<name>_stage1/` and stops.
4. vmbetter runs each `postbuild` script in a chroot, parents first.
5. vmbetter produces the target. A kernel and initrd build packs the tree with
   `cpio` and `gzip` and copies `/boot/vmlinu*` out. A disk build creates the
   image with `qemu-img` and attaches it with `qemu-nbd`. Then it writes one
   bootable ext3 partition, copies the tree in, and installs `extlinux` with a
   `root=/dev/sda1` entry and the MBR. An ISO build squashes the tree with
   `mksquashfs` and runs `genisoimage` with the isolinux files. A rootfs build
   copies the tree to `<name>_rootfs/`.
6. vmbetter removes the temporary build directory. If you give `-noclean`, or if
   the build runs from a stage-one directory with `-2`, vmbetter keeps the
   directory.

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

Then finish with `-2`. Name the stage-one directory and the original
configuration. The postbuild scripts run at this point, so they run again on
every `-2` invocation. vmbetter leaves the stage directory in place, so you can
iterate:

```bash
$ sudo bin/vmbetter -2 miniccc_stage1 misc/vmbetter_configs/miniccc.conf
```

### Ubuntu builds

vmbetter can debootstrap Ubuntu as well. Use these flags:

1. Point `-mirror` at an Ubuntu archive.
2. Name an Ubuntu release with `-branch`.
3. Pass `-constraints ubuntu`, so vmbetter selects the `// +build ubuntu` kernel
   line in `default.conf`.
4. Enable the extra components with `-debootstrap-append`.

On a Debian host, first install the `ubuntu-keyring` package. Then debootstrap
can verify the archive. `minimal_ubuntu.conf` is a self-contained example that
builds a disk image:

```bash
$ sudo bin/vmbetter -level info -disk -size 5G -branch noble \
    -mirror http://us.archive.ubuntu.com/ubuntu/ \
    -debootstrap-append "--components=main,universe,restricted,multiverse" \
    misc/vmbetter_configs/minimal_ubuntu.conf
```

The comment at the top of `minimal_ubuntu.conf` still shows `-branch bionic`.
Substitute a current release codename.

### The vmbetter.bash wrapper

`misc/vmbetter.bash` in the repository wraps the builds the project uses. It
expects a source build in `bin/`. It pins `-branch stable`, the US Debian
mirror, and `-level info`. It takes one or more target names:

```bash
$ sudo bash misc/vmbetter.bash miniccc minirouter minicccfs minirouterfs
```

`miniccc`, `minirouter`, `bro`, and `miniception` build kernel and initrd
pairs. `minicccfs` and `minirouterfs` build the container configurations with
`-rootfs`. Then they pack the directory with `tar` and `gzip` into
`<name>.tar.gz`. `vm config filesystem tar:<name>.tar.gz` consumes this form
(see [File management](file.md)). `ccc_host`, `carnac_host`, and the buildbot
targets build cluster-node images. Their `_ubuntu` variants build the same
configurations on Ubuntu.

Before you use the Ubuntu targets, edit the `vmbetter_ubuntu` function to a
current release. Or call vmbetter directly as shown above. The Ubuntu path of
the script is stale. It hard-codes `-branch xenial` (Ubuntu 16.04) and the
`us.archive.ubuntu.com` mirror.

## Container root filesystems from Docker or LXC

The `-rootfs` output of vmbetter is one way to get a container filesystem. Any
Linux root filesystem that meets a few expectations works. So you can also
start from a Docker image or an LXC template.

### What minimega expects of a container filesystem

`vm config filesystem` names a directory that is a root filesystem. If `/dev`,
`/proc`, and `/sys` are missing, minimega creates them. minimega mounts over
them with a tmpfs `/dev` (plus `/dev/shm` and `/dev/pts`), `proc`, and a
read-only `sysfs`. minimega creates `/dev/null`, `zero`, `full`, `tty`,
`random`, and `urandom`. It also links `/dev/fd`, `/dev/stdin`, `/dev/stdout`,
and `/dev/stderr` to `/proc/self`. The tmpfs hides anything that the image
placed under `/dev`.

`vm config init` (default `/init`) names the program minimega executes as PID 1,
with optional arguments. It must be executable and must not exit. If it exits,
the container stops. Use a shell script, because minimega does not set up the
cgroup and mount layout that systemd expects. Distribution images whose
`/sbin/init` is systemd need a script that starts the services you want
directly.

`vm config preinit` names an optional program that runs as root before minimega
confines the container. It runs after the namespaces exist and the filesystems
are mounted. It runs before minimega drops capabilities and before the chroot.
Use it for anything that fails inside the container because `/proc/sys` is
read-only there, such as enabling IP forwarding. The container itself runs with
a reduced capability set. Therefore, mounting filesystems or loading modules
from inside the container does not work.

Network interfaces appear as `veth0`, `veth1`, and so on, in
`vm config networks` order. minimega listens on a UNIX socket at `/cc` in the
root of the container, so miniccc runs with `-family unix -parent /cc`. With the
default `vm config snapshot true`, the container runs on an overlay and
minimega discards changes on `vm flush`. With `snapshot false`, the container
writes into the directory itself. In this case, minimega refuses to launch a
second container on the same directory unless both containers are in snapshot
mode.

### From a Docker image

Install the packages that your guest needs inside a container, export the
container, and add an `init`:

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
minimega$ vm config net 100
minimega$ vm launch container deb1
minimega$ vm start deb1
```

To distribute the filesystem across a cluster, pack it as a tarball with a
single top-level directory. Then use the `tar:` prefix:

```bash
$ sudo tar -C /tmp/minimega/files -czf /tmp/minimega/files/debianfs.tar.gz debianfs
```

```minimega
minimega$ vm config filesystem tar:debianfs.tar.gz
```

### From an LXC template

The [LXC](https://linuxcontainers.org/lxc/) `download` template fetches a
plain root filesystem. Copy it out of LXC's tree. Do not point minimega
at the tree. Then add the same `init` as above:

```bash
$ sudo lxc-create -t download -n base -- -d debian -r bookworm -a amd64
$ sudo cp -a /var/lib/lxc/base/rootfs /tmp/minimega/files/lxcfs
```

### Caveats

- Exported images often have no `/etc/resolv.conf`, or a stale one. If the
  DHCP server supplies DNS, `dhclient` writes one. Otherwise, create the file
  yourself.
- `init` starts with no `PATH` and no locale. Set what the image needs.
- Packages that you install while `snapshot` is `true` disappear on flush. To
  keep changes, launch with `snapshot false`. Then switch back to snapshot mode
  to share the directory among many containers.

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
