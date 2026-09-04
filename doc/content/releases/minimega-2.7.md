# minimega 2.7 release notes

<a id="TOC_1."></a>

## Introduction

The minimega team is pleased to announce the release of minimega 2.7. This
release includes several new features and bug fixes as well as updates to
support the latest dependency packages. This release introduces igor web, a web
UI similar to miniweb that allows easy use of the igor command-line tool. This
release also includes the addition of the minibuilder UI to the miniweb
front-end allowing rapid configuration and deployment of virtual machines and a
suite of training modules that cover nearly every tool in the minimega suite,
which can be found on minimega.org/training.

<a id="TOC_2."></a>

## What's New?

<a id="TOC_2.1."></a>

### Major Changes and Milestones

<a id="TOC_2.1.1."></a>

#### minibuilder: A UI for minimega

minibuilder allows the user to quickly and easily design and configure VM
networks using a web interface built into miniweb.

PR [#1429](https://github.com/sandia-minimega/minimega/v2/pull/1429)

<a id="TOC_2.1.2."></a>

#### mini101: Training course for minimega

This PR represents a full course series on using minimega, comprised of 15
modules that cover nearly every aspect of using minimega and its various tools.

Training course modules can be found at
[Mini101 training](../training/module01.md).

PR [#1404](https://github.com/sandia-minimega/minimega/v2/pull/1404).

<a id="TOC_2.1.3."></a>

#### igorweb: A web interface to igor

`igorweb` allows users to create/modify/delete `igor` reservations using a
friendly web interface. It has all of the functionality of `igor` in a shiny
package.

`igorweb` is its own package separate from `igor` and issues commands to `igor`.
So, `igor` works without `igorweb`, but `igorweb` can't work without `igor`.

PR [#1371](https://github.com/sandia-minimega/minimega/v2/pull/1371).

<a id="TOC_2.1.4."></a>

#### Upgraded dependency packages

minimega now runs with `Go` 1.12+. Older versions of `Go` are no longer
supported. Upgrades to

- `jQuery` (3.1.1 -> 3.4.1)
- `dataTables` (1.10.12 -> 1.10.20)
- `bootstrap` (3.3.5 -> 4.3.1)

PR [#1409](https://github.com/sandia-minimega/minimega/v2/pull/1409),
PR [#1382](https://github.com/sandia-minimega/minimega/v2/pull/1382).

<a id="TOC_2.1.5."></a>

#### Docker File for minimega

Dockerfile to build a minimega docker image
docker-compose script to run a minimega docker container using the image
README with details

PR [#1428](https://github.com/sandia-minimega/minimega/v2/pull/1428)

<a id="TOC_2.1.6."></a>

#### vncdrone: Improved VNC automation

Support for phases of execution and better name matching. Allows the user to
specify various workloads and randomize work depending on stage (login stage,
work stage, logout) For example, you can have a drone

- login
- randomly pick vnc files to do work
- log out

PR [#1406](https://github.com/sandia-minimega/minimega/v2/pull/1406).

<a id="TOC_2.2."></a>

### Additional New Features

<a id="TOC_2.2.1."></a>

#### minimega: Specify `flush` targets

This PR Changes 'all' VM flush method to 'FlushAll()'.
- Created new 'Flush()' method that uses the 'Apply' method to flush a specified
  target.
- Updated help strings to reflect enhancement.
- Backwards compatible with old `vm flush` behavior

PR [#1392](https://github.com/sandia-minimega/minimega/v2/issues/1392).

<a id="TOC_2.2.2."></a>

#### minimega: use "actual path" from qemu-img for backing image if present

If you run qemu-img info in the same directory as the image you're passing to it
then the (actual path: ...) item isn't included in the output. However, if you
run qemu-img info from a different directory, or you pass the absolute path of
the image to qemu-img info, then the (actual path: ...) item is present.
minimega always passes the full path of a disk image to qemu-img info.

PR [#1437](https://github.com/sandia-minimega/minimega/v2/issues/1437).

<a id="TOC_2.2.3."></a>

#### minimega: package python bindings

Packaging the minimega python bindings would allow users to easily install them
into their python environments with pip.

PR [#1415](https://github.com/sandia-minimega/minimega/v2/pull/1415).

<a id="TOC_2.2.4."></a>

#### minimega: create minimega group in deb package and use it in init script

This allows users to run minimega commands without needing to be root by
creating a "minimega" group. All users that are part of the "minimega" group
can execute commands.

This is achieved by running the following after starting minimega:

```text
chgrp -R minimega $MM_RUN_PATH
chmod -R g=u $MM_RUN_PATH
chmod g+s $MM_RUN_PATH ${MM_RUN_PATH}/minimega
```

in the systemctl script or by minimega on startup.

PR [#1414](https://github.com/sandia-minimega/minimega/v2/pull/1414).

<a id="TOC_2.2.5."></a>

#### minimega: vm net add

Adding `vm net add` command which allows the user to hotplug a nic to a kvm, and
specify some attributes - bridge, vlan, mac, and driver
- Does not work for containers
- Does not include the remove feature

PR [#1363](https://github.com/sandia-minimega/minimega/v2/pull/1363).

<a id="TOC_2.2.6."></a>

#### igor: Specify VLAN for a reservation

Allows users to specify which VLAN they'd like a reservation to connect to.
`-vlan `can be specified for `sub` and `edit`

PR [#1369](https://github.com/sandia-minimega/minimega/v2/pull/1369).

<a id="TOC_2.2.7."></a>

#### igor: Pause activity

Adds the ability to pause `igor` activity. When igor is paused, attempts to
`sub`, `show`, `del`, or other commands will fail and display a configurable
message. `igorweb` will display the same message.

PR [#1403](https://github.com/sandia-minimega/minimega/v2/pull/1403).

<a id="TOC_2.2.8."></a>

#### igor: Enhanced logging

Logging now include all relevant reservation information

PR [#1395](https://github.com/sandia-minimega/minimega/v2/pull/1395),
PR [#1399](https://github.com/sandia-minimega/minimega/v2/pull/1399).

<a id="TOC_2.2.9."></a>

#### igor: extend clarification

Clarifying logs for extend when duration exceeds limits. Before, the message
indicated that the user request more time than specified in their command
because we were adding it to the time remaining in their reservation. Now, we
specify that the requested time, plus the time remaining, is what exceeds the
limit.

PR [#1432](https://github.com/sandia-minimega/minimega/v2/pull/1432)

<a id="TOC_3."></a>

## Bug Fixes

<a id="TOC_3.1."></a>

### minimega

<a id="TOC_3.1.1."></a>

#### Fixed issue adding virtual network interfaces to newer Ubuntu distribution

containers.

On newer distros, `ip link add` actually respects the `iface` name
given within the container `netns`, whereas it didn't before and just gave the
`vethX` as expected.

PR [#1378](https://github.com/sandia-minimega/minimega/v2/pull/1378).

<a id="TOC_3.1.2."></a>

#### Fixed error checking on '`vm config net`' when only using containers

Throws warning instead of an error when running just with containers (no
installation of `kvm/qemu`)

PR [#1380](https://github.com/sandia-minimega/minimega/v2/pull/1380).

<a id="TOC_3.1.3."></a>

#### Fixed vm name check for rtunnel

Fixes issue with reverse tunnel error by changing when the name check for a VM
occurs.

PR [#1384](https://github.com/sandia-minimega/minimega/v2/issues/1384).

<a id="TOC_3.1.4."></a>

#### QOS Update

Clarification for qos help and fixed an issue with burst computation where
setting rate to kbits would result in a 0 burst value, causing an error in TC.

PR [#1381](https://github.com/sandia-minimega/minimega/v2/pull/1381).

<a id="TOC_3.1.5."></a>

#### minimega.namespace_cli: don't use loopback addresses for GRE tunnels

This prohibits loopback addresses from being used as endpoints in bridges in
cliNamespaceBridge in minimega.namespace_cli.go.

PR [#1419](https://github.com/sandia-minimega/minimega/v2/pull/1419).

<a id="TOC_3.2."></a>

### igor

<a id="TOC_3.2.1."></a>

#### Don't uninstall / clear network settings when deleting future reservation

Fixes an issue where deleting future reservations clobbers network settings for
active reservations.

PR [#1391](https://github.com/sandia-minimega/minimega/v2/pull/1391).

<a id="TOC_3.2.2."></a>

#### Allow uninstall/clear for expired reservations

Fixes an issue where the network is not cleared when a reservation expires.

PR [#1401](https://github.com/sandia-minimega/minimega/v2/pull/1401).

<a id="TOC_3.2.3."></a>

#### Fix for the igor stats test to avoid issues with Daylight Savings Time

The stats test was failing (twice a year, it would seem) due to time changes
affecting test outcome.

PR [#1405](https://github.com/sandia-minimega/minimega/v2/pull/1405).

<a id="TOC_3.3."></a>

### miniweb

<a id="TOC_3.3.1."></a>

#### Forcing the href update before reload as the order of operations is

otherwise not guaranteed.

PR [#1364](https://github.com/sandia-minimega/minimega/v2/pull/1364).

<a id="TOC_3.4."></a>

### misc

<a id="TOC_3.4.1."></a>

#### ldd.go

ubuntu 16.04 gives different output for ldd from ubuntu 18.04 this quick change
makes it so you can build containers on 16.04

PR [#1426](https://github.com/sandia-minimega/minimega/v2/pull/1426).

<a id="TOC_3.4.2."></a>

#### Update so git treats build artifacts of uminirouter as ignored.

Changed perms on build.bash files from 664 to 775 so users don't get a git
modified flag upon changing it themselves.

PR [#1417](https://github.com/sandia-minimega/minimega/v2/pull/1417).

<a id="TOC_4."></a>

## Availability

minimega is available in several ways, both as pre-built distributions and
source. See the [installing guide](../articles/installing.md)
guide for more information.

<a id="TOC_4.1."></a>

### Debian package

minimega is available as an x86-64 debian package, available
[here](https://storage.googleapis.com/minimega-files/minimega-2.7.deb). It is
known to work in debian 9 (stretch) and ubuntu 16.04. It is known not to work on
debian 10 (buster).

<a id="TOC_4.2."></a>

### tarball

A pre-built, x86-64 distribution is available in a single distributable tarball
here. It should be sufficient to simply unpack the tarball and run tools from
the `bin/` directory directly. Most default paths in minimega, vmbetter, and
other tools are built to be used in this way (i.e. `bin/minimega`, which will
then look for the web directory in `misc/web`).

<a id="TOC_4.3."></a>

### Building from source

Source of the entire distribution is available on
[Github](https://github.com/sandia-minimega/minimega/v2). Follow the directions
for cloning or forking minimega on github.com. In order to build minimega, you
will need a Go 1.12+ compiler and libpcap headers.
