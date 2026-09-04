# Module 15 - Booting a disk image

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-15-booting-a-disk-image/).

## Introduction

The process is relatively simple to boot a disk instead of an ISO.

### Commands

`vm config cdrom` becomes `vm config disk`

```
minimega:/tmp/minimega/minimega$ vm kill all
minimega:/tmp/minimega/minimega$ vm flush
minimega:/tmp/minimega/minimega$ clear vm config
minimega:/tmp/minimega/minimega$ vm config disk /home/ubuntu/wireshark.qcow2
minimega:/tmp/minimega/minimega$ vm config memory 256
minimega:/tmp/minimega/minimega$ vm launch kvm wireshark[1-3]
minimega:/tmp/minimega/minimega$ vm start all
```

### Explore

With the knowledge you have now, it wouldn't be a bad idea to take some time and try booting some of the other qcow images you created.
