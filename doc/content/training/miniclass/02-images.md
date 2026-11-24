# Chapter 2: Get VM images

A VM needs something to boot. In this chapter you build the two guest images
that the whole course runs on. The *client* image starts the miniccc agent at
boot. The *router* image also runs minirouter, dnsmasq, and bird. vmbetter
builds both images from the configurations that ship with minimega. Both are
Debian systems, and both come out as a kernel and initrd pair. At the end, the
four files are in the files directory of minimega. You boot one VM from each
image to make sure that the images work.

This chapter assumes a running minimega from [chapter 1](01-install.md).
[Building images with vmbetter](../../articles/vmbetter.md) covers vmbetter
itself, its configuration format, disk and ISO builds, and container
filesystems. This chapter runs only the two builds that the course needs.

## What you are building

A *kernel and initrd pair* is a Linux kernel plus an initramfs that contains
the entire root filesystem. QEMU loads both into memory, and the guest runs from
RAM. The guest boots in a few seconds and never touches a disk. Every VM that
starts from the pair is identical. These properties suit disposable experiment
nodes. The cost is that nothing persists across a reboot. The whole system must
fit in memory, which is why the images are small.

The two configurations in `misc/vmbetter_configs/` differ only in what runs at
boot:

- `miniccc.conf` inherits `default.conf` (a kernel, a DHCP client, an SSH
  server, and the usual network tools, with password-less root logins). Its
  `/init` mounts the pseudo filesystems and loads the virtio and common network
  drivers. It brings up `eth0` with DHCP and starts `sshd`. It links the
  virtio-serial ports under `/dev/virtio-ports/` and starts
  `/miniccc -serial /dev/virtio-ports/cc`. Then it opens a root shell on the
  console. This is the client image: `vm-left` and `vm-right` boot it.
- `minirouter.conf` adds the `dnsmasq`, `bird` and `iptables` packages. Its
  `/init` has no `eth0` DHCP step. It adds the `tun` module, ARP tuning, and
  IPv4 and IPv6 forwarding. It starts `/minirouter` next to `/miniccc`. This is
  the router image.

The miniccc agent lets minimega run commands inside a guest without any
network. The router also receives its configuration through the agent in the
next chapter. The images carry the agent from the start, so the [command and
control API](08-cc.md) works as soon as a VM is up.

## Install the build tools

vmbetter calls `debootstrap` and `chroot`, and it must run as root. A kernel and
initrd build needs only `debootstrap`, `chroot`, `cp`, and `bash`:

```bash
sudo apt install debootstrap        # Debian and Ubuntu
sudo dnf install debootstrap        # RHEL-family; enable EPEL first if dnf cannot find it
```

