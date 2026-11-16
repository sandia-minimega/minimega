# Installing minimega

This page shows how to install minimega on a Linux host. It describes what the
host needs. It also describes the four ways to install minimega: distribution
packages, the Docker image, the Python bindings, or a source build. The page
also lists the external programs that minimega calls at runtime and the host
preparation that no package does for you. After you install minimega, the
[quickstart](quickstart.md) boots your first VMs. [Running minimega](running.md)
explains the service, the flags, and the environment file.

## What you need

minimega runs on 64-bit Linux. Release packages and the Docker image are built
for amd64 only. It drives KVM through QEMU and Open vSwitch for networking, so
the host must have:

- Hardware virtualization enabled in firmware, with the `kvm` module and
  `kvm_intel` or `kvm_amd` loaded. If no `kvm` module is present, minimega
  warns at startup.
- A kernel of 3.18 or later. At startup, minimega makes sure of this.
  Containers need OverlayFS, which arrived in 3.18.
- Root, or equivalent capabilities. minimega creates taps and bridges, mounts
  disk images, and manages cgroups. If it does not run as root, it logs a
  warning.

If the host is itself a virtual machine, you need nested virtualization. Before
you install anything, make sure that the CPU exposes it:

```bash
cat /sys/module/kvm_intel/parameters/nested   # Y or 1 on Intel
cat /sys/module/kvm_amd/parameters/nested     # Y or 1 on AMD
virt-host-validate                            # from libvirt-clients, optional
```

Size the host for the experiments you plan to run. Each VM defaults to 2048 MB
of memory and one vCPU (`vm config memory`, `vm config vcpus`). VMs in the
default snapshot mode get a copy-on-write disk under the base directory,
`/tmp/minimega` by default. If `/tmp` is a tmpfs, those snapshots live in RAM.
For more than a handful of VMs, move the base directory to a real filesystem
with `-base` (see [Running minimega](running.md)).

## Install from packages

