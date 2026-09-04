# minimega 2.4 release notes

<a id="TOC_1."></a>

## Introduction

The minimega team is pleased to announce the release of minimega 2.4. This
release includes improvements to make experiments more portable, an effort that
started in 2.3, and numerous bug fixes. This release contains changes to the
existing API which will improve user experience and programmability.

<a id="TOC_2."></a>

## What's new

<a id="TOC_2.1."></a>

### Major changes and milestones

<a id="TOC_2.1.1."></a>

#### miniplumber

Add plumbing support to minimega and miniccc to facilitate non-networked
communication between VMs, processes on guests or hosts, and instances of
minimega. See the miniplumber [article](../articles/plumbing.md) and
[presentation](../presentations/miniplumber.md) for details.

<a id="TOC_2.1.2."></a>

#### namespaces

In 2.3, we introduced namespaces as the first step towards more portable
experiments. In 2.4, we have refactored namespaces to be more comprehensive.
minimega is now always in a namespace -- `minimega` by default. All state
associated with a namespace (e.g. VMs, taps, captures, VNC) is now
automatically cleaned up when it is destroyed.

As part of this refactor, we have made several APIs work properly with
namespaces. Specifically, there is now a separate `vm config` per namespace.
`vm config clone` and `vm config qemu-override` now work with namespaces. There
is also a separate `router` and `cc` instances per namespace (including a
separate `cc filter` and `cc prefix`).

One notable change is that `miniccc_responses` are now separated by namespace.
Each namespace includes its own command-and-control server that writes to a
separate directory. This addresses an issue with clients having the same UUIDs
in different namespaces (before, they would have written responses to the same
directory).

<a id="TOC_2.1.3."></a>

#### Scheduler

The scheduler has been vastly improved in this release. In 2.3, we did
round-robin scheduling to distribute VMs across hosts. Now, the scheduler
attempts to load balance based on CPU, memory, or network commit. There are
also two new APIs to fine-tune the scheduler: `vm config schedule` and
`vm config coschedule`. See the [namespaces](../articles/namespaces.md)
article for details.

<a id="TOC_2.1.4."></a>

#### ns API

The `ns` API replaces `nsmod` to configure the active namespace. See `help ns`
for more information.

We added `ns run` to run a command on all nodes in the namespace.

<a id="TOC_2.1.5."></a>

#### capture API

The `capture` API was rewritten and is now much more stable than before. We
have used it with thousands of captures without crashing.

Added new APIs to configure the snaplen and BPF for new PCAP captures. Added
new APIs to configure ASCII or Raw and compression for new netflow captures.
Simplified the `capture netflow` API to use the stored configuration rather
than pass it in the command.

<a id="TOC_2.1.6."></a>

#### container VCPUs

Containers had a VCPUs parameter but no CPU limit was actually enforced. Now,
we use the `cpu` cgroup to set a quota on processing time for the container
based on the value of `vm config vcpus`.

<a id="TOC_2.2."></a>

### Additional new features

<a id="TOC_2.2.1."></a>

#### cc listen API

To support the above changes to `cc`, we removed the `-ccport` flag and
replaced it with the `cc listen` API. This runs on all hosts in the namespace.

`cc listen` must be run manually if users wish to use a network-based
backchannel for command and control.

<a id="TOC_2.2.2."></a>

#### cc log level API

Added new API to change miniccc's log level at runtime: `cc log level`.

It uses the value of `cc filter` to determine which VMs to update.

<a id="TOC_2.2.3."></a>

#### file API

`file delete` now supports globs (e.g. `file delete *.qcow`).

<a id="TOC_2.2.4."></a>

#### router API

Added `router <vm> gw` API to set the routers default gateway.

<a id="TOC_2.2.5."></a>

#### vm top API

Added `vm top` API to show the memory and CPU resources that VMs and containers
are actively using.

<a id="TOC_2.2.6."></a>

#### vm info API

Moved the `bandwidth` column to `vm top`.

Added an `uptime` column for the time since the VM was launched. Added `pid`
column for QEMU process for VMs and for init process for containers.

<a id="TOC_2.2.7."></a>

#### vm config volume API

Added `vm config volume` API to specify additional directories to mount into
the container's filesystem.

<a id="TOC_2.2.8."></a>

#### vm config backchannel API

