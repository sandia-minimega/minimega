# minimega 2.6 release notes

<a id="TOC_1."></a>

## Introduction

The minimega team is pleased to announce the release of minimega 2.6. This
release includes many new features, improvements, and bug fixes. This release
contains changes to the existing API which will improve user experience and
programmability.

<a id="TOC_2."></a>

## What's new

<a id="TOC_2.1."></a>

### Major changes and milestones

<a id="TOC_2.1.1."></a>

#### tap mirror API

Allows users to create mirrors between VM taps so that VMs can monitor the
traffic of other VMs.

PR [#1221](https://github.com/sandia-minimega/minimega/v2/pull/1221).

<a id="TOC_2.1.2."></a>

#### vm config colocate API

To support `tap mirror` which needs VMs to be scheduled on the same host, we
added the `vm config colocate` API to specify that the new VM should be
scheduled on the same host as an existing VM.

PR [#1222](https://github.com/sandia-minimega/minimega/v2/pull/1222).

<a id="TOC_2.1.3."></a>

#### Disk snapshots

Remove the `-snapshot` flag from the qemu args and instead create snapshots of
the disks when the VM first launches. These are saved in the VM instance
directory and can be saved in the future. minimega now warns if you configure a
VM with more than 4 IDE disks.

PRs [#1284](https://github.com/sandia-minimega/minimega/v2/pull/1284),
[#1302](https://github.com/sandia-minimega/minimega/v2/pull/1302),
[#1347](https://github.com/sandia-minimega/minimega/v2/pull/1347).

<a id="TOC_2.1.4."></a>

#### GRE meshes for namespace

Automatically creates GRE or VXLAN tunnels on a separate bridge for a
namespace. Very experimental and subject to change.

PR [#1263](https://github.com/sandia-minimega/minimega/v2/pull/1263).

<a id="TOC_2.2."></a>

### Additional new features

<a id="TOC_2.2.1."></a>

#### minicli improvements

Reduce the overhead for minicli to parse commands.

PR [#1184](https://github.com/sandia-minimega/minimega/v2/pull/1184).

<a id="TOC_2.2.2."></a>

#### miniclient redial

Too many minimega -e's can cause minimega's listen queue to fill up and start
rejecting new miniclients. Add redial/backoff when we detect a temporary error.

PR [#1185](https://github.com/sandia-minimega/minimega/v2/pull/1185).

<a id="TOC_2.2.3."></a>

#### optimize API

Make the optimize API mostly work with namespaces. `hugepages` and `affinity`
can be set per namespace. `ksm` is set globally.

PR [#1199](https://github.com/sandia-minimega/minimega/v2/pull/1199).

<a id="TOC_2.2.4."></a>

#### file stream API

Add API to stream a file from the iomeshage directory. This is used by miniweb
to serve files from this directory back to users via the browser. Note that due
to the way minimega processes commands, this works for small files but large
files will cause minimega to wait until the file is fully downloaded before
continuing.

PR [#1126](https://github.com/sandia-minimega/minimega/v2/pull/1126).

<a id="TOC_2.2.5."></a>

#### vm config vga, vm config sockets, vm config threads APIs

Add new APIs to set additional QEMU parameters.

PR [#1240](https://github.com/sandia-minimega/minimega/v2/pull/1240).

<a id="TOC_2.2.6."></a>

#### ns dry-run API

Dry run of the scheduler that prints out VM placement. User can then edit the
placement as needed before running it. Rename ns schedules to ns schedule
status. Fix bug in schedule status where launched VMs weren't being counted
properly.

PR [#1247](https://github.com/sandia-minimega/minimega/v2/pull/1247).

<a id="TOC_2.2.7."></a>

#### vnc API

Many improvements to the underlying code. Add `WaitForIt` and `ClickIt` events
to vnc playback. Uses a template image to wait for a something to appear on the
screen (`WaitForIt`) and then click the center of it (`ClickIt`).

PRs [#1267](https://github.com/sandia-minimega/minimega/v2/pull/1267),
[#1280](https://github.com/sandia-minimega/minimega/v2/pull/1280).

<a id="TOC_2.2.8."></a>

#### Instance symlinks

minimega now creates symlinks so that users can reference VMs in the minimega
directory by namespace and UUID.

PRs [#1287](https://github.com/sandia-minimega/minimega/v2/pull/1287),
[#1338](https://github.com/sandia-minimega/minimega/v2/pull/1338).

<a id="TOC_2.2.9."></a>

#### vm cdrom API

Add "force" option to the eject API.

PR [#1272](https://github.com/sandia-minimega/minimega/v2/pull/1272).

<a id="TOC_2.2.10."></a>

#### vm config virtio-ports API

Users can now specify a list of named virtio ports or a number of virtio
ports to automatically generate names for (old behavior).

PR [#1296](https://github.com/sandia-minimega/minimega/v2/pull/1296).

<a id="TOC_2.2.11."></a>

#### VM names

minimega now sanitizes VM names since it creates directories and argument
strings using them.

PRs [#1304](https://github.com/sandia-minimega/minimega/v2/pull/1304),
[#1315](https://github.com/sandia-minimega/minimega/v2/pull/1315).

<a id="TOC_2.2.12."></a>

#### deploy API

Add subcommands to specify files to write `stdout` and `stderr` to.

PR [#1263](https://github.com/sandia-minimega/minimega/v2/pull/1263).

<a id="TOC_2.2.13."></a>

#### bridge API

Add subcommand to configure bridge. Add `key` option to differentiate tunnels.

PR [#1263](https://github.com/sandia-minimega/minimega/v2/pull/1263).

<a id="TOC_2.2.14."></a>

#### mesh API and host completion

`mesh size` is one when there is only a single node. Add completion for
`mesh send`, `mesh hangup`, `vm config schedule`, `ns add-hosts` and
`ns del-hosts`. Add `mesh list peers` and `mesh list all` subcommands. Resolve
`localhost` for `vm config schedule`.

PR [#1319](https://github.com/sandia-minimega/minimega/v2/pull/1319).

<a id="TOC_2.2.15."></a>

#### cc tunnel API

Allow tunnels to be specified based on VM name or UUID.

PR [#1342](https://github.com/sandia-minimega/minimega/v2/pull/1342).

<a id="TOC_2.2.16."></a>

#### vm launch API

Add option to specify a saved VM config name to launch.

PR [#1326](https://github.com/sandia-minimega/minimega/v2/pull/1326).

<a id="TOC_2.2.17."></a>

#### Output coalescing

minimega now coalescing repeated patterns in output strings as opposed to just
prefixes. For example, `foo1.bar` and `foo2.bar` would coalesce to
`foo[1-2].bar`.

PR [#1327](https://github.com/sandia-minimega/minimega/v2/pull/1327).

<a id="TOC_2.2.18."></a>

#### cc APIs

Shove the command ID into the `Data` field of responses so that scripts can
easily determine which command they issued.

PR [#1346](https://github.com/sandia-minimega/minimega/v2/pull/1346).

<a id="TOC_2.2.19."></a>

#### Dependencies checks

minimega now warns if it does not detect the `kvm` kernel module.

PR [#1348](https://github.com/sandia-minimega/minimega/v2/pull/1348).

<a id="TOC_2.2.20."></a>

#### vm flush API

Allow flushes to occur in parallel, speeding up flushing large experiments.

PR [#1353](https://github.com/sandia-minimega/minimega/v2/pull/1353).

<a id="TOC_2.2.21."></a>

#### Documentation updates

Updated several articles. The layout for articles was updated to include the
header and sidebar.

PRs [#1207](https://github.com/sandia-minimega/minimega/v2/pull/1207),
[#1237](https://github.com/sandia-minimega/minimega/v2/pull/1237),
[#1246](https://github.com/sandia-minimega/minimega/v2/pull/1246).

<a id="TOC_2.3."></a>

### Auxiliary Tools

<a id="TOC_2.3.1."></a>

#### minitest

Sort errors to make tests more reliable.

PR [#1250](https://github.com/sandia-minimega/minimega/v2/pull/1250).

<a id="TOC_2.3.2."></a>

#### igor

Many new features and improvements. `igor` will likely be migrated to a
separate repo with separate release notes during the next release cycle.

<a id="TOC_2.3.3."></a>

#### minirouter

Add support for basic BGP routing.

PR [#1206](https://github.com/sandia-minimega/minimega/v2/pull/1206).

<a id="TOC_2.3.4."></a>

#### vmbetter

Add build constraints to control what gets built in different contexts. Add
option to specify target name. Change `-qcow` to `-disk` and add the option to
specify the disk format (currently allows qcow, qcow2, raw, and vmdk). Rename
`-qcowsize` to `-size`. Change the default mbr location so that it matches
debian.

PRs [#1241](https://github.com/sandia-minimega/minimega/v2/pull/1241),
[#1305](https://github.com/sandia-minimega/minimega/v2/pull/1305),
[#1321](https://github.com/sandia-minimega/minimega/v2/pull/1321).

<a id="TOC_2.3.5."></a>

#### Python bindings

Improve performance using readline. Drop timeout option since we cannot use
readline in non-blocking mode. Add `as_dict` helper.

PRs [#1316](https://github.com/sandia-minimega/minimega/v2/pull/1316),
[#1335](https://github.com/sandia-minimega/minimega/v2/pull/1335).

<a id="TOC_2.3.6."></a>

#### miniweb

Revert noVNC back to previous version due to problems with v1.0.0. Change
`-console` flag to string to allow specifying a path to minimega's domain
socket. Add new VM page to launch a VM from a saved config.

PRs [#1277](https://github.com/sandia-minimega/minimega/v2/pull/1277),
[#1318](https://github.com/sandia-minimega/minimega/v2/pull/1318),
[#1326](https://github.com/sandia-minimega/minimega/v2/pull/1326).

<a id="TOC_3."></a>

## Bug fixes

<a id="TOC_3.1."></a>

### vm volume API

Automatically create the volume source if it does not exist.

PR [#1208](https://github.com/sandia-minimega/minimega/v2/pull/1208).

<a id="TOC_3.2."></a>

### Filesystem does not exist

If a container filesystem does not exist, minimega will now print a more useful
error message.

PR [#1244](https://github.com/sandia-minimega/minimega/v2/pull/1244).

<a id="TOC_3.3."></a>

### Clogged containers

Fix issues with container filesystems failing to unmount.

PR [#1245](https://github.com/sandia-minimega/minimega/v2/pull/1245).

<a id="TOC_3.4."></a>

### Quoting with minimega -e

Fix an issue with quoted commands.

PR [#1294](https://github.com/sandia-minimega/minimega/v2/pull/1294).

<a id="TOC_3.5."></a>

### vm qmp API

The API now attempts to unmarshal the JSON object before sending it to QMP to
prevent malformed JSON from borking the connection.

PR [#1273](https://github.com/sandia-minimega/minimega/v2/pull/1273).

<a id="TOC_3.6."></a>

### vm config schedule API

minimega will now return an error if the host is not in the namespace at the
time `vm config schedule` is called.

PR [#1303](https://github.com/sandia-minimega/minimega/v2/pull/1303).

<a id="TOC_3.7."></a>

### Properly handle connection errors

Fix a rare crash in minimega due to connection errors.

PR [#1325](https://github.com/sandia-minimega/minimega/v2/pull/1325).

<a id="TOC_3.8."></a>

### cc response API

Fix missing responses when using prefixes.

PR [#1341](https://github.com/sandia-minimega/minimega/v2/pull/1341).

<a id="TOC_3.9."></a>

### vm config cpu API

Fix bug where we ignored the first line of output in versions where QEMU does
not include a header.

PR [#1328](https://github.com/sandia-minimega/minimega/v2/pull/1328).

<a id="TOC_3.10."></a>

### vm info API

Fix a bug where the VM PID was not copied to the head node.

PR [#1352](https://github.com/sandia-minimega/minimega/v2/pull/1352).

<a id="TOC_3.11."></a>

### .alias API

Fix double expansion and only replace the first full word on a line.

PR [#1362](https://github.com/sandia-minimega/minimega/v2/pull/1362).

<a id="TOC_4."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing](../articles/installing.md) guide for more
information.

<a id="TOC_4.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.6.deb). It is
known to work in debian 9 (stretch) and ubuntu 16.04. It is known not to work
on debian 10 (buster).

<a id="TOC_4.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
[here](https://storage.googleapis.com/minimega-files/minimega-2.6.tar.bz2).
It should be sufficient to simply unpack the tarball and run tools from the
`bin/` directory directly. Most default paths in minimega, vmbetter, and other
tools are built to be used in this way (i.e. `bin/minimega`, which will then
look for the web directory in `misc/web`).

<a id="TOC_4.3."></a>

### Building from source

Source of the entire distribution is available on
[Github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.10+ compiler and libpcap headers.
