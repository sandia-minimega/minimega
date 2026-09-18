# Chapter 2: Get VM images

A VM needs something to boot. In this chapter you build the two guest images
the whole course runs on: a *client* image that starts the miniccc agent at
boot, and a *router* image that also runs minirouter, dnsmasq and bird. Both
are Debian systems built by vmbetter from the configurations that ship with
minimega, and both come out as a kernel and initrd pair. At the end the four
files are in minimega's files directory and you have booted one VM from each
to prove they work.

This chapter assumes a running minimega from
[chapter 1](01-install.md). vmbetter itself, its configuration format, disk
and ISO builds and container filesystems are covered in
[Building images with vmbetter](../../articles/vmbetter.md); this chapter only
runs the two builds the course needs.

## What you are building

A *kernel and initrd pair* is a Linux kernel plus an initramfs that contains
the entire root filesystem. QEMU loads both into memory and the guest runs
from RAM: it boots in a few seconds, never touches a disk, and every VM
started from the pair is identical. That is exactly what you want for
disposable experiment nodes. The cost is that nothing persists across a
reboot and the whole system has to fit in memory, which is why the images are
small.

The two configurations in `misc/vmbetter_configs/` differ only in what runs at
boot:

- `miniccc.conf` inherits `default.conf` (a kernel, a DHCP client, an SSH
  server, and the usual network tools, with password-less root logins). Its
  `/init` mounts the pseudo filesystems, loads the virtio and common network
  drivers, brings up `eth0` with DHCP, starts `sshd`, links the virtio-serial
  ports under `/dev/virtio-ports/`, starts
  `/miniccc -serial /dev/virtio-ports/cc`, and opens a root shell on the
  console. This is the client image: `vm_left` and `vm_right` boot it.
- `minirouter.conf` adds the `dnsmasq` and `bird` packages. Its `/init` does
  everything the client's does, turns on IPv4 and IPv6 forwarding, and starts
  `/minirouter` next to `/miniccc`. This is the router image.

The miniccc agent is what lets minimega run commands inside a guest without
any network, and it is how the router receives its configuration in the next
chapter. The images carry it from the start so that the [command and control
API](08-cc.md) works the moment a VM is up.

## Install the build tools

vmbetter drives `debootstrap` and `chroot` and must run as root. A kernel and
initrd build needs only `debootstrap`, `chroot`, `cp` and `bash`:

```bash
sudo apt install debootstrap        # Debian and Ubuntu
sudo dnf install debootstrap        # RHEL-family; enable EPEL first if dnf cannot find it
```