Disk and ISO builds need more (`qemu-utils`, `extlinux`, `squashfs-tools`,
`genisoimage`). The
[vmbetter guide](../../articles/vmbetter.md#installing-vmbetter-and-its-dependencies)
lists them. Skip those for now.

## Where the agent binaries come from

The overlay directories next to the configurations contain the files that
vmbetter copies over the root of the new system. `miniccc_overlay/` holds the
`init` script and a link named `miniccc`. `minirouter_overlay/` holds `init` and
links named `miniccc` and `minirouter`. The links point at `../../../bin/`. This
path resolves to `/opt/minimega/bin/` on a package install and to `bin/` in a
source tree. When vmbetter copies an overlay, it follows links. Before you
build, make sure that the links resolve. `ls -L` fails on a dangling link:

```bash
ls -L /opt/minimega/misc/vmbetter_configs/miniccc_overlay/
ls -L /opt/minimega/misc/vmbetter_configs/minirouter_overlay/
```

Either command can report a missing file, for example because you unpacked the
configurations somewhere other than the binaries. In that case, use one of these
fixes:

- Put copies of `miniccc` and `minirouter` where the links point (a `bin/`
  directory three levels above the overlay).
- Remove the links and copy the binaries into the overlays in their place.

The agents in an image must come from the same release as the daemon. A client
from a different version logs a mismatch warning and can drop commands.

## Build the two pairs

vmbetter writes `<name>.kernel` and `<name>.initrd` into the current directory.
Run it from the files directory of minimega, so the results land where the next
chapters look for them. `-branch bookworm` pins the Debian release that
`debootstrap` installs (the default is `testing`). `-level info` shows progress.
Use the name of the release, not `stable`. `stable` is now Debian 13 (trixie).
This release no longer carries the `ntp` and `dnsutils` packages that
`misc/vmbetter_configs/default.conf` installs. Because of this, the build stops
with `E: Couldn't find these debs: dnsutils ntp`.

Package:

```bash
sudo sh -c 'cd /tmp/minimega/files && /opt/minimega/bin/vmbetter -level info -branch bookworm /opt/minimega/misc/vmbetter_configs/miniccc.conf'
sudo sh -c 'cd /tmp/minimega/files && /opt/minimega/bin/vmbetter -level info -branch bookworm /opt/minimega/misc/vmbetter_configs/minirouter.conf'
```

Source:

The `misc/vmbetter.bash` script in the repository wraps the same two builds with
the same flags. It expects the binaries in `bin/`. It pins `branch=stable`, so
first change that line to `branch=bookworm`, for the reason above. The script
writes into the current directory. Run it from the repository root, and then
move the results:

```bash
sudo bash misc/vmbetter.bash miniccc minirouter
sudo mv miniccc.kernel miniccc.initrd minirouter.kernel minirouter.initrd /tmp/minimega/files/
```

Docker:

The image contains `vmbetter` but not `debootstrap` or the configurations. Build
on the host with the binary from the container and the configurations from the
repository. The container shares `/tmp/minimega`, so the output is visible
inside the container:

```bash
sudo docker cp minimega:/opt/minimega/bin/vmbetter /usr/local/bin/vmbetter
git clone --depth 1 https://github.com/sandia-minimega/minimega.git ~/minimega
mkdir -p ~/minimega/bin
sudo docker cp minimega:/opt/minimega/bin/miniccc ~/minimega/bin/miniccc
sudo docker cp minimega:/opt/minimega/bin/minirouter ~/minimega/bin/minirouter
sudo sh -c "cd /tmp/minimega/files && /usr/local/bin/vmbetter -level info -branch bookworm $HOME/minimega/misc/vmbetter_configs/miniccc.conf"
sudo sh -c "cd /tmp/minimega/files && /usr/local/bin/vmbetter -level info -branch bookworm $HOME/minimega/misc/vmbetter_configs/minirouter.conf"
```

A fresh clone has no `bin/` directory. So the overlay links dangle until the two
`docker cp` lines put the agents of the container where the links point. Taking
the agents from the container makes sure that they match the version of the
daemon.

Each build runs `debootstrap` against the Debian mirror and installs the listed
packages. It copies the overlay in and runs the `postbuild` script of the
configuration in a chroot. Then it packs the tree with `cpio` and `gzip`. Each
build takes several minutes and downloads a few hundred megabytes. The second
build is not faster, because each build starts from an empty directory. When
both builds finish, list the files:

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

`/tmp/minimega/files` is the *files directory* of minimega. The `-filepath` flag
sets it (`MM_FILEPATH` under systemd or Docker), and the default is
`<base>/files`. Two properties make it the right place for images:

- A relative path in `vm config kernel`, `vm config initrd`, `vm config disks`,
  and the other image fields resolves inside the files directory. So the course
  can write `vm config kernel miniccc.kernel`, and the same script works on any
  host that has the files there.
- The `file` API serves the directory to the rest of a cluster. On a multi-host
  experiment, minimega fetches an image named `file:miniccc.kernel` from
  whichever node has it before the VM launches. [Chapter 20](20-clusters.md)
  uses this.

`file list /` from the prompt shows what minimega sees there:

```minimega
minimega$ file list /
```

minimega serves anything else in the directory the same way. Examples are the
protonuke binary in [chapter 11](11-traffic.md) and scripts that you push to
guests with `cc send`. Another example is the screenshots that miniweb serves
under its Files page.

## Boot one VM from each image

Before you build on the images, make sure that both images boot and that
miniccc connects. `vm config kernel` and `vm config initrd` point the next
launch at a pair. `vm launch kvm <name>` creates a VM from the current
description, and `vm start` boots it. Both VMs go into a namespace of their own,
so they are easy to remove. [Chapter 3](03-router-sandwich.md) explains
namespaces in detail.

```minimega
minimega$ namespace sandwich
minimega$ vm config kernel miniccc.kernel
minimega$ vm config initrd miniccc.initrd
minimega$ vm config memory 3072
minimega$ vm launch kvm client-test
minimega$ vm config kernel minirouter.kernel
minimega$ vm config initrd minirouter.initrd
minimega$ vm launch kvm router-test
minimega$ vm start all
```

Give the guests a minute or two. The `init` of the images waits ten seconds for
devices to settle before it starts anything. So `cc_active`, the column that
says whether a miniccc agent is connected, takes a while to turn `true`. A
`shell` loop can wait for it. The loop asks minimega every three seconds until
two VMs report `true`. It gives up after five minutes. [Chapter
3](03-router-sandwich.md#start-in-order) explains the loop:

```minimega
minimega$ shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true' | grep -qx 2; do sleep 3; done"
```

Then look:

```minimega
minimega$ .annotate false .columns name,state,cc_active vm info
name        | state   | cc_active
client-test | RUNNING | true
router-test | RUNNING | true
```

If a VM is `RUNNING` but `cc_active` stays `false`, the agent inside the VM does
not connect. Usually the overlay link dangled, and the image has no `/miniccc`.
Fix the link, and then rebuild. If a VM is in `ERROR`, the `error` tag says why.
`vm tag client-test error` prints it. The most common causes are a misspelled
image name and a host without KVM.

Then remove the test VMs. `vm kill` stops them and leaves them in the `QUIT`
state. `vm flush` forgets them:

```minimega
minimega$ vm kill all
minimega$ vm flush
```

The script for this chapter does that, so you can rerun the test any time that
you rebuild an image:

```minimega title="02-images.mm"
--8<-- "training/miniclass/scripts/02-images.mm"
```

[Download this example](scripts/02-images.mm){ download="02-images.mm" }

## Other kinds of image

The course uses only kernel and initrd pairs. But the same `vm config` and
`vm launch` commands boot anything that QEMU can boot:

- A disk image that vmbetter builds: `vmbetter -disk -size 4G ...` produces a
  bootable qcow2 from a configuration that installs an init system. Use it for
  guests that need to keep state across reboots.
- A disk image that you install from an ISO: you can install any distribution,
  or Windows, into an empty qcow2 through its own installer. Then give the image
  miniccc with `disk inject`.
  [Creating VMs from install media](../../articles/newvm.md) walks through it.
  [Disk images and the disk API](../../articles/disk-images.md) covers
  snapshots, backing chains, and injection.
- A container root filesystem: `vmbetter -rootfs`, or the `minicccfs` and
  `minirouterfs` targets of `misc/vmbetter.bash`, produce a directory tree.
  `vm launch container` runs the tree without a kernel of its own.
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
