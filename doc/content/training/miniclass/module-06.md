# Module 6 - Navigating the CLI and Output Rendering

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-6-navigating-the-cli-and-output-rendering/).

## Introduction

Now that you have minimega up and running let's get familiar with the CLI.

Let's start minimega and attach

```
minimega -nostdin &
minimega -attach
```

## Disconnecting

When launched interactively one can simply issue the command quit to exit

```
quit
```

When launched as a daemon you can use the command disconnect or control+d

```
disconnect
```

## Help

```
minimega:/tmp/minimega/minimega$ help
ubuntu: Display help on a command. Here is a list of commands:
.alias                   :       create an alias
.annotate                :       enable or disable hostname annotation
.columns                 :       show certain columns from tabular data
.compress                :       enable or disable output compression
.csv                     :       enable or disable CSV mode
.filter                  :       filter tabular data by column value
.headers                 :       enable or disable headers for tabular data
.json                    :       enable or disable JSON mode
.record                  :       enable or disable history recording
.sort                    :       enable or disable sorting of tabular data
.unalias                 :       remove an alias
background               :       execute a command in the background
bridge                   :       display information and modify virtual bridges
capture                  :       capture experiment data
-- press [ENTER] to show more, EOF to discard --
```

Pressing enter will print more information and control+d will stop printing

You can also do help <command>

```
minimega:/tmp/minimega/minimega$ help .record
ubuntu: Usage:
        .record [true,false]
        .record  (command)

Enable or disable the recording of a given command in the command history.
```

From here you may notice you can actually do tab completion type he tab and you will notice it will expand to help

```
he<tab>
```

Tab expansion also works on local filesystem files

```
shell cat /etc/issue
```

Through the remaining modules you'll learn what all these commands do.

For now let's focus on output rendering

## Output Rendering

With minimega you can actually manipulate the way data is printed. For example if you only want to see certain columns or vms you can tell minimega to only print those.

I find it best to learn by example so here are some examples.

```
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .csv true host
host,name,cpus,load,memused,memtotal,bandwidth,vms,vmsall
ubuntu,ubuntu,1,0.00 0.00 0.00,190 MB,2000 MB,0.0/0.0 (rx/tx MB/s),0,0
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .csv true
minimega:/tmp/minimega/minimega$ host
host,name,cpus,load,memused,memtotal,bandwidth,vms,vmsall
ubuntu,ubuntu,1,0.00 0.00 0.00,190 MB,2000 MB,0.0/0.0 (rx/tx MB/s),0,0
minimega:/tmp/minimega/minimega$ .csv false
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .json true host
[{"Host":"ubuntu","Response":"","Header":["name","cpus","load","memused","memtotal","bandwidth","vms","vmsall"],"Tabular":[["ubuntu","1","0.00 0.00 0.00","190 MB","2000 MB","0.0/0.0 (rx/tx MB/s)","0","0"]],"Error":""}]
minimega:/tmp/minimega/minimega$ .columns memtotal,bandwidth host
host   | memtotal | bandwidth
ubuntu | 2000 MB  | 0.0/0.0 (rx/tx MB/s)
```

## Setting and Unsetting Variables

minimega uses variables and they can be set by calling the command and setting the variable. They are unset using the clear command. Here's an example.

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
minimega:/tmp/minimega/minimega$ vm config disk
```

Notice how the disk path is not set.

```
minimega:/tmp/minimega/minimega$ vm config disk
/home/ubuntu/mydisk.img
minimega:/tmp/minimega/minimega$ vm config disk
ubuntu: [/home/ubuntu/mydisk.img]
```

This is setting and checking the `vm config disk` variable.

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
Disk Paths:         [/home/ubuntu/mydisk.img]
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

The disk path is now set in the vm configuration.

```
minimega:/tmp/minimega/minimega$ clear vm config disk
minimega:/tmp/minimega/minimega$ vm config disk
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

The clear command was used to unset the disk path.
