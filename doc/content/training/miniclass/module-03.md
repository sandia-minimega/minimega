# Module 3 - Nested Virtualization Revisited

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-3-nested-virtualization-revisited/).

## Introduction

Now that everything needed for minimega to run is installed. If you are going running minimega inside a virtual machine we can do some pre flight checks.

If you are using Bare Metal you can skip this module.

## Prechecks

KVM by default enables nested virtualization if the host operating system and hardware support it.

Ubuntu 16.04 LTS has the kernel module enabled by default and won't require additional configuration.

There are some things we can check for to make sure this is the case.

### Kernel Module

On a virtual machine with an Intel Processor run:

```
# cat /sys/module/kvm_intel/parameters/nested
```

On a virtual machine with an AMD Processor run:

```
# cat /sys/module/kvm_amd/parameters/nested
```

If the command returns the letter "Y" the module is loaded. If it returns the letter "N" the module is not loaded.

#### Debugging

If you got a letter "N" on VMWare make sure you checked the box to "Virtualize Intel VT-x/EPT or AMD-V/RVI"

If you got a letter "N" ensure your OS has the nested virtualization kernel module enabled.

If you still get the letter "N" your hardware does not support nested virtualization and you will need to use bare metal or a different machine.

### virt-host-validate

The program virt-host-validate can be used to verify nested virtualization support. On Ubuntu this program is included in libvirt-bin.

```
apt-get install libvirt-bin
```

When you run virt-host-validate all checks should "PASS". Depending on your hardware you may get a "WARN".

```
# virt-host-validate
QEMU: Checking for hardware virtualization                                 : PASS
QEMU: Checking for device /dev/kvm                                         : PASS
QEMU: Checking for device /dev/vhost-net                                   : PASS
QEMU: Checking for device /dev/net/tun                                     : PASS
```
