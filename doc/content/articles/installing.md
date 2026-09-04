# Installing minimega


<a id="TOC_1."></a>

## Obtaining minimega

Install minimega from a GitHub release, build it from source, or run the
container image. Release packages are the simplest choice for a native
installation; source builds provide the latest development version.

<a id="TOC_1.1."></a>

### GitHub release

Open the
[latest GitHub release](https://github.com/sandia-minimega/minimega/releases/latest)
and download the appropriate asset:

- Install the `.deb` package on Debian-family AMD64 systems.
- Install the `.rpm` package on RPM-based x86-64 systems.
- Use the `-binaries.tar.gz` archive for a portable prebuilt distribution.
- Use the `.whl` file when only the Python client package is needed.

Install a downloaded Debian or RPM package with the distribution package
manager:

```bash
VERSION="<release>"
sudo apt install "./minimega_${VERSION}_amd64.deb"
sudo dnf install "./minimega-${VERSION}-1.x86_64.rpm"
```

Alternatively, unpack the binaries archive and run minimega from its top-level
directory:

```bash
VERSION="<release>"
tar xzf "minimega-${VERSION}-binaries.tar.gz"
cd minimega
sudo ./bin/minimega
```

The [downloads page](download.md) links to previous and legacy releases.

<a id="TOC_1.2."></a>

### Building from source

To build from source you will need [Go](http://golang.org) (version 1.24 or
later) and libpcap headers. On a Debian-type system, you can install
compile-time dependencies with:

```bash
sudo apt-get install libpcap-dev
```

Having installed the dependencies, grab the minimega source:

```bash
git clone https://github.com/sandia-minimega/minimega.git
cd minimega
```

Next, check out the latest release. If you wish to run the development version
("tip") of minimega, skip this command.

```bash
git fetch --tags
git checkout "$(git describe --tags "$(git rev-list --tags --max-count=1)")"
```

Finally, compile minimega:

```bash
./scripts/all.bash
```

This will build and test each of the libraries and tools in the minimega
distribution and create a `bin/` subdirectory containing each of the minimega
tools. If you have a Windows cross compiler for Go set up, it will also build
Windows binaries of several tools.

### Docker container

The container image includes minimega, its runtime dependencies, miniweb, and
this documentation. It requires Linux host access to KVM, Open vSwitch, devices,
and networking, so run it as a privileged container:

```bash
docker run -d \
  --name minimega \
  --hostname minimega \
  --privileged \
  --cap-add ALL \
  -p 9000:9000/udp \
  -p 9001:9001 \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /var/log/minimega:/var/log/minimega \
  -v /tmp/minimega:/tmp/minimega \
  --health-cmd "mm version" \
  ghcr.io/sandia-minimega/minimega:master
```

See the
[Docker guide](https://github.com/sandia-minimega/minimega/blob/master/docker/README.md)
for image builds, Docker Compose, host wrappers, configuration, and Open
vSwitch integration.

<a id="TOC_2."></a>

## Deploying minimega

minimega is a single binary and needs no configuration files. However, because
minimega makes use of external programs, you'll need to have some things
installed--see the section "System requirements and runtime dependencies"
below.

To deploy minimega to any number of nodes, simply copy the binary to each node.
See [the usage article](usage.md) for information about launching
minimega.

Depending on your cluster configuration, it is also possible to have minimega
deploy itself. By launching minimega on a single node, you can use the `deploy`
API which will cause minimega to copy itself and run remotely using `ssh` on a
provided list of nodes. See the [API documentation](../reference/minimega.md) on `deploy`
for more information, or read the [article on setting up a cluster](cluster.md).

<a id="TOC_2.1."></a>

### System requirements and runtime dependencies

minimega is designed to be simple to deploy. It has only one runtime
dependency, libpcap, which is included on almost all standard Linux distros.

To launch containers, the kernel must support OverlayFS, which was added in
Linux 3.18.

minimega also has a number of external tools it executes. When you start
minimega, it will check to see if each of the tools it may need are available
in `$PATH`. Depending on your intended use case, you may not need every single
external program.

If you plan to launch and maintain VMs, you'll need the following programs at a
minimum:

- kvm - qemu-kvm with the kvm kernel module loaded (minimum version 1.6)
- ip - ip tool for manipulating devices
- ovs-vsctl - Open vSwitch switch control with daemon running and kernel module loaded (minimum version 1.11)
- ovs-ofctl - Open vSwitch openflow control with daemon running and kernel module loaded

We also recommend installing the following; they are not strictly necessary for
basic VM use but are required for some more advanced operations:

- dhclient - dhcp client
- dnsmasq - DNS and DHCP server (minimum version 2.73)
- qemu-nbd/qemu-img - tool for interacting with qemu disk images (in the Debian package "qemu-tools")
- mkdosfs - used when creating router images
- taskset - set CPU affinity for VMs
- ntfs-3g - NTFS with write support for injecting files into NTFS images
- ssh/scp - used to deploy minimega to other nodes in a cluster
- tc - used by QoS API to set latency and bandwidth for VMs

The following debian packages should install most of the dependencies:

```text
openvswitch-switch qemu-kvm qemu-utils dnsmasq ntfs-3g iproute
```

<a id="TOC_2.1.1."></a>

#### Grub note

If you intend to run Linux containers, you need to have the `memory` cgroup
enabled, but Debian (and some other distros) do not enable it by default. If
you try to start a container and get an error, you may need to enable the
memory cgroup.

To enable it, add the following to your kernel boot parameters:

```text
cgroup_enable=memory
```

On Debian, you can do this by opening /etc/default/grub and adding that
parameter to the `GRUB_CMDLINE_LINUX_DEFAULT` line. It should end up looking
something like this:

```text
GRUB_CMDLINE_LINUX_DEFAULT="quiet cgroup_enable=memory"
```

Then run update-grub and reboot for the change to take effect.

<a id="TOC_3."></a>

## Getting help

Use the [GitHub issue tracker](https://github.com/sandia-minimega/minimega/issues)
to report bugs or request help.
