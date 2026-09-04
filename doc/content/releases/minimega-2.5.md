# minimega 2.5 release notes

<a id="TOC_1."></a>

## Introduction

The minimega team is pleased to announce the release of minimega 2.5. This
release includes various improvements and numerous bug fixes. This release
contains changes to the existing API which will improve user experience and
programmability.

<a id="TOC_2."></a>

## What's new

<a id="TOC_2.1."></a>

### Major changes and milestones

<a id="TOC_2.1.1."></a>

#### cc synchronization

miniccc and the cc server now synchronize with magic bytes ("RON") before their
handshake so that extra data from a previous connection can be flushed from
their buffers. This allows VMs to be shutdown/`vm`start`'d and reconnect to the
cc server. Because of the way minimega connects to the client, we only attempt
to reconnect after a `vm start`. If the VM restarts (i.e. with the in-guest
restart mechanism), we currently do not detect this to attempt a reconnect.

This changes requires users to update the miniccc binaries in their VMs.

PR [#1177](https://github.com/sandia-minimega/minimega/v2/pull/1177).

<a id="TOC_2.1.2."></a>

#### cc mount API

Extended miniccc to expose the guest filesystem to the host over the miniccc
connection. This allows the filesystem to be mounted on the host or across the
network on the head node.

The [command and control](../articles/tutorials/cc.md) article has been
updated to show its use.

PR [#1108](https://github.com/sandia-minimega/minimega/v2/pull/1108).

<a id="TOC_2.1.3."></a>

#### ns snapshot API

Added `ns snapshot` which calls `vm migrate` on all VMs in the namespace and
writes a launch script to recreate those VMs from the migration files.

This capability has very limited testing.

PR [#1081](https://github.com/sandia-minimega/minimega/v2/pull/1081).

<a id="TOC_2.1.4."></a>

#### read API

The `read` API now records the namespace that is active at the beginning of the
read command and prepends it to all commands. To allow read scripts to change
the namespace, the `read` API inspects commands and updates the namespace
accordingly. This prevents issues where multiple read scripts run
simultaneously in different namespaces.

The `read` API also reports the line number when there is a parse error.

PRs [#977](https://github.com/sandia-minimega/minimega/v2/pull/977) and
[#1116](https://github.com/sandia-minimega/minimega/v2/pull/1116).

<a id="TOC_2.1.5."></a>

#### -namespace flag

Added `-namespace` flag to specify a namespace to use when running commands for
the `-attach` and `-e` flags.

PR [#970](https://github.com/sandia-minimega/minimega/v2/pull/970).

<a id="TOC_2.2."></a>

### Additional new features

<a id="TOC_2.2.1."></a>

#### tap mirror API

Added `tap mirror` API to create a mirror of a tap. This allows another VM to
inspect the traffic from an experiment. See the
[article](../articles/mirror.md) for an example.

PR [#1118](https://github.com/sandia-minimega/minimega/v2/pull/1118).

<a id="TOC_2.2.2."></a>

#### log ring API

Added `log ring` API which tracks the most recent log messages in an in-memory
ring buffer. The log ring can be created with an arbitrary size and can be
dumped by calling `log ring` with no arguments.

PR [#1121](https://github.com/sandia-minimega/minimega/v2/pull/1121).

<a id="TOC_2.2.3."></a>

#### Improved QEMU integration

Added `vm config cores` and `vm config machine` APIs allowing users to specify
the number of cores and the machine type. The acceptable machine types, CPUs,
and network drivers are based on what the binary `vm config qemu` reports.

PR [#1070](https://github.com/sandia-minimega/minimega/v2/pull/1070).

<a id="TOC_2.2.4."></a>

#### tar: prefix

Added support for `tar:` prefix which fetches and untars tarballs via meshage.
Currently only untars if there is a single top-level directory. It untars to
the same directory that contains the tarball.

PR [#1130](https://github.com/sandia-minimega/minimega/v2/pull/1130).

<a id="TOC_2.2.5."></a>

#### `file delete <GLOB>`

Added glob support to `file delete`.

PR [#972](https://github.com/sandia-minimega/minimega/v2/pull/972).

<a id="TOC_2.2.6."></a>

#### vnc API

Added support for running commands against multiple VMs in the same VNC
command with an API similar to `vm start`.

PR [#1158](https://github.com/sandia-minimega/minimega/v2/pull/1158).

<a id="TOC_2.2.7."></a>

#### cc filter API

Added support for all `vm info` fields for `cc filter` such as:

```text
cc filter name=server
cc filter vlan=DMZ
```

PR [#1161](https://github.com/sandia-minimega/minimega/v2/pull/1161).

<a id="TOC_2.2.8."></a>

#### qemu

Trim `-balloon` flag from the default QEMU args.

PR [#1165](https://github.com/sandia-minimega/minimega/v2/pull/1165).

<a id="TOC_2.2.9."></a>

#### noVNC

Upgraded noVNC to v1.0.0.

PR [#1110](https://github.com/sandia-minimega/minimega/v2/pull/1110).

<a id="TOC_2.2.10."></a>

#### Documentation updates

Added several new articles:

- [Connecting to the Internet](../articles/nat.md)
- [Building a new VM](../articles/newvm.md)
- [Python bindings](../articles/python.md)
- [Network Troubleshooting](../articles/troubleshooting.md)

Removed Vyatta article (deprecated API removed in v2.4).￼

<a id="TOC_2.3."></a>

### Auxiliary Tools

<a id="TOC_2.3.1."></a>

#### miniweb

Added support for namespaces to miniweb is several ways. First, users may now
force a namespace by starting miniweb with the `-namespace` flag. Second, when
miniweb is not forced into a namespace, users may specify namespaces in the
URL. For example, http://localhost:9001/foo/vms will only show VMs in the "foo"
namespace.

Added montage page to show VM screenshots with minimal wrappings.

PRs [#876](https://github.com/sandia-minimega/minimega/v2/pull/876),
[#987](https://github.com/sandia-minimega/minimega/v2/pull/987), and
[#1157](https://github.com/sandia-minimega/minimega/v2/pull/1157).

<a id="TOC_2.3.2."></a>

#### vmbetter

Replaced `fdisk` with `sfdisk` to fix issues with newer versions of `fdisk`.

PR [#1164](https://github.com/sandia-minimega/minimega/v2/pull/1164).

<a id="TOC_2.3.3."></a>

#### igor

Improved show command.

PRs [#1159](https://github.com/sandia-minimega/minimega/v2/pull/1159) and
[#1160](https://github.com/sandia-minimega/minimega/v2/pull/1160).

Added sync command.

PR [#1169](https://github.com/sandia-minimega/minimega/v2/pull/1169).

<a id="TOC_3."></a>

## protonuke

Added support for user-agent strings in HTTP requests and fix a reporting
error with DNS hits/second.

PR [#1172](https://github.com/sandia-minimega/minimega/v2/pull/1172).

<a id="TOC_4."></a>

## Bug fixes

<a id="TOC_4.1."></a>

### vnc API with namespaces

`vnc` API no longer reports `vm not found errors` when there are multiple hosts
in the namespace and only one is running the target VM.

PR [#1125](https://github.com/sandia-minimega/minimega/v2/pull/1125).

<a id="TOC_4.2."></a>

### disk API

Fixed the `disk` API so that it returns an error when the partition is not
specified and the disk has more than one partition.

PR [#1127](https://github.com/sandia-minimega/minimega/v2/pull/1127).

<a id="TOC_4.3."></a>

### vm config coschedule API

Allow `localhost` as value to `vm config coschedule`.

PR [#1134](https://github.com/sandia-minimega/minimega/v2/pull/1134).

<a id="TOC_4.4."></a>

### vm tag API

Fix bug in `vm tag` where tags for all VMs were being shown instead of just
those for the specified target.

PR [#1156](https://github.com/sandia-minimega/minimega/v2/pull/1156).

<a id="TOC_4.5."></a>

### capture API

Fix bug in `capture` where some arguments caused an `unreachable` error.

PR [#1167](https://github.com/sandia-minimega/minimega/v2/pull/1167).

<a id="TOC_5."></a>

## vmbetter images

Added Bro [#1163](https://github.com/sandia-minimega/minimega/v2/pull/1163).

Added minimal Ubuntu [#1179](https://github.com/sandia-minimega/minimega/v2/pull/1179).

<a id="TOC_6."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing](../articles/installing.md) guide for more
information.

<a id="TOC_6.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.5.deb). It is
known to work in debian 7 (wheezy) and 8 (testing/jessie) and ubuntu 16.04.

<a id="TOC_6.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
[here](https://storage.googleapis.com/minimega-files/minimega-2.5.tar.bz2).
It should be sufficient to simply unpack the tarball and run tools from the
`bin/` directory directly. Most default paths in minimega, vmbetter, and other
tools are built to be used in this way (i.e. `bin/minimega`, which will then
look for the web directory in `misc/web`).

<a id="TOC_6.3."></a>

### Building from source

Source of the entire distribution is available on
[Github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.8+ compiler and libpcap headers.