Disk and ISO builds need more (`qemu-utils`, `extlinux`, `squashfs-tools`,
`genisoimage`); the
[vmbetter guide](../../articles/vmbetter.md#installing-vmbetter-and-its-dependencies)
lists them. Skip those for now.

## Where the agent binaries come from

The overlay directories next to the configurations contain the files copied
over the root of the new system. `miniccc_overlay/` holds the `init` script
and a link named `miniccc`; `minirouter_overlay/` holds `init` and links
named `miniccc` and `minirouter`. The links point at `../../../bin/`, which
resolves to `/opt/minimega/bin/` on a package install and to `bin/` in a
source tree, and vmbetter follows links when it copies an overlay. Check that
they resolve before building; `ls -L` fails on a dangling link:

```bash
ls -L /opt/minimega/misc/vmbetter_configs/miniccc_overlay/
ls -L /opt/minimega/misc/vmbetter_configs/minirouter_overlay/
```

If either command reports a missing file, for example because you unpacked
the configurations somewhere other than the binaries, either put copies of
`miniccc` and `minirouter` where the links point (a `bin/` directory three
levels above the overlay) or remove the links and copy the binaries into the
overlays in their place. The agents baked into an image must come from the same release
as the daemon; a client built from a different version logs a mismatch
warning and may drop commands.

## Build the two pairs

vmbetter writes `<name>.kernel` and `<name>.initrd` into the current
directory, so run it from minimega's files directory and the results land
where the next chapters look for them. `-branch stable` pins the Debian
release that `debootstrap` installs (the default is `testing`), and
`-level info` shows progress.

**Package**

```bash
cd /tmp/minimega/files
sudo /opt/minimega/bin/vmbetter -level info -branch stable /opt/minimega/misc/vmbetter_configs/miniccc.conf
sudo /opt/minimega/bin/vmbetter -level info -branch stable /opt/minimega/misc/vmbetter_configs/minirouter.conf
```

**Source**

The repository's `misc/vmbetter.bash` wraps the same two builds with the same
flags and expects the binaries in `bin/`. It writes into the current
directory, so run it from the repository root and move the results:

```bash
sudo bash misc/vmbetter.bash miniccc minirouter
sudo mv miniccc.kernel miniccc.initrd minirouter.kernel minirouter.initrd /tmp/minimega/files/
```

**Docker**

The image contains `vmbetter` but not `debootstrap` or the configurations, so
build on the host with the binary from the container and the configurations
from the repository. The container shares `/tmp/minimega`, so the output is
visible inside it:

```bash
sudo docker cp minimega:/opt/minimega/bin/vmbetter /usr/local/bin/vmbetter
git clone --depth 1 https://github.com/sandia-minimega/minimega.git ~/minimega
mkdir -p ~/minimega/bin
sudo docker cp minimega:/opt/minimega/bin/miniccc ~/minimega/bin/miniccc
sudo docker cp minimega:/opt/minimega/bin/minirouter ~/minimega/bin/minirouter
cd /tmp/minimega/files
sudo /opt/minimega/bin/vmbetter -level info -branch stable ~/minimega/misc/vmbetter_configs/miniccc.conf
sudo /opt/minimega/bin/vmbetter -level info -branch stable ~/minimega/misc/vmbetter_configs/minirouter.conf
```

A fresh clone has no `bin/` directory, so the overlay links dangle until the
two `docker cp` lines put the container's agents where they point. Taking
them from the container guarantees they match the daemon's version.

Each build runs `debootstrap` against the Debian mirror, installs the listed
packages, copies the overlay in, runs the configuration's `postbuild` script
in a chroot, and packs the tree with `cpio` and `gzip`. Expect several
minutes and a few hundred megabytes of download per build; the second build
is not faster, because each starts from an empty directory. When both finish:

```bash
ls -l /tmp/minimega/files/
```

```text
miniccc.initrd
miniccc.kernel
minirouter.initrd
minirouter.kernel
```

## Where images live

`/tmp/minimega/files` is minimega's *files directory*, set by the `-filepath`
flag (`MM_FILEPATH` under systemd or Docker) and `<base>/files` by default.
Two things make it the right home for images:

- A relative path in `vm config kernel`, `vm config initrd`, `vm config disks`
  and the other image fields is resolved inside it, so the course can write
  `vm config kernel miniccc.kernel` and the same script works on any host
  that has the files there.
- The `file` API serves it to the rest of a cluster. On a multi-host
  experiment, an image named `file:miniccc.kernel` is fetched from whichever
  node has it before the VM launches. [Chapter 20](20-clusters.md) uses this.

`file list /` from the prompt shows what minimega sees there:

```minimega
minimega$ file list /
```

Anything else you put in the directory is served the same way: the protonuke
binary in [chapter 11](11-traffic.md), scripts pushed to guests with
`cc send`, and the screenshots miniweb serves under its Files page.

## Boot one VM from each image

Before building on them, prove that both images boot and that miniccc
connects. `vm config kernel` and `vm config initrd` point the next launch at
a pair; `vm launch kvm <name>` creates a VM from the current description and
`vm start` boots it. Both VMs go into a namespace of their own so that they
are easy to remove; [chapter 3](03-router-sandwich.md) explains namespaces
properly.

```minimega
minimega$ namespace sandwich
minimega$ vm config kernel miniccc.kernel
minimega$ vm config initrd miniccc.initrd
minimega$ vm launch kvm client_test
minimega$ vm config kernel minirouter.kernel
minimega$ vm config initrd minirouter.initrd
minimega$ vm launch kvm router_test
minimega$ vm start all
```

Give the guests half a minute. The images' `init` waits ten seconds for
devices to settle before it starts anything, so `cc_active`, the column that
says whether a miniccc agent is connected, takes a moment to turn `true`:

```minimega
minimega$ .annotate false .columns name,state,cc_active vm info
name        | state   | cc_active
client_test | RUNNING | true
router_test | RUNNING | true
```

If a VM is `RUNNING` but `cc_active` stays `false`, the agent inside is not
connecting: usually the overlay link was dangling and the image has no
`/miniccc`. Rebuild after fixing the link. If a VM is in `ERROR`, the `error`
tag says why; `vm tag client_test error` prints it, and the most common
causes are a misspelled image name and a host without KVM.

Then remove the test VMs. `vm kill` stops them and leaves them in the `QUIT`
state; `vm flush` forgets them:

```minimega
minimega$ vm kill all
minimega$ vm flush
```

The script for this chapter does exactly that, so you can rerun the test any
time you rebuild an image:

```minimega title="02-images.mm"
--8<-- "training/miniclass/scripts/02-images.mm"
```

[Download this example](scripts/02-images.mm){ download="02-images.mm" }

## Other kinds of image

The course sticks to kernel and initrd pairs, but the same `vm config` and
`vm launch` commands boot anything QEMU can:

- **A disk image built by vmbetter.** `vmbetter -disk -size 4G ...` produces
  a bootable qcow2 from the same configuration, for guests that need to keep
  state across reboots.
- **A disk image installed from an ISO.** Any distribution, or Windows, can
  be installed into an empty qcow2 through its own installer, then given
  miniccc with `disk inject`. [Creating VMs from install media](../../articles/newvm.md)
  walks through it, and [Disk images and the disk API](../../articles/disk-images.md)
  covers snapshots, backing chains and injection.
- **A container root filesystem.** `vmbetter -rootfs`, or the `minicccfs`
  and `minirouterfs` targets of `misc/vmbetter.bash`, produce a directory
  tree that `vm launch container` runs without a kernel of its own.
  [Chapter 17](17-containers.md) builds one.

## What you built

- `miniccc.kernel` and `miniccc.initrd`, the client image, with the miniccc
  agent started at boot.
- `minirouter.kernel` and `minirouter.initrd`, the router image, with
  minirouter, dnsmasq and bird on top of the client image.
- Both in `/tmp/minimega/files`, and each booted once with a connected agent.

## Where to read more

- [Building images with vmbetter](../../articles/vmbetter.md): configuration
  files, overlays, every flag, disk and ISO builds, container filesystems.
- [Creating VMs from install media](../../articles/newvm.md) and
  [Disk images and the disk API](../../articles/disk-images.md) for
  installed-OS images.
- [File management](../../articles/file.md): the files directory and the
  `file:` prefix.
- Reference: [`vm config kernel`](../../reference/minimega.md#vm-config-kernel),
  [`vm config initrd`](../../reference/minimega.md#vm-config-initrd),
  [`vm launch`](../../reference/minimega.md#vm-launch),
  [`file`](../../reference/minimega.md#file).

## Next

[Chapter 3: The router sandwich](03-router-sandwich.md) builds the experiment
the rest of the course extends.
