# Module 8 - Starting a VM with KVM

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-8-starting-a-vm-with-kvm/).

## Introduction

We have minimega installed, and we have a general idea of how to navigate the console. Let's start some VMs.

Note: These commands are used to only start VMs on a single node. Further modules will explain how to use `mesh send` to do so on multiple nodes.

## Downloading TinyCore

TinyCore is a very small distro of Linux that boots a GUI. The LiveCD is 16MB. You can get a copy from [tinycorelinux.net/downloads.html](http://tinycorelinux.net/downloads.html)

Copy this to your machine using `wget`, `scp`, or however you decide

## Starting minimega

Let's start minimega and attach

```
minimega -nostdin &
minimega -attach
```

## Booting a VM from an ISO

Let's call `vm config` and see what the configuration currently looks like

```
minimega:/tmp/minimega/minimega$ vm config
ubuntu: Current VM configuration:
Memory:   2048
VCPUS:    1
Networks: []
Snapshot: true
UUID:
Tags:     {}

Current KVM configuration:
Migrate Path:
Disk Paths:         []
CDROM Path:
Kernel Path:
Initrd Path:
Kernel Append:
QEMU Path:          /usr/bin/kvm
QEMU Append:        []
SerialPorts:        0
Virtio-SerialPorts: 0

Current container configuration:
Filesystem Path:
Hostname:
Init:            [/init]
Pre-init:
FIFOs:           0
```

By default minimega uses 2048MB of RAM for each VM; let's change that with `vm config memory`

```
vm config memory 128
```

Next we'll set the CDROM drive to use the ISO. Remember you can use tab completion.

```
vm config cdrom /home/ubuntu/tinycore.iso
```

Now let's issue a `vm config` command and verify our changes.

```
minimega:/tmp/minimega/minimega$ vm config
ubuntu: Current VM configuration:
Memory:   128
VCPUS:    1
Networks: []
Snapshot: true
UUID:
Tags:     {}

Current KVM configuration:
Migrate Path:
Disk Paths:         []
CDROM Path:         /home/ubuntu/tinycore.iso
Kernel Path:
Initrd Path:
Kernel Append:
QEMU Path:          /usr/bin/kvm
QEMU Append:        []
SerialPorts:        0
Virtio-SerialPorts: 0

Current container configuration:
Filesystem Path:
Hostname:
Init:            [/init]
Pre-init:
FIFOs:           0
```

Easy enough, let's go ahead and create the VM

```
vm launch kvm myfirstvm
```

Let's verify it was built properly with `vm info`

```
minimega:/tmp/minimega/minimega$ vm info
host   | id | name      | state    | namespace | memory | vcpus | type | vlan | bridge | tap | mac | ip | ip6 | bandwidth | migrate | disk | snapshot | initrd | kernel | cdrom                     | append | uuid                                 | cc_active | vnc_port | tags | qos
ubuntu | 0  | myfirstvm | BUILDING |           | 128    | 1     | kvm  | []   | []     | []  | []  | [] | []  | []        |         |      | true     |        |        | /home/ubuntu/tinycore.iso |        | 4b678706-fd2f-467b-97e3-dc1ffa8ff4d0 | false     | 35096    | {}   | []
```

That prints a lot of stuff, let's just get some of the columns

```
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state    | snapshot | cdrom
ubuntu | myfirstvm | BUILDING | true     | /home/ubuntu/tinycore.iso
```

A VM with state BUILDING means the VM was created. A state of ERROR would mean something failed.

Now let's start the VM with `vm start` and see if it is running with `vm info`

```
minimega:/tmp/minimega/minimega$ vm start myfirstvm
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state   | snapshot | cdrom
ubuntu | myfirstvm | RUNNING | true     | /home/ubuntu/tinycore.iso
```

Congratulations, you have officially launched your first VM

## Troubleshooting

If a machine fails to launch for whatever reason. You can use the `vm flush` command to remove the failed machine.

```
vm flush
```

If a complaint arises about missing kernel modules, you may be missing one. Refer back to the Starting and Stopping minimega module's Bash Script section.

```
minimega:/tmp/minimega/minimega$ vm launch kvm myfirstvm
2016/08/25 14:49:25 ERROR kvm.go:573: vm 0 failed to connect to qmp: dial unix /tmp/minimega/0/qmp: connect: connection refused
2016/08/25 14:49:25 ERROR kvm.go:556: kill qemu: exit status 1 QEMU waiting for connection on: disconnected:unix:/tmp/minimega/0/qmp,server
Could not access KVM kernel module: No such file or directory
failed to initialize KVM: No such file or directory

Error (ubuntu): vm 0 failed to connect to qmp: dial unix /tmp/minimega/0/qmp: connect: connection refused
minimega:/tmp/minimega/minimega$ disconnect
root@ubuntu:~/minimega# sudo modprobe kvm_intel;
modprobe: ERROR: could not insert 'kvm_intel': Operation not supported
root@ubuntu:~/minimega# sudo modprobe kvm;
root@ubuntu:~/minimega# sudo modprobe openvswitch;
root@ubuntu:~/minimega# sudo modprobe 8021q;
root@ubuntu:~/minimega# sudo modprobe nbd max_part=8;
```

In this example minimega is unable to load `kvm_intel`; this was because we were running minimega in a Virtual Machine without virtualization nesting enabled.
