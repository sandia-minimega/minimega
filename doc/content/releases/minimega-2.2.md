# minimega 2.2 release notes

<a id="TOC_1."></a>

## Introduction

The minimega development team is pleased to announce the release of minimega
2.2. This release includes several key new features, including support for full
system containers, and numerous bugfixes. In addition, several changes to the
existing API have been for improved user experience and programmability.

<a id="TOC_2."></a>

## What's new

<a id="TOC_2.1."></a>

### Major changes and milestones

<a id="TOC_2.1.1."></a>

#### containers

The most notable feature of this release is support for full system containers
as a new VM type. `minimega` can boot a mixture KVM and container based VMs.
Describing container based VMs is identical to KVM type (with some additional
configuration parameters), and all VM lifecycle commands and behaviors are the
same (start, stop, launch, kill, networking, ...). `minimega` uses a custom
container implementation that is very fast and can scale to several thousand
containers on a single node (more than 8000 in our testing).

More information can be found in the [VM types](../articles/vmtypes.md)
article.

<a id="TOC_2.1.2."></a>

#### disk API

The `vm inject` API has been replaced by the new `disk` API. Previously, the
`vm inject` API allowed you to fork disk images and inject files in a single
command. The `disk` API splits this and adds new capabilities such as creating
new images and injecting with special mount options.

<a id="TOC_2.1.3."></a>

#### file: prefix

To make it easier to reference files stored in iomeshage, the meshage-based
file transfer layer provided by minimega, we have added a `file:` prefix that
can be used anywhere minimega expected a file. For example, if a remote node
has a file `foo.qcow2`, and you want to use it locally as a disk image:

```text
vm config disk file:foo.qcow2
```

This will cause minimega to automatically fetch `foo.qcow2` from the remote
node before executing the command. See the [file](../articles/file.md)
guide for more information.

<a id="TOC_2.2."></a>

### Additional new features

<a id="TOC_2.2.1."></a>

#### cc on by default

The command and control layer, `cc`, is now enabled by default over both TCP
and the networkless backchannel options. When booting KVM based VMs, a
virtio-serial device is created by default in `\\.\Global\cc`, and on container
based VMs a UNIX domain socket is created in `/cc`.

See the [cc](../articles/tutorials/cc.md) tutorial for more information.

<a id="TOC_2.2.2."></a>

#### Process control in miniccc

A new `cc process` API has been added to inspect and kill processes started
with `cc background`.

See the [cc](../articles/tutorials/cc.md) tutorial for more information.

<a id="TOC_2.2.3."></a>

#### dnsmasq runtime configuration

A new `dnsmasq configure` API has been added to support adding static IP,
hostname to IP DNS entries, and DHCP options to running dnsmasq instances.

See the [dnsmasq API](../reference/minimega.md) for more information.

<a id="TOC_2.2.4."></a>

#### log filter API

`minimega` can generate quite a bit of logging information, especially when
debug logging is enabled. The `log filter` API now lets you discard log entries
based on a simple string search.

<a id="TOC_2.2.5."></a>

#### minitest improvements

`minitest`, the `minimega` test framework, has been improved to add support for
prologs and epilogs. These files are run before and after all test files,
respectively, and run commands that prepare minimega for tests and clean up
afterwards. Additionally, we have added several new tests to test newer
features.

<a id="TOC_2.2.6."></a>

#### Unified vm info view

`minimega` 2.1 introduced a suffix for displaying VM info by VM type. This has
been replaced with a unified view. KVM or container specific fields are simlpy
left blank when displaying `vm info` of a VM of the other type. This enables
simpler parsing and automation of `vm info` data.

<a id="TOC_2.2.7."></a>

#### vm config cpu API

By default, `minimega` uses the `-host` option when specifying the CPU type for
KVM type VMs. You can now override this with the `vm config cpu` flag.

<a id="TOC_2.2.8."></a>

#### vm config tag API

Users can provide arbitrary key-value pairs to any running VM using the
`vm tag` API. To extend this, users can now assign tags before launching VMs
using the `vm config tag` API. This allows setting tags during VM description
and launching multiple VMs with the same starting tags.

<a id="TOC_2.2.9."></a>

#### New protonuke flags

Along with several bugfixes, protonuke can now be configured to use a user
provided TLS cert instead of the runtime-generated (and invalid) certificate.
Additionally, users can specify the size of the served image in the built-in
HTTP and HTTPS servers.

See the [protonuke](../articles/protonuke.md) article for more
information.

<a id="TOC_2.2.10."></a>

#### rootfs support in vmbetter

`vmetter` can now produce rootfs filesystems suitable for building container
images. See the `vmbetter` help for more information.

<a id="TOC_2.2.11."></a>

#### Scripted multi-file VNC playback

`minimega` supports a new action in stored vnc kb files: `LoadFile`. When a vnc
playback is in process and it encounters this action, playback will continue
with all the actions in the loaded file, returning to the original file after
all actions have been completed. Users may want to use this to discretize the
actions in their playback files (e.g. `unlock screen`, `open browser`, ...).

For example:

```text
0:LoadFile,browse_slashdot.vnc
10000000000:LoadFile,/home/john/recordings/reboot_windows.vnc
```

<a id="TOC_2.2.12."></a>

#### Improved tab completion

In 2.1, we added tab completion for minimega commands. In 2.2, we've made a
small improvement to the tab completion so that the longest common prefix of
the remaining completions is automatically added to the line when you strike
the TAB key.

Leveraging the new `file:` API described above, we are also able to complete
filenames across the mesh. Simply strike the TAB key for a filename prefixed by
`file:` to see the list of possible completions.

<a id="TOC_2.2.13."></a>

#### Version checking in minimega, miniccc

`minimega` and `miniccc` have both been updated to include checks for the
remote client versions during their handshakes. This may affect users in two
ways. If a cluster is running multiple versions of `minimega`, `minimega` will
warn the user. Likewise, `minimega` will warn the user if it connects to
`miniccc` running in a VM whose version does not match `minimega`'s.

These changes will alert the user much sooner about a version mismatch and
prevent subtle bugs in experiments due to version mismatches.

<a id="TOC_3."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing](../articles/installing.md)
guide for more information.

<a id="TOC_3.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.2.deb). It
is known to work in debian 7 (wheezy) and 8 (testing/jessie).

<a id="TOC_3.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
[here](https://storage.googleapis.com/minimega-files/minimega-2.2.tar.bz2).
It should be sufficient to simply unpack the tarball and run tools from the
`bin/` directory directly. Most default paths in minimega, vmbetter, and other
tools are built to be used in this way (i.e. `bin/minimega`, which will then
look for the web directory in `misc/web`).

<a id="TOC_3.3."></a>

### Building from source

Source of the entire distribution is available on
[github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.6+ compiler, libreadline, and libpcap headers.