Added `vm config backchannel` to control whether a network-less backchannel is
created for the VM/container or not. Solved an issue where some VMs only
recognized one virtio port.

This defaults to true so existing scripts do not need to change.

<a id="TOC_2.2.9."></a>

#### vm cdrom API

`vm cdrom` can now address one or more VMs using the same syntax as `vm start`.
This allows users to add or remove disks for multiple VMs in one command.

Modified the QEMU arguments so that VMs have an empty CD device by default.
This allows users to add a CD to all VMs, not just ones that were launched with
a CD in `vm config cdrom`.

<a id="TOC_2.2.10."></a>

#### vm hotplug API

Similarly to the `vm cdrom` API, `vm hotplug` now supports addressing one or
more VMs using the syntax from `vm start`.

`vm hotplug` also now includes two optional parameters: the USB version and USB
serial number. The USB version controls which bus the device is connected to --
either 1.1 or 2.0. The serial number is visible to the VM.

Renamed `vm hotplug show` to `vm hotplug`. To limit results to a particular VM,
use `.filter`.

<a id="TOC_2.2.11."></a>

#### vm net API

Similarly to the `vm cdrom` API, `vm net` now supports addressing one or more
VMs using the syntax from `vm start`.

The bridge parameter is now optional and defaults to the bridge that the tap is
already connected to (or `mega_bridge` if the tap is disconnected).

Changed the parameter order in order to support the above changes.

<a id="TOC_2.2.12."></a>

#### host API

Added many new columns to support the scheduler. See `help host` for details.

<a id="TOC_2.2.13."></a>

#### debug API

Added `debug goroutine` to dump goroutine stack traces to file.

<a id="TOC_2.2.14."></a>

#### clear all API

Added `clear all` API to reset minimega to a vanilla state. Restarting is still
preferred.

<a id="TOC_2.2.15."></a>

#### help API

Added support for sub-command help (e.g. `help vnc record`).

<a id="TOC_2.2.16."></a>

#### vyatta API

Removed deprecated API.

<a id="TOC_2.2.17."></a>

#### web API

