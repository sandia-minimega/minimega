# Module 16 - CDROMs

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-16-cdroms/).

## Introduction

With a running VM you can change the ISO using `vm cdrom`

### Creating an ISO from a folder

```
apt-get install mkisofs
cd /home/ubuntu/
mkdir -p isofolder
echo 'a'> isofolder/test
mkisofs -o new.iso isofolder/
```

### Example

```
vm kill all
vm flush
clear vm config
vm config disk /home/ubuntu/wireshark.qcow2
vm config memory 256
vm launch kvm wireshark
vm start wireshark
```

Insert your ISO

```
vm cdrom change wireshark /home/ubuntu/new.iso
```

Eject your ISO

```
vm cdrom eject wireshark
```
