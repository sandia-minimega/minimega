# Module 14 - Building a VM with vmbetter

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-14-building-a-vm-with-vmbetter-2/).

## Use Case #1: LiveCD with Wireshark

#### Introduction

vmbetter is a versatile tool for making customized Debian-based environments. It can generate kernel+initramfs pairs, QCOW hard disk images, and ISO CDROM images. This document focuses on the latter, specifically on how you can use vmbetter to easily make a customized LiveCD.

In this article, we'll create a LiveCD which boots you straight into a GUI with Wireshark running-it's just an example, but one that could be useful for someone trying to make a ready-to-deploy security toolkit ala Kali Linux.

#### Set up vmbetter

To get vmbetter, you'll want to download and compile the minimega repository. First, make sure you have a [Go compiler](http://golang.org) installed. Then, install some libraries needed to build minimega and the associated tools:

```
# fetch pre-requisites
$ sudo apt-get install libpcap-dev syslinux
# On ubuntu 16.04lts you need these as well
$ sudo apt-get install debootstrap genisoimage extlinux
```

With those installed, you can pull down the minimega repo and build:

```
# get the repository
$ git clone git@github.com:sandia-minimega/minimega.git
$ cd minimega
$ ./all.bash
```

There should now be a binary called `vmbetter` in the `bin` subdirectory.

#### Making a vmbetter config

vmbetter builds images based on rules specified in a configuration file. It also inserts any additional files you may want into the filesystem of the image it creates.

#### Write the config file

We'll start by creating our configuration file. From the minimega repository root, open a new file called `misc/vmbetter_configs/wireshark.conf`. This directory contains several other configurations which could be instructive, but for now just paste the following text into the file and save it:

```
parents = "default.conf"
packages = "traceroute dnsutils wget tcpdump telnet wireshark xserver-xorg-core xserver-xorg-input-all xserver-xorg-video-fbdev xserver-xorg-video-cirrus xterm xinit blackbox"
overlay = "misc/vmbetter_configs/wireshark_overlay"
postbuild = `
    sed -i 's/nullok_secure/nullok/' /etc/pam.d/common-auth
    sed -i 's/PermitRootLogin without-password/PermitRootLogin yes/' /etc/ssh/sshd_config
    sed -i 's/PermitEmptyPasswords no/PermitEmptyPasswords yes/' /etc/ssh/sshd_config
    passwd -d root
`
```

The configuration file specifies several important options: one or more parent configs, a list of packages to install, the "overlay" directory, and the "postbuild" commands. The overlay directory contains any files you want to add to the disc's filesystem. The postbuild commands are a list of shell commands to be run as the final step of the building process; we use it here to set up passwordless root login for convenience.

Note that `wireshark.conf` has a parent configuration, `default_amd64.conf`. That file contains some very basic packages which you'll want in almost every case, so most configurations will want to either include `default_amd64.conf` as a parent, or set their parent to another configuration which uses `default_amd64.conf` as **its** parent.

You'll see that we install Wireshark, the basic X.org packages, and some bare minimum X utilities: xterm, xinit, blackbox. If you want additional programs, you can add their package names to the list. vmbetter will automatically figure out any dependencies they may have and install them for you.

#### Populate the overlay filesystem

Our configuration file says we're going to put any additional files we want into `misc/vmbetter_configs/wireshark_overlay`. This directory contains a hierarchy of files and directories, based at /, that we want copied into the image we build. Let's start by copying over an existing overlay to get a baseline:

```
$ cp -r misc/vmbetter_configs/default_overlay misc/vmbetter_configs/wireshark_overlay
```

If you look in that directory, you'll see a few basic files in a hierarchy:

```
$ du -a
 misc/vmbetter_configs/wireshark_overlay/
4       misc/vmbetter_configs/wireshark_overlay/init
8       misc/vmbetter_configs/wireshark_overlay/
```

In order to make the LiveCD automatically launch X and Wireshark as soon as we log in as root, we can set up a few configuration files:

```
$ cd misc/vmbetter_configs/wireshark_overlay
$ mkdir -p root
$ cat > root/.profile << EOF
#!/bin/sh
xinit -- /usr/bin/Xorg &
EOF
$ cat > root/.xinitrc << EOF
#!/bin/sh
wireshark &
exec blackbox
EOF
$ cd ../../..    # go back to the root of the minimega repo
# On ubuntu init may not have proper permissions
$ chmod 777 misc/vmbetter_conf/wireshark_overlay/init
```

Our `.profile` launches X automatically as soon as root logs in, and then `.xinitrc` starts Wireshark and the Blackbox window manager once X starts.

#### Build the image

Now that we've specified **how** to build the image, we can fire up vmbetter and actually **build** it. Here's the command I used to build:

```
$ sudo ./bin/vmbetter -branch stable -iso -level debug  -mbr /usr/lib/syslinux/mbr/mbr.bin misc/vmbetter_configs/wireshark.conf
```

It's rather complex, so I'll break down each part:

- `-branch stable` : specifies that vmbetter should use the Debian stable branch to build
- `-iso` : specifies that we want an ISO image
- `-level debug` : specifies that we want lots of debug output
- `-mbr /usr/lib/syslinux/mbr/mbr.bin` : On Debian Unstable, the `syslinux` package puts the `mbr.bin` file in a different place than expected by vmbetter
- `misc/vmbetter_configs/wireshark.conf` : the path to the configuration file

