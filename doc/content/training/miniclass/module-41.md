# Module 41 - QMP Sockets

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-41-qmp-sockets/).

## Introduction

You can interface with QEMU directly. A lot of the functions built into the CLI do so for you, but here is how you can do it yourself.

## VM QMP

minimega provides `vm qmp` to issue JSON-encoded commands directly to a VM's QMP socket.

This is useful if you know of some low level QEMU function you need completed that is not yet implemented in minimega.

A simple command would be to query the status of a VM

```
vm qmp 0 '{ "execute": "query-status" }'
```

## Background

You can read more about QMP sockets on QEMU's website: [wiki.qemu.org/QMP](http://wiki.qemu.org/QMP)

## Example

Let's boot a VM in `snapshot true` mode:

```
vm config memory 1024
vm config disk /home/ubuntu/ubuntu.qcow
vm config snapshot true
vm config vcpus 1
vm launch kvm ubuntu
vm start ubuntu
```

Let's a make a change

```
echo test > test.txt
sync
```

Now let's see if we can save that change back to disk even though we started the VM in `snapshot true`

Print the existing drives

```
vm qmp 0 '{ "execute": "query-block" }'
```

```
ubuntu:
{"return":[
{"device":"ide0-hd0","inserted":
{"backing_file":"/home/ubuntu/tinycore.qcow","backing_file_depth":1,"bps":0,"bps_rd":0,"bps_wr":0,"cache":
{"direct":false,"no-flush":true,"writeback":true},
    "detect_zeroes":"off","drv":"qcow2","encrypted":false,"encryption_key_missing":false,"file":"/var/tmp/vl.zezSm1","image":
{"actual-size":790528,"backing-filename":"/home/ubuntu/ubuntu.qcow","backing-filename-format":"qcow","backing-image":
{"actual-size":3.1531008e+07,"cluster-size":4096,"dirty-flag":false,"filename":"/home/ubuntu/ubuntu.qcow","format":"qcow",
    "virtual-size":1.073741824e+10},"cluster-size":65536,"dirty-flag":false,"filename":"/var/tmp/vl.zezSm1","format":"qcow2","format-specific":
{"data":
{"compat":"1.1","corrupt":false,"lazy-refcounts":false,"refcount-bits":16},"type":"qcow2"},
    "virtual-size":1.073741824e+10},"iops":0,"iops_rd":0,"iops_wr":0,"node-name":"#block866",
    "ro":false,"write_threshold":0},"io-status":"ok","locked":false,"removable":false,"type":"unknown"},
{"device":"ide0-cd1","io-status":"ok","locked":false,"removable":true,"tray_open":false,"type":"unknown"},
{"device":"ide1-cd0","io-status":"ok","locked":false,"removable":true,"tray_open":false,"type":"unknown"},
{"device":"floppy0","locked":false,"removable":true,"tray_open":true,"type":"unknown"},
{"device":"sd0","locked":false,"removable":true,"tray_open":false,"type":"unknown"}]}
```

Out of that whole blob you just need to know that the snapshot image for `/home/ubuntu/ubuntu.qcow` is using the device name `ide0-hd0`

Now let's tell QEMU to write the changes from memory and save them to disk.

```
$ vm qmp 0 '{"execute":"block-commit", "arguments": { "device": "ide0-hd0" } }'
ubuntu: {"return":{}}
```

This may take a while to run; you can check if it is still doing something with

```
$ vm qmp 0 '{"execute":"query-block-jobs"}'
```

When finished you can turn off the VM from the guest and see if the change took

```
vm flush
vm launch kvm ubuntu
vm start
```

After it boots, you should be able to find the file `test.txt`, even though the machine was originally started in `snapshot true`.

Be careful running this command as it goes behind minimega's back and if multiple machines are running in `snapshot true` off the same image you could corrupt images or crash your box.
