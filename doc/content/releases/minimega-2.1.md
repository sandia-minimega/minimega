# minimega 2.1 release notes

<a id="TOC_1."></a>

## Introduction

The minimega development team is pleased to announce the release of minimega
2.1. This release includes several bugfixes and tweaks to the 2.0 release, as
well as substantive internal changes to pave the way for future development.
Several new usability features have been added which should make the
programmability and visibility of minimega environments much better.

<a id="TOC_2."></a>

## What's new

<a id="TOC_2.1."></a>

### Major changes and milestones

<a id="TOC_2.1.1."></a>

#### New web interface

The minimega web interface has been updated again; it now features a
live diagram of the network layout and a more modern look. The new web
interface also fixes several performance issues with the previous interface.
Most notably the web interface now only loads VM screenshots for VMs that are
currently displayed on the screen instead of attempting to render all VM
screenshots on every pageload.

<a id="TOC_2.1.2."></a>

#### vm launch kvm and internal changes

In preparation for Linux container support, we have changed the way the
`vm launch` command works. It now expects the type of the VM to be launched;
currently, only KVM is supported. Where you would have previously run
`vm launch 10` to launch 10 VMs, you should now use `vm launch kvm 10` to
accomplish the same task.

<a id="TOC_2.1.3."></a>

#### Tab completion

Tab completion now works for minimega commands, not just filenames. As
in bash, striking the TAB key after typing a sufficiently unique prefix
will complete the command; if there are multiple possible completions,
striking the TAB key again will show the possibilities.

<a id="TOC_2.1.4."></a>

#### Runtime test framework

To help squash bugs earlier, minimega has a new runtime test framework that
exercises some basic single-host functionality. There is a new tool, minitest,
that runs tests from the `tests/` directory against a running minimega instance
and checks that the output (*.got) matches the expected output (*.want). This
tool is still in its infancy -- there are only a handful of tests and the
framework has limited ability to clean up after itself when things go wrong
(requiring the user to manually cleanup VMs or restart minimega). However, it
has proven useful to find bugs introduced during the changes to the internals.
We hope to improve upon this initial work in future releases to provide more
complete runtime testing for minimega developers.

<a id="TOC_2.2."></a>

### Additional new features

<a id="TOC_2.2.1."></a>

#### vm config serial API

minimega now supports specifying the number of ISA serial and virtio serial
ports. Previous versions of minimega hardcoded one of these ports (1.0 use ISA
serial, 2.0 used virtio serial). The default is to have no serial ports
whatsoever. The `vm config serial` command lets you specify how many ports to
create.

See the [vm config serial API](../reference/minimega.md) for more
information.

<a id="TOC_2.2.2."></a>

#### bridge tunnel API

The `bridge` API now supports VXLAN and GRE tunneling. This API must be
instrumented from both ends of two minimega instances.

See the [bridge API](../reference/minimega.md) for more information.

<a id="TOC_2.2.3."></a>

#### vm launch/start/stop/kill/tag range support

All of `vm launch`, `vm start`, `vm stop`, `vm kill`, and `vm tag` now support
range operations, such as:

```text
vm launch kvm foo[1-20]
```

Which will create foo1, foo2, ..., foo20 VMs.

<a id="TOC_2.2.4."></a>

#### rfbplay ffmpeg transcoding

The `rfbplay` tool now supports invoking ffmpeg for VNC framebuffer recordings
directly in addition to the built-in MJPEG webserver.

See the [VNC record/replay](../articles/vnc.md#TOC_3.1.2.) article for
more information.

<a id="TOC_2.2.5."></a>

#### bridge trunk multiple trunks

The `bridge trunk` API now supports adding more than one trunk port per bridge.

<a id="TOC_2.2.6."></a>

#### .record API

The `.record` API allows toggling if a command is recorded in the command
history or not.

See the [.record](../reference/minimega.md) API for more information.

<a id="TOC_2.2.7."></a>

#### MAC address generation

Automatically generated MAC addresses are now valid according to
[standards.ieee.org/develop/regauth/oui/oui.txt](http://standards.ieee.org/develop/regauth/oui/oui.txt). You may still
override the automatically generated MAC address with any you like using the
`vm config net` API.

<a id="TOC_3."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing](../articles/installing.md)
guide for more information.

<a id="TOC_3.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.1.deb). It
is known to work in debian 7 (wheezy) and 8 (testing/jessie).

<a id="TOC_3.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
[here](https://storage.googleapis.com/minimega-files/minimega-2.1.tar.bz2).
It should be sufficient to simply unpack the tarball and run tools from the
`bin/` directory directly. Most default paths in minimega, vmbetter, and other
tools are built to be used in this way (i.e. `bin/minimega`, which will then
look for the web directory in `misc/web`).

<a id="TOC_3.3."></a>

### Building from source

Source of the entire distribution is available on
[github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.3+ compiler, libreadline, and libpcap headers.