(Depending on your system, you may not need to specify the `-mbr` flag, or you may find that `mbr.bin` is in yet a different location.)

When you run vmbetter, it will print lots of debugging information as it calculates dependencies, fetches packages (this will take a long time), installs the packages in a temporary environment, and finally creates an ISO image. If all goes well, you should end up with a file called `wireshark.iso` in the current directory

#### Test the image

If everything worked right, you should have a file called `wireshark.iso` in the current directory. We can test it using minimega:

```
$ sudo ./bin/minimega
minimega$ vm config cdrom wireshark.iso
minimega$ vm launch kvm wireshark
minimega$ vm start wireshark
```

Start [miniweb](../../articles/miniweb.md) and then point your web browser to [http://localhost:9001](http://localhost:9001/), you should be able to see minimega's web front-end. There should be one VM listed on the main page; click the screenshot to open a VNC session to it.

Once you're in the VNC session, you will probably see a login prompt. Log in as `root`, no password necessary. As soon as you log in, it should start up X, run the window manager, and start up Wireshark.

If you want to run the image on real computers, simply burn `wireshark.iso` to a CD and use it to boot.

## Use Case #2: VM from Install CD

### Introduction

Before you can use minimega, you must build disk images or RAM disks for your VMs. The minimega distribution includes `vmbetter`, which can build several minimal RAM disks (see [the vmbetter tutorial for more information](../../articles/tutorials/vmbetter.md)). These work well for generic experiments but sometimes, you need more than a minimal install. This article describes the process to build a VM disk from an install CD.

### Install CD

First, you must obtain an install CD as an ISO image. This can be downloaded from the web or provided by a vendor.

### Starting a VM

In order to perform the install, we create a new qcow and launch a VM with it and the cdrom:

```
disk create qcow2 ubuntu.qcow2 20G\
vm config disk ubuntu.qcow2\
vm config cdrom ubuntu.iso\
vm config snapshot false\
vm launch kvm ubuntu\
vm start all
```

Importantly, we set `snapshot=false` so that any changes we make to the disk persist. Without this flag, all the changes would disappear when we killed the VM.

### Network-based install

Some installers make use the Internet to pull more recent packages or software. See [this article](../../articles/nat.md) for more information.

### Perform the install

The VM should now be running and the installer will be waiting for user input. Start `miniweb` and click through the installer using the web interface.

### Post-install configuration

Once the VM is installed, you may make additional changes to the filesystem in order to make persistent changes. This could mean installing additional software or configurations.

#### Adding miniccc to VMs

Typically, we add `miniccc` to our VM images to facilitate experiment control. First, we must load the binary onto the VM. This can be done in a number of different ways including over the network (if the VM is launched with an interface) or via the `hotplug` API.

Once `miniccc` is on the VM, we must configure it to start automatically.

#### init Scripts (Linux)

There are several examples of integrating miniccc into vmbetter-built VMs in the repo (see `misc/vmbetter_configs/`).

#### systemd Integration (Linux)

To start miniccc automatically with systemd, add the following to `/etc/systemd/system/miniccc.service`:

```
[Unit]
Description=miniccc

[Service]
ExecStart=/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /var/log/miniccc.log

[Install]
WantedBy=multi-user.target
```

You may need to adjust the ExecStart command if you copied miniccc elsewhere. And then enable the service:

```
systemctl enable miniccc.service
```

This will start miniccc whenever the VM starts. If you are adding to a VM with snapshot=false, you should now shutdown the VM and save the disk image.

#### Task Scheduler (Windows)

The built-in Task Scheduler on Windows can be configured to run `miniccc.exe` on startup.

### Final steps

#### Clean up

To make the image smaller, you may wish to remove any cached files (e.g. run `apt clean`).

#### Sysprep (Windows)

Before finalizing a Windows VM, it is a good idea to use Sysprep to generalize the VM. This removes computer-specific information so that when you launch multiple VMs from the same image, they each have a different identifier.

#### Shutdown

Finally, shutdown the VM from within the VM using the standard shutdown mechanism. Wait for the VM to fully shutdown to ensure that the filesystem is in a consistent state. minimega will update the VM state to `quit` when the VM has fully powered off. You may now call `vm flush` to remove the VM.

The image should now be ready to use:

```
vm config disk ubuntu.qcow2\
vm launch kvm ubuntu[0-3]\
vm start all
```

Note that further changes can be made by launching the VM with `snapshot=false`.

### Optional: multi-stage builds

Occasionally builds are complex enough to require manual intervention between the initial install and the final packaging. To facilitate this, `vmbetter` is roughly split into two stages (build and package). You can use the `-1` and `-2` flags, along with `chroot` to perform post-install activities before having `vmbetter` build the final image.

To begin, use any arguments as you normally would, and add the `-1` flag. After the deployment, `vmbetter` will create a `<config>_stage1` directory, named after the config file you used. For example:

```
vmbetter -1 miniccc.conf\
ls miniccc_stage1
```

From here, you can use `chroot` to enter the filesystem, make changes, and then finalize the build:

```
chroot miniccc_stage1\
touch foo\
exit
```

When your changes are complete - you can have vmbetter finalize the build by using the `-2` flag (along with the original config file):

```
vmbetter -2 miniccc_stage1 miniccc.conf
```

### Authors

The minimega authors

Last updated: 11 April 2022

Published: 30 May 2017
