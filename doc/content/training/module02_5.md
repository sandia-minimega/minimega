# Better vmbetter

> How to create VM images using vmbetter

<a id="TOC_1."></a>

## Building a VM image with vmbetter

The most critical file needed to run a VM is the VM image file.

This can be a single file, a pair, or an entire filesystem.

But as many options as there are for launching and running a VM, they can be thought of in clear, categorical ways.

This module will discuss the types of VMs minimega can run, the image files needed to run those VMs and, most importantly, how to create those image files using vmbetter, part of the minimega tool suite.

So let's get started with the types of VMs you can build.

<a id="TOC_2."></a>

## KVM vs Container

There are broadly two types of VMs minimega is capable of launching: KVMs and Containers

The details of how each work are beyond the scope of this tutorial.

However, it is worth pointing out some key differences, and some key trade-offs.

The kind of VM you decide to run in your experiment will be largely dictated by the needs of your experiment.

<a id="TOC_3."></a>

## KVM

The Kernel-based Virtual Machine (KVM) can be described as a self-contained virtual machine which includes both the kernel and filesystem needed to run.

KVM technology works by turning Linux into a hypervisor, allowing it to act as a host to guest Virtual Machines.

Every KVM VM is implemented as a regular Linux process, and therefore has all the componenets needed by the hypervisor to run as a VM with dedicated virtual hardware.

Because KVM VMs have all the components needed to run as a self-contained Linux machine, they tend to require more resources to run.

<a id="TOC_4."></a>

## Containers

With containers, instead of virtualizing the entire machine, including the hardware stacks, just the OS is virtualized.

The container sits on top of the physical host and its OS and shares the host OS kernel.

The host OS takes on the responsibility for running the processes within each container, and provisioning hardware resources for each.

A container only requires a filesystem with the various programs and files needed to run, and just enough of a virtualized OS to run it.

Consequently, containers require far fewer resources to run, and you can therefore run many more containers on a host compared to KVMs.

<a id="TOC_5."></a>

## How minimega containers differ from Docker/LXC

minimega implements its own lightweight container runtime rather than shelling out to Docker or LXC.

minimega builds containers directly using Linux namespaces (mount, PID, network, UTS, IPC) and cgroups, without requiring a container engine daemon (such as dockerd) to be installed or running on the host.

Because minimega manages the namespaces itself, containers are treated as first-class citizens alongside KVM VMs: they share the same `vm config / vm launch` workflow, the same networking (VLANs, taps), the same command and control (miniccc), and the same instrumentation tools (`vm top`, packet capture, etc.).

minimega containers also boot from a plain root filesystem (an `init` program and a directory tree) rather than a layered image format like a Docker image, which keeps the tooling (vmbetter) simple and avoids the overhead of an image registry or union filesystem.

<a id="TOC_6."></a>

## Types of VM image files

There are a number of image files you can use to launch a virtual machine or container.

- Disk

    -  Disk image for launching KVM VMs in minimega
    -  qcow, qcow2, qc2, etc.

- Kernel/Initrd

    -  Used to launch a KVM VM

- ISO

    -  ISO CD image can be used by minimega

- filesystem

    -  Launch containers in minimega

<a id="TOC_7."></a>

## vmbetter script

vmbetter can be used to produce a simplified Debian environment that is configured to the user's choosing.

The configuration files used by vmbetter allow for many choices. vmbetter itself has many runtime options.

To help simplify the former, we have provided an array of configurations files to get you started.

To help simplify the latter, we have provided a script that will run vmbetter with some sensible default options and settings for the provided configs.

<a id="TOC_8."></a>

## vmbetter script

You can find this script in the minimega directory:

```text
<minimega directory>/misc/vmbetter.bash
```

The script itself is straight-forward and we leave its useage as an exercise for the reader.

However, there are some important fundamentals to understanding how vmbetter configuration works.

Note: vmbetter produces a simplified Debian environment, however it is possible to create an Ubuntu VM. See the misc/vmbetter.bash script for examples.

<a id="TOC_9."></a>

## vmbetter configuration

A vm image can be defined with a configuration file. minimega has a number of default files located in

```text
misc/vmbetter_configs
```

These configuration files can (but not always) have an associated overlay directory. More on this later.

The configuration files contain a number of key sections:

- parents

    -  vmbetter configs are recursive, and therefore load the properties and overlay directories of ancestor configurations before loading the child configurations
    -  this allows for narrowly specified VMs enabling you to avoid loading unecessary software

<a id="TOC_10."></a>

## vmbetter configuration

- overlay

    -  allows you to optionally specify an overlay directory (more on this later)

- packages

    -  here you can specify packages to install via apt

- postbuild

    -  this allows you to specify bash-like commands which run during the building of the VM image.
    -  this is useful for installing software that cannot be installed via apt or other one-time configurations that should not be run each time the VM launches.

<a id="TOC_11."></a>

## vmbetter overlay

The overlay directory is useful for placing files you would like to exist on the built VM

The directory structure is the same as that of a standard directory structure.

A file placed in `<targetVM>_overlay/etc/foo/` will be in the VM under `/etc/foo/`.

There can also be an init file in the overlay directory. Here is where you write any bash-type commands that are to be executed whenever the VM is launched.

<a id="TOC_12."></a>

## Build miniccc.kernel and miniccc.initrd

A simple KVM image you can use is already configured in the vmbetter_config directory called miniccc.conf.

This image file comes preloaded with miniccc, the command and control component of minimega.

For more information on command and control, see [module 07 - Command and control](module07.md)

To build this using the vmbetter.bash script, try the following command while in the minimega directory:

```text
minimega$ sudo bash misc/vmbetter.bash miniccc
```

<a id="TOC_13."></a>

## Build minirouterfs

vmbetter also lets you build file systems for containers.

One such container already defined in misc/vmbetter_configs is for minirouterfs

minirouterfs is a filesystem that contains miniccc and minirouter built in.

For more information on miniccc, minimega's command and control tool, see [module 07 - Command and control](module07.md)

For more information on minirouter and minimega's router API, see [module 06 - Experimental Network](module06.md)

To build this using the vmbetter.bash script, try the following command while in the minimega directory:

```text
minimega$ sudo bash misc/vmbetter.bash minirouterfs
```

<a id="TOC_14."></a>

## and more...

There are a number of configurations in the vmbetter_config directory.

Feel free to explore everything in there, and try building your own.

Don't forget to include a parent config in your own config if you want to build from an existing image.

<a id="TOC_15."></a>

## Next Up...

[Module 03: VM Orchestration](module03.md)
