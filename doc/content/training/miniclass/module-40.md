# Module 40 - Injecting Files

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-40-injecting-files/).

## Introduction

You can inject files into a HDD image without booting a virtual machine.

## minimega

First let's take a snapshot of a disk:

```
disk snapshot windows7.qc2 window7_miniccc.qc2
```

If the destination name is omitted, a name will be randomly generated and the snapshot will be stored in the `files/` directory. Snapshots are always created in the `files/` directory.

To inject files into an image:

```
disk inject window7_miniccc.qc2 files "miniccc":"Program Files/miniccc"
```

Each argument after the image should be a source and destination pair, separated by a colon ('`:`'). If the file paths contain spaces, use double quotes. Optionally, you may specify a partition (partition 1 will be used by default):

```
disk inject window7_miniccc.qc2:2 files "miniccc":"Program Files/miniccc"
```

You can optionally specify mount arguments to use with `inject`. Multiple options should be quoted. For example:

```
disk inject foo.qcow2 options "-t fat -o offset=100" files foo:bar
```

Disk image paths are always relative to the `files/` directory. Users may also use absolute paths if desired.

### Newer versions of minimega

In order to be namespace-aware, newer versions of minimega require that disk images be in the `files/` directory (`/data/mmfiles` if booted using `startMM.sh`)

## qemu-nbd

Manually you can inject files using QEMU Network Block Device mounts.

```
su -
modprobe nbd max_part=8
qemu-nbd --connect /dev/nbd0 linux.qcow2
mkdir -p /mnt/kvm
sudo mount /dev/nbd0p1 /mnt/kvm
echo "test" > /mnt/kvm/test.txt
sudo umount /mnt/kvm
sudo qemu-nbd --disconnect /dev/nbd0
```

## Special Note

minimega is only able to inject files into images that do NOT use LVM.