Removed API. See replacement, [miniweb](#TOC_2.3.1.).

<a id="TOC_2.2.18."></a>

#### vnc API

Changed the default location for reading and writing recordings to the
iomeshage directory rather than the current directory.

<a id="TOC_2.2.19."></a>

#### .preprocess API

Added `.preprocess` API to disable preprocessor. minimega automatically fetches
files with a `file:` or `http://` prefix -- this API allows you to disable that
preprocessing. For example, `.preprocess false cc exec curl http://...`.

<a id="TOC_2.2.20."></a>

#### .env API

Added `.env` API to print/update/unset environment variables.

<a id="TOC_2.2.21."></a>

#### Apropos for .columns, .filter APIs

Added apropos support for `.columns` and `.filter`. Users can now uses a
distinct prefix for column names rather than the full column name.

<a id="TOC_2.2.22."></a>

#### QEMU flags

Changed the default video driver from cirrus to std.

<a id="TOC_2.2.23."></a>

#### VLANs file

minimega now writes out the VLAN mappings to the filesystem.

<a id="TOC_2.2.24."></a>

#### Tabbed completion

minimega now completes commands when using the `-attach` interface.

Added completion for namespace, tap, and bridge names in supporting APIs.

Added environment variable completions.

<a id="TOC_2.2.25."></a>

#### Header uniformity

Updated the headers on several API to make them easier to use with `.columns`
and `.filter`:

- bridge
- capture
- cc
- debug
- disk
- dnsmasq
- file status
- mesh status
- optimize
- vm hotplug
- vm tag

All column names should now be one word and lowercase.

<a id="TOC_2.2.26."></a>

#### Readline replacement

Replaced the GNU Readline library in minimega with a pure Go implementation,
eliminating a C dependency. Users may notice slightly different behaviors
between the implementations.

<a id="TOC_2.2.27."></a>

#### Vanity URL

Users can now clone minimega via `http://minimega.org/minimega.git` which
redirects to the Github repo.

<a id="TOC_2.2.28."></a>

#### Travis Integration

Added Travis integration to Github. Added new script `check.bash` to ensure
source code meets `gofmt` and `go vet` standards.

<a id="TOC_2.3."></a>

### Auxillary Tools

<a id="TOC_2.3.1."></a>

#### miniweb

Created standalone webserver based on `web` API. Added many
new features, see the [miniweb](../articles/miniweb.md) article for more
information.

<a id="TOC_2.3.2."></a>

#### vmbetter

The vmbetter configs included with minimega have been heavily refactored.
Specifically, we:

- Added new host config for CARNAC.
- Renamed ccc_host_ovs to ccc_host.
- Added new configs to build images with the dependencies to build minimega.
- Renamed miniccc_virtio to miniccc.
- Deleted protonuke.
- Changed ccc_host to set experiment IP from management IP.
- Set motd throughout.
- Added symbolic links to miniccc and minirouter (so that they copy in automatically from bin/).

The last change means that users no longer have to copy binaries into the
overlay directory before building.

<a id="TOC_2.3.3."></a>

#### uminiccc/uminirouterfs

Added a busybox-based container filesystem that includes miniccc based on the
busybox-based minirouter filesystem (renamed to uminirouterfs).

<a id="TOC_2.3.4."></a>

#### igor

igor has had a significant overhaul. It now performs scheduling, rather that
just reservation. Users specify how many nodes they need and for how long; igor
looks through its schedule to find a time when it has enough nodes available
and reserves them then. When the reservation starts, igor will copy in the boot
files as usual. It can now also reboot nodes automatically when the reservation
starts (if desired), and has experimental support to put each reservation in a
different Q-in-Q (802.1ad) network segment to avoid network conflicts. Users
can also make reservations at a specific time in the future rather than next
available, reserve specific nodes rather than the  next available, or ask igor
to show them some available reservation slots without actually creating a
reservation.

<a id="TOC_2.3.5."></a>

#### passwordify

Added new tool to modify credentials for a ramdisk image.

<a id="TOC_2.3.6."></a>

#### vmconfiger

Added new tool to automatically generate the `vm config` APIs. This helps keep
documentation consistent and simplifies adding new `vm config` APIs.

<a id="TOC_2.3.7."></a>

#### protonuke

Added a flag to enable cookie jar for protonuke http and https clients.

Added simple FTP server and client.

Added size query parameter to `image.png` to request image of specified size.
protonuke generates the image on the first request and stores it for future
requests.

<a id="TOC_2.3.8."></a>

#### powerbot

Add IPMI support.

<a id="TOC_2.3.9."></a>

#### minitest

Added recursive mode and new distributed tests.

<a id="TOC_2.3.10."></a>

#### rond

Added new standalone `ron` server that can be used separate from minimega to
provide command-and-control to physical machines. Partially implemented.

<a id="TOC_3."></a>

## Bug fixes

<a id="TOC_3.1."></a>

### containers

minimega now creates `/proc`, `/dev`, `/dev/shm`, `/dev/pts`, `/sys` in the
container filesystem if they do not exist. Solved an issue where containers
would fail to start.

<a id="TOC_3.2."></a>

### bandwidth on VMs

There was a bug where the bandwidth statistics were reversed (Rx and Tx were
swapped). Changed to a weighted moving average to show changes in transfer
rates faster.

<a id="TOC_3.3."></a>

### qos API

Use tbf instead of netem for rate limiting since netem does not seem to behave
correctly between VMs on different hosts. Unfortunately, this means that rate
and loss/delay are mutually exclusive now.

<a id="TOC_3.4."></a>

### miniccc

In 2.3, miniccc added support to set upstream tags using a UDS. This socket was
not properly cleaned up and would prevent miniccc from restarting if the VM
reboots. Changed miniccc so that it deletes the UDS if it does not detect that
there is an instance on miniccc running.

Fix UUID handler for Windows.

<a id="TOC_4."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing](../articles/installing.md) guide for more
information.

<a id="TOC_4.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.4.deb). It is
known to work in debian 7 (wheezy) and 8 (testing/jessie) and ubuntu 16.04.

<a id="TOC_4.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
[here](https://storage.googleapis.com/minimega-files/minimega-2.4.tar.bz2).
It should be sufficient to simply unpack the tarball and run tools from the
`bin/` directory directly. Most default paths in minimega, vmbetter, and other
tools are built to be used in this way (i.e. `bin/minimega`, which will then
look for the web directory in `misc/web`).

<a id="TOC_4.3."></a>

### Building from source

Source of the entire distribution is available on
[Github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.8+ compiler and libpcap headers.
