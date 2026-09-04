# Module 12 - Building a VM with KVM

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-12-building-a-vm-with-kvm/).

Module 12 - Building a VM with KVM

## Introduction

In this guide we are going to build a VM using KVM

## Disk Creation

KVM typically stores virtual machine hdd images in single qcow2 format file and minimega provides a way to create those files.

Note: A 100G file is not created. This is just the maximum size the VM can grow to.

```
minimega:/tmp/minimega/minimega$ disk create qcow2 /home/ubuntu/test.qcow2 100G
```

You can create disks in more formats using the qemu-img command.

```
root@ubuntu:~/minimega# qemu-img create -f qcow /home/ubuntu/test.img 1G
Formatting '/home/ubuntu/test.img', fmt=qcow2 size=1073741824 encryption=off cluster_size=65536 lazy_refcounts=off refcount_bits=16
```

You can also enable things like compression if you want to sacrifice speed for file size.

```
qemu-img convert -f qcow -O qcow -c /home/ubuntu/test.img /home/ubuntu/test-compressed.qcow
```

## Disk Conversion

Sometimes you will need to convert disk images from one file type to another. Such as qcow->qcow2, vmdk->qcow2, or qcow2->qcow. Thankfully `qemu-img` convert can be used for this.

```
root@ubuntu:~/minimega# qemu-img create -f qcow /home/ubuntu/file.qcow 1G
Formatting '/home/ubuntu/file.qcow', fmt=qcow size=1073741824 encryption=off
root@ubuntu:~/minimega# qemu-img convert -f qcow -O qcow2 /home/ubuntu/file.qcow /home/ubuntu/file.qcow2
```

## Disk Shrinking

There is a side effect to the way virtual image formats work. If you create a large file say 50GB and then delete the large file.

The file on disk will have grown to 50GB+ and will not shrink back down on its own. Shrinking a disk is a very time consuming process.

You need to create a file of all zeros that fills the disk and then delete it.

With a Windows Guest this can be done with `fsutil`

```
fsutil file createnew myfile 100000000000
del myfile
```

With a Linux Guest you can use `dd`

```
dd if=/dev/zero of=/mytempfile
rm -f /mytempfile
```

When that is complete you can convert the disk with `qemu-img convert`

```
mv image.qcow2 image.qcow2_backup
qemu-img convert -O qcow2 image.qcow2_backup image.qcow2
```

## VM Creation

Now we have a disk image, we can use this in minimega to save files. Notice how we turn snapshot to false. When snapshot is false all writes are committed to a disk. When you kill and remove a VM the changes will remain.

Lets use the ISO module 0 to install Ubuntu Server to a disk image.

```
minimega:/tmp/minimega/minimega$ disk create qcow2 /home/ubuntu/ubuntuserver.qcow2 100G
minimega:/tmp/minimega/minimega$ clear vm config
minimega:/tmp/minimega/minimega$ vm config disk /home/ubuntu/ubuntuserver.qcow2
minimega:/tmp/minimega/minimega$ vm config cdrom /home/ubuntu/ubuntuserver.iso
minimega:/tmp/minimega/minimega$ vm config memory 1024
minimega:/tmp/minimega/minimega$ vm config snapshot false
minimega:/tmp/minimega/minimega$ vm launch kvm ubuntuserver
minimega:/tmp/minimega/minimega$ vm start ubuntuserver
```

## Installing

Connect to the VNC session and finish the install following the guide in [Module 1](module-01.md).
