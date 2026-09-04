# Module 0 - Bare Metal vs Nested Virtualization

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-0-bare-metal-vs-nested-virtualization/).

## Introduction

minimega [sic] is software written in GoLang that allows you to interface with KVM and create virtual machine environments very rapidly.

In order to use minimega you must first have a computer.

You can either do so with bare-metal or with nested virtualization.

## Bare Metal

The preferred way to use minimega is to use dedicated hardware to run a series of virtual machines (VM).

If you choose this method, find an extra computer you have laying around and that you don't mind deleting the files off of and continue to the next module.

## Nested Virtualization

If you don't have an extra computer to use and are just trying to learn how to use minimega it possible and convenient to use nested virtual machines.

You can install Ubuntu Server to a VM and run VMs inside of the first VM with minimega.

This has large performance impacts and can slow things down quite a bit.

If you choose this path, use VMWare Player, VMWare Workstation, or VMWare Fusion and create a standard Virtual Machine.

- Set the Memory to 4096mb or higher
- Set the number of processors to 4 or higher.
- Set the "Virtualization Engine" to "Automatic"
- Check the box to enable "Virtualize Intel VT-x/EPT or AMD-V/RVI"
