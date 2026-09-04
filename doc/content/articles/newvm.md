# Building VMs with vmbetter


<a id="TOC_1."></a>

## Introduction

Before you can use minimega, you must build disk images or RAM disks for your
VMs. The minimega distribution includes `vmbetter`, which can build several
minimal RAM disks (see
[the vmbetter tutorial for more information](tutorials/vmbetter.md)).
These work well for generic experiments but sometimes, you need more than a
minimal install. This article describes the process to build a VM disk from an
install CD.

<a id="TOC_2."></a>

## Install CD

First, you must obtain an install CD as an ISO image. This can be downloaded
from the web or provided by a vendor.

<a id="TOC_3."></a>

## Starting a VM

In order to perform the install, we create a new qcow and launch a VM with it
and the cdrom:

```text
disk create qcow2 ubuntu.qcow2 20G
vm config disk ubuntu.qcow2
vm config cdrom ubuntu.iso
vm config snapshot false
vm launch kvm ubuntu
vm start all
```

Importantly, we set `snapshot=false` so that any changes we make to the disk
persist. Without this flag, all the changes would disappear when we killed the
VM.

<a id="TOC_3.1."></a>

### Network-based install

Some installers make use the Internet to pull more recent packages or software.
See [this article](nat.md) for more information.

<a id="TOC_4."></a>

## Perform the install

The VM should now be running and the installer will be waiting for user input.
Start `miniweb` and click through the installer using the web interface.

<a id="TOC_5."></a>

## Post-install configuration

Once the VM is installed, you may make additional changes to the filesystem in
order to make persistent changes. This could mean installing additional
software or configurations.

<a id="TOC_5.1."></a>

### Adding miniccc to VMs

Typically, we add `miniccc` to our VM images to facilitate experiment control.
First, we must load the binary onto the VM. This can be done in a number of
different ways including over the network (if the VM is launched with an
interface) or via the `hotplug` API.

Once `miniccc` is on the VM, we must configure it to start automatically.

<a id="TOC_5.1.1."></a>

#### init Scripts (Linux)

There are several examples of integrating miniccc into vmbetter-built VMs in
the repo (see `misc/vmbetter_configs/`).

<a id="TOC_5.1.2."></a>

#### systemd Integration (Linux)

To start miniccc automatically with systemd, add the following to
`/etc/systemd/system/miniccc.service`:

```text
[Unit]
Description=miniccc

[Service]
ExecStart=/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /var/log/miniccc.log

[Install]
WantedBy=multi-user.target
```

You may need to adjust the ExecStart command if you copied miniccc elsewhere.
And then enable the service:

```text
systemctl enable miniccc.service
```

This will start miniccc whenever the VM starts. If you are adding to a VM with
snapshot=false, you should now shutdown the VM and save the disk image.

<a id="TOC_5.1.3."></a>

#### Task Scheduler (Windows)

The built-in Task Scheduler on Windows can be configured to run `miniccc.exe`
on startup.

<a id="TOC_6."></a>

## Final steps

<a id="TOC_6.1."></a>

### Clean up

To make the image smaller, you may wish to remove any cached files (e.g. run
`apt clean`).

<a id="TOC_6.2."></a>

### Sysprep (Windows)

Before finalizing a Windows VM, it is a good idea to use Sysprep to generalize
the VM. This removes computer-specific information so that when you launch
multiple VMs from the same image, they each have a different identifier.

<a id="TOC_6.3."></a>

### Shutdown

Finally, shutdown the VM from within the VM using the standard shutdown
mechanism. Wait for the VM to fully shutdown to ensure that the filesystem is
in a consistent state. minimega will update the VM state to `quit` when the VM
has fully powered off. You may now call `vm flush` to remove the VM.

The image should now be ready to use:

```text
vm config disk ubuntu.qcow2
vm launch kvm ubuntu[0-3]
vm start all
```

Note that further changes can be made by launching the VM with
`snapshot=false`.

<a id="TOC_7."></a>

## Optional: multi-stage builds

Occasionally builds are complex enough to require manual intervention between
the initial install and the final packaging. To facilitate this, `vmbetter` is
roughly split into two stages (build and package). You can use the `-1` and
`-2` flags, along with `chroot` to perform post-install activities before
having `vmbetter` build the final image.

To begin, use any arguments as you normally would, and add the `-1` flag. After
the deployment, `vmbetter` will create a `<config>_stage1` directory, named
after the config file you used. For example:

```text
vmbetter -1 miniccc.conf
ls miniccc_stage1
```

From here, you can use `chroot` to enter the filesystem, make changes, and then
finalize the build:

```text
chroot miniccc_stage1
touch foo
exit
```

When your changes are complete - you can have vmbetter finalize the build by
using the `-2` flag (along with the original config file):

```text
vmbetter -2 miniccc_stage1 miniccc.conf
```