Every release on
[GitHub Releases](https://github.com/sandia-minimega/minimega/releases) carries
a `.deb` for Debian and Ubuntu and an `.rpm` for RHEL-family distributions. It
also carries a `-binaries.tar.gz` archive of the built tree and a Python wheel.
Both packages install the same tree:

| Path | Contents |
|---|---|
| `/opt/minimega/bin/` | every built tool: `minimega`, `miniweb`, `miniccc`, `minirouter`, `vmbetter`, `protonuke`, `minitest`, `vncdrone`, `rfbplay` and the rest, plus `miniccc.exe` and `protonuke.exe` for Windows guests |
| `/usr/bin/minimega`, `/usr/bin/miniweb`, `/usr/bin/protonuke` | symlinks into `/opt/minimega/bin` |
| `/opt/minimega/misc/daemon/` | `minimega.service`, `minimega.conf` and the legacy `minimega.init` |
| `/etc/minimega/minimega.conf` | symlink to `/opt/minimega/misc/daemon/minimega.conf`, the environment file the service reads |
| `/opt/minimega/misc/vmbetter_configs/` | vmbetter configurations and overlays |
| `/opt/minimega/lib/minimega.py` | the generated Python bindings |
| `/opt/minimega/doc/`, `/opt/minimega/web/` | this documentation and the miniweb web root |
| `/usr/share/bash-completion/completions/minimega` | bash completion, generated with `minimega -completion bash` |

The post-install script creates a `minimega` system user and group and gives
them ownership of `/opt/minimega` and `/etc/minimega`. Members of the group
can talk to the daemon without root. See
[Running minimega](running.md).

### Debian and Ubuntu

```bash
VERSION=3.4.0
sudo apt install "./minimega_${VERSION}_amd64.deb"
```

The package depends on `qemu-system-x86` (or `qemu-kvm`), `qemu-utils`,
`openvswitch-switch`, `dnsmasq`, `dosfstools` and `ntfs-3g`, so `apt` pulls in
the runtime programs. The runtime `libpcap0.8` comes from the shared-library
dependencies. Every Debian system includes `iproute2`. If you use
`tap create ... dhcp`, add `isc-dhcp-client`. If you use `deploy`, add
`openssh-client`.

The `.deb` installs and enables the systemd unit but does not start it. Run
`sudo systemctl start minimega`.

### RHEL, Rocky, Alma and Fedora

```bash
VERSION=3.4.0
sudo dnf install epel-release            # ntfs-3g comes from EPEL
sudo dnf install "./minimega-${VERSION}-1.x86_64.rpm"
```

The RPM requires `qemu-kvm`, `qemu-kvm-common`, `qemu-img`, `dnsmasq`,
`dosfstools`, `ntfs-3g`, `net-tools` and `libpcap`. It does
not require Open vSwitch. Open vSwitch is not in the base RHEL repositories.
Install the `openvswitch` package that your distribution provides. The CentOS
NFV SIG publishes it for RHEL-family systems. Before you start minimega, start
Open vSwitch with `systemctl enable --now openvswitch`.

The RPM installs the unit as `/usr/lib/systemd/system/minimega.service`. Its
post-install script starts the service immediately. To start the service at
boot, run `sudo systemctl enable minimega`. If the binary cannot find
`libpcap.so.0.8`, the same script links `libpcap.so.0.8` to the installed
`libpcap.so`.

On RHEL-family hosts QEMU is installed as `/usr/libexec/qemu-kvm` and nothing
named `kvm` is on `PATH`. minimega runs `kvm` unless you tell it otherwise.
Before you launch VMs, either link it
(`sudo ln -s /usr/libexec/qemu-kvm /usr/local/bin/kvm`) or set
`vm config qemu /usr/libexec/qemu-kvm`.

### The binaries archive

`minimega-<version>-binaries.tar.gz` is the built source tree: `bin/`, `misc/`,
`lib/`, `web/` and `doc/`. Unpack it anywhere and run `sudo ./bin/minimega` from
the top-level directory. Nothing is installed, so you must arrange the runtime
dependencies below and the service files under `misc/daemon/`.

## The Docker image

Every push to `master` and every release builds
`ghcr.io/sandia-minimega/minimega`. The image bundles minimega, its runtime
dependencies, miniweb, the Python bindings and this documentation on top of
Ubuntu 22.04.

| Tag | Meaning |
|---|---|
| `latest`, `3.4.0`, `3.4`, `3` | the newest release and its version aliases |
| `master` | the current development branch |
| `<short commit sha>` | one build of one commit |

```bash
docker pull ghcr.io/sandia-minimega/minimega:latest
```

The container must run privileged with `/dev` and `/lib/modules` from the host,
because Open vSwitch and QEMU run inside it. [Running in Docker](docker.md)
has the full `docker run` command, the environment variables and the host
wrapper script.

## The Python bindings

The [`minimega` package on PyPI](https://pypi.org/project/minimega/) is the
same generated `minimega.py` that the packages install under
`/opt/minimega/lib/`. It talks to a running daemon over its command socket, so
install it wherever your scripts run:

```bash
python3 -m pip install minimega
```

Match the module version to the daemon. If the two were built from different
revisions, `minimega.connect()` prints a warning. See
[Python bindings](python.md).

## Build from source

You need [Go](https://go.dev/dl/) 1.24 or later, a C compiler, and the libpcap
headers. If you want the Python source distribution, you also need Python 3:

```bash
sudo apt install build-essential libpcap-dev     # Debian/Ubuntu
git clone https://github.com/sandia-minimega/minimega.git
cd minimega
git checkout "$(git describe --tags "$(git rev-list --tags --max-count=1)")"   # latest release; skip for tip
./scripts/all.bash
```

`scripts/all.bash` runs `check.bash` (gofmt and go vet), `build.bash`,
`test.bash`, and `doc.bash` in turn. If you only want binaries, use
`./scripts/build.bash` on its own. The build does these steps:

- It installs every tool under `cmd/` into `bin/`.
- It always cross-compiles `miniccc.exe` and `protonuke.exe` for Windows
  guests. The Go toolchain needs no extra setup for that.
- It collects the licenses of the Go modules into `LICENSES/go/` with
  `scripts/licenses.bash`.
- It regenerates `lib/minimega.py` with `pyapigen`.
- If `python3` is available, it builds the Python source distribution.

The Go toolchain downloads the Go modules into the module cache on the first
build. `scripts/env.bash` points `GOBIN` at the `bin/` directory of the
repository. Before you run `go` commands by hand, source it. To build without
network access, see [Dependencies](developer/dependencies.md#building-offline).

On macOS or Windows, use the development container under `.devcontainer/`. It
provides the Linux toolchain for building and unit tests. It cannot run
minimega, because KVM, Open vSwitch, and cgroups need a real Linux host.

Run the result with `sudo ./bin/minimega` from the repository root. To run it
as a service, install `misc/daemon/minimega.service` and `minimega.conf` as
described in [Running minimega](running.md). The unit expects the binary at
`/usr/bin/minimega`.

## Runtime dependencies

minimega calls external programs rather than linking to them. At startup, and
whenever you run [`check`](../reference/minimega.md#check), minimega looks for
each of them in `PATH`. It makes sure that each program has at least the
minimum version and that Open vSwitch is running. A missing program matters
only for the features that use it.

| Program | Debian/Ubuntu package | RHEL-family package | Used for |
|---|---|---|---|
| `kvm` (QEMU 1.6 or later) | `qemu-kvm` (pulls in `qemu-system-x86`) | `qemu-kvm` | KVM VMs |
| `qemu-img`, `qemu-nbd` | `qemu-utils` | `qemu-img` | snapshots, the `disk` commands, file injection |
| `ovs-vsctl`, `ovs-ofctl` (1.11 or later) | `openvswitch-switch` | `openvswitch` | every bridge, tap and VLAN |
| `ip`, `tc` | `iproute2` | `iproute`, `iproute-tc` | interfaces, `qos` |
| `dnsmasq` (2.73 or later) | `dnsmasq` | `dnsmasq` | the `dnsmasq` DHCP and DNS servers |
| `dhclient` | `isc-dhcp-client` | `dhcp-client` | `tap create ... dhcp` |
| `ssh`, `scp` | `openssh-client` | `openssh-clients` | `deploy` |
| `ntfs-3g` | `ntfs-3g` | `ntfs-3g` (EPEL) | injecting files into NTFS images |
| `mount`, `blockdev`, `lsmod`, `modprobe`, `taskset`, `cp`, `tar` | `util-linux`, `kmod`, `coreutils`, `tar` | `util-linux`, `kmod`, `coreutils`, `tar` | file injection, `optimize`, `tar:` paths |
| libpcap (shared library) | `libpcap0.8` | `libpcap` | captures. The binary does not start without it |
| `emulator`, `adb` | Android SDK | Android SDK | [Android VMs](android.md) |
| a TPM emulator such as `swtpm` | `swtpm` | `swtpm` | `vm config tpm-socket` (minimega attaches to a socket you provide) |

The smallest useful set is QEMU, Open vSwitch and iproute2, which gives you KVM
VMs on private networks. Add dnsmasq to hand out addresses from the host.

vmbetter builds guest images. It needs its own tools: `debootstrap`, `chroot`,
`bash`, `cpio`, `mksquashfs` (`squashfs-tools`), `genisoimage`, `extlinux`
(`syslinux`), and `sfdisk`. It also needs `mkfs.ext3` (`e2fsprogs`), `qemu-img`,
`qemu-nbd`, and `dd`. See [Building images with vmbetter](vmbetter.md).

## Host preparation

### Kernel modules and services

- When virtualization is enabled, `kvm` with `kvm_intel` or `kvm_amd` loads
  automatically. To make sure that the modules loaded, run `lsmod | grep kvm`.
- The Open vSwitch service loads `openvswitch`. Before you start minimega,
  start and enable that service (`openvswitch-switch` on Debian and Ubuntu,
  `openvswitch` on RHEL-family systems). The startup check runs `ovs-vsctl`.
  If the service is not running, the check reports
  `openvswitch does not appear to be running`.
- minimega loads `nbd` itself with `modprobe nbd max_part=10`. It does this
  the first time it injects files into a disk image. When vmbetter builds
  disk images, it does the same. If something else loaded `nbd` earlier without
  `max_part`, minimega logs `no max_part parameter set for module nbd` and
  partitions inside images do not appear. To apply the parameter however the
  module is loaded, put `options nbd max_part=10` in a file under
  `/etc/modprobe.d/`.

### Containers need cgroup v1

Container VMs use the `freezer`, `memory`, `devices` and `cpu` controllers. They
use them as separate cgroup v1 hierarchies under the path given by `-cgroup`
(`/sys/fs/cgroup` by default). Some distributions boot with the unified cgroup
v2 hierarchy: Debian 11 and later, Ubuntu 21.10 and later, and RHEL 9 and later.
These distributions do not provide those directories. On them, the first
`vm launch container` fails with a cgroup error. Boot the host with

```text
systemd.unified_cgroup_hierarchy=0
```

on the kernel command line. systemd 256 and 257 also need
`SYSTEMD_CGROUP_ENABLE_LEGACY_FORCE=1` there. systemd 258 and later have no
cgroup v1 support, so this procedure does not work on those hosts. Run
`systemctl --version` to see the version. If `/sys/fs/cgroup/memory` is still
missing after the reboot, the kernel has the memory controller disabled. In that
case, also add `cgroup_enable=memory`. On Debian and Ubuntu, put the parameters
in `GRUB_CMDLINE_LINUX_DEFAULT` in `/etc/default/grub` and run `update-grub`. On
RHEL-family systems, use `grubby --update-kernel=ALL --args="..."`.

KVM VMs are unaffected. If you never launch containers, skip this.
[Virtual machine types](vmtypes.md) explains what minimega does with each
controller.

### Services that fight over the bridge

Installing the `dnsmasq` package on Debian and Ubuntu enables a system-wide
dnsmasq that listens on every interface. For each `dnsmasq start`, minimega
starts its own dnsmasq on the tap address. That instance can fail to bind while
the system dnsmasq holds the port. Disable the system service with
`sudo systemctl disable --now dnsmasq`. minimega needs only the binary.

On desktop hosts, NetworkManager can take over `mega_bridge` and the `mega_tap*`
interfaces that minimega creates, and it can run DHCP on them. Tell
NetworkManager to leave them alone in
`/etc/NetworkManager/conf.d/minimega.conf`:

```ini
[keyfile]
unmanaged-devices=interface-name:mega_bridge;interface-name:mega_tap*
```

Then run `sudo systemctl reload NetworkManager`.

### Firewall

Cluster nodes talk to each other on TCP and UDP port 9000 (`-port`). Discovery
uses UDP broadcast on the same port. miniweb listens on 9001. VXLAN
tunnels between nodes use UDP port 4789. Open those ports between nodes and no
other ports. [Security considerations](security.md)
covers what we recommend that you do not expose.

## Older releases

Releases that predate GitHub Releases remain on the legacy Google Cloud Storage
bucket. They are kept for reproducing old experiments and receive no updates.

| Version | Debian package | Binaries archive |
| --- | --- | --- |
| 2.7 | [minimega-2.7.deb](https://storage.googleapis.com/minimega-files/minimega-2.7.deb) | [minimega-2.7.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.7.tar.bz2) |
| 2.6 | [minimega-2.6.deb](https://storage.googleapis.com/minimega-files/minimega-2.6.deb) | [minimega-2.6.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.6.tar.bz2) |
| 2.5 | [minimega-2.5.deb](https://storage.googleapis.com/minimega-files/minimega-2.5.deb) | [minimega-2.5.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.5.tar.bz2) |
| 2.4 | [minimega-2.4.deb](https://storage.googleapis.com/minimega-files/minimega-2.4.deb) | [minimega-2.4.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.4.tar.bz2) |
| 2.3 | [minimega-2.3.deb](https://storage.googleapis.com/minimega-files/minimega-2.3.deb) | [minimega-2.3.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.3.tar.bz2) |
| 2.2 | [minimega-2.2.deb](https://storage.googleapis.com/minimega-files/minimega-2.2.deb) | [minimega-2.2.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.2.tar.bz2) |
| 2.1 | [minimega-2.1.deb](https://storage.googleapis.com/minimega-files/minimega-2.1.deb) | [minimega-2.1.tar.bz2](https://storage.googleapis.com/minimega-files/minimega-2.1.tar.bz2) |

## Next steps

- [Quickstart](quickstart.md): build an image and boot two VMs.
- [Running minimega](running.md): the service, the environment file and every
  flag.
- [Running in Docker](docker.md): the container image in detail.

## See also

- [Cluster setup](cluster.md)
- [Troubleshooting](troubleshooting.md)
- [Reference: `check`](../reference/minimega.md#check)
