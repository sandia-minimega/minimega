# minimega 2.9 release notes

## Introduction

The minimega team is pleased to announce the release of minimega 2.9. This release contains new features for copy & paste and tracking miniccc tunnels. There are also additional changes to clean up the repository.

## Major changes and new features

### [minimega] Add support for copy and paste (#1520)

This feature allows copy and pasting into VMs using the textbox provided by noVNC. There are two modes copy and paste operates in.

By default all VMs will support "primitive paste". Any ASCII text put into the noVNC buffer will be directly typed into the VM.

Bidirectional copy and paste can be enabled on a per-vm basis which allows use of ctrl+c and ctrl+v inside the VM. The noVNC clipboard mirrors the VM's clipboard. Bidirectional copy and paste requires 1) QEMU 6.1+ with the `qemu-vdagent` and 2) The spice agent installed in the VM.

Learn more in the recovered
[copy and paste miniclass lesson](../training/miniclass/module-46.md).

### [minitunnel] track tunneled ports for listing and closing (#1509)

`cc tunnel` allows users to tunnel TCP connections to a local port through a VM. This feature adds two additional commands: `cc tunnel list` and `cc tunnel close`. This allows for the ability to track and close tunnels after they are created.

### [igor] Remove igor source (#1523)

igor has been removed from the minimega repository. Earlier this year, igor2 released in its own repository here: <https://github.com/sandia-minimega/igor2>

## Minor changes and new features

### [minimega] fix bridge variable so deleting netflow capture works (#1516)

Fixes an issue that prevented the deletion of netflow captures.

### Bump version to 2.9; add global version file (#1524)

Updates the version to 2.9. Also creates a single file that defines the version within minimega as well as the deb and python packages.

### fix typos found on website (#1517)

Fixes a number of typos in the repository for help articles and API documentation.

### Fix LICENSES files and update copyrights (#1525)

Fixes broken and missing symlinks in the `LICENSES` directory. Also updates the year on the copyright header in source files.

### Bump golang.org/x/net from 0.7.0 to 0.17.0 (#1522)

### Bump golang.org/x/crypto from 0.14.0 to 0.17.0 (#1526)
