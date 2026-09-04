# Module 43 - Physical to Virtual

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-46-physical-to-virtual/).

## Introduction

You can convert physical machines to virtual machines.

## Clonezilla

An open source way would be to boot a Clonezilla LiveCD.

Here are the summarized steps:

Download, burn and boot the LiveCD. Walk through the prompts and backup your computer to external media. Create a virtual machine in minimega with a max harddrive size at least as big as the source. Boot the clonezilla LiveCD in the virtual machine with the external media attached and follow the prompts to restore a backup. Here's a video on YouTube that walks you through the process: [www.youtube.com/watch?v=LS6VhLDw-io](https://www.youtube.com/watch?v=LS6VhLDw-io)

- Note: I find it easiest to save the backup to a FTP or SMB server. You can also save to a USB drive if you have one that's big enough.
- Note: Nothing is done to address driver issues. You may need to add special `qemu-append` flags for things such as `ioapic` or CPU features that your host CPU does not support (but the physical device did).
- Note: This process can also be used to shrink a virtual machine down without writing 0s to the remaining free space.

## VMware

VMware makes a product that can run on a host to create a virtual image. Some driver-related issues are addressed.

[www.vmware.com/products/converter.html](http://www.vmware.com/products/converter.html)

Once created as a `vmdk` file, it can be converted with `qemu-img convert` to be made into a QCOW.

- Note: You may need to add special `qemu-append` flags for things such as `ioapic` or CPU features that your host CPU does not support (but the physical device did).

## Stop 7b

Windows doesn't like when you change driver chipsets. This is especially common when converting a physical Windows machine <= Vista to a virtual instance.

You can read more about this issue here.

[support.microsoft.com/en-us/help/314082](https://support.microsoft.com/en-us/help/314082)

I've found running `Mergeide.reg` before turning off and backing up your HDD will often times fix this issue.
