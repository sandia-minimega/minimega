# Module 9 - Starting Multiple VMs with KVM

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-9-starting-and-killing-multiple-vms-with-kvm/).

## Introduction

We have a virtual machine running now, let's make some more virtual machines.

## Starting Multiple Virtual Machines

This is where the beauty of minimega really starts to show. Let's issue some commands to launch more VMs.

First let's make sure we have the launch config from the previous module.

```
minimega:/tmp/minimega/minimega$ clear vm config
minimega:/tmp/minimega/minimega$ vm config memory 128
minimega:/tmp/minimega/minimega$ vm config cdrom /home/ubuntu/tinycore.iso
```

Now let's launch 10 VMs

```
vm launch kvm 10
```

And let's check to see if they were built

```
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state    | snapshot | cdrom
ubuntu | myfirstvm | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-1      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-10     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-2      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-3      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-4      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-5      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-6      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-7      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-8      | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-9      | BUILDING | true     | /home/ubuntu/tinycore.iso
```

Issue a `vm start all` to start these VMs

```
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state    | snapshot | cdrom
ubuntu | myfirstvm | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-1      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-10     | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-11     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-12     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-13     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-2      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-3      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-4      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-5      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-6      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-7      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-8      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-9      | RUNNING  | true     | /home/ubuntu/tinycore.iso
```

## Filtering

Now that we have VMs running we can do some filtering. Let's create some VMs in a building state.

```
minimega:/tmp/minimega/minimega$ vm launch kvm 3
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state    | snapshot | cdrom
ubuntu | myfirstvm | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-1      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-10     | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-11     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-12     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-13     | BUILDING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-2      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-3      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-4      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-5      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-6      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-7      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-8      | RUNNING  | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-9      | RUNNING  | true     | /home/ubuntu/tinycore.iso
```

Let's practice a filter command

```
minimega:/tmp/minimega/minimega$ .filter state!=running vm info
host   | id | name  | state    | namespace | memory | vcpus | type | vlan | bridge | tap | mac | ip | ip6 | bandwidth | migrate | disk | snapshot | initrd | kernel | cdrom                     | append | uuid                                 | cc_active | vnc_port | tags | qos
ubuntu | 11 | vm-11 | BUILDING |           | 128    | 1     | kvm  | []   | []     | []  | []  | [] | []  | []        |         |      | true     |        |        | /home/ubuntu/tinycore.iso |        | 483ed51e-a2c9-4350-84c2-9ebecf9b8116 | false     | 34878    | {}   | []
ubuntu | 12 | vm-12 | BUILDING |           | 128    | 1     | kvm  | []   | []     | []  | []  | [] | []  | []        |         |      | true     |        |        | /home/ubuntu/tinycore.iso |        | 809e37c4-d075-4f4e-b6bb-8be3901b2eb7 | false     | 40079    | {}   | []
ubuntu | 13 | vm-13 | BUILDING |           | 128    | 1     | kvm  | []   | []     | []  | []  | [] | []  | []        |         |      | true     |        |        | /home/ubuntu/tinycore.iso |        | d1e6c233-8442-427c-9af4-dbc574a052d5 | false     | 40982    | {}   | []
```

Now let's stack filters and columns

```
minimega:/tmp/minimega/minimega$ .filter state!=RUNNING .columns id,name,state vm info
host   | id | name  | state
ubuntu | 11 | vm-11 | BUILDING
ubuntu | 12 | vm-12 | BUILDING
ubuntu | 13 | vm-13 | BUILDING
```

Let's go one step further and print as a comma-separated value (CSV) list

```
minimega:/tmp/minimega/minimega$ .filter state!=RUNNING .csv true .columns id,name,state vm info
host,id,name,state
ubuntu,11,vm-11,BUILDING
ubuntu,12,vm-12,BUILDING
ubuntu,13,vm-13,BUILDING
```

Let's do some wildcard matching

```
minimega:/tmp/minimega/minimega$ .filter name~vm-1 .columns id,name vm info
host   | id | name
ubuntu | 1  | vm-1
ubuntu | 10 | vm-10
ubuntu | 11 | vm-11
ubuntu | 12 | vm-12
ubuntu | 13 | vm-13
```

That was dead simple, let's start them up

```
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name      | state   | snapshot | cdrom
ubuntu | myfirstvm | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-1      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-10     | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-2      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-3      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-4      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-5      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-6      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-7      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-8      | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | vm-9      | RUNNING | true     | /home/ubuntu/tinycore.iso
```

## Killing VMs

The `vm kill` command can be used to turn off a VM.

The `vm flush` command will delete VMs in a "QUIT" or "ERROR" state.

This is akin to pulling the power out from under the machine. Issuing a shutdown from inside a VM would do so gracefully.

```
minimega:/tmp/minimega/minimega$ vm kill all
minimega:/tmp/minimega/minimega$ vm flush
```

## String Expansion

You can also start VMs with string expansion. First delete all the VMs and make some new ones.

```
minimega:/tmp/minimega/minimega$ vm kill all
minimega:/tmp/minimega/minimega$ vm flush
minimega:/tmp/minimega/minimega$ vm launch kvm hello[1-10]
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name    | state   | snapshot | cdrom
ubuntu | hello1  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello10 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello2  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello3  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello4  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello5  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello6  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello7  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello8  | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | hello9  | RUNNING | true     | /home/ubuntu/tinycore.iso
```

That was neat, now let's use comma-separated values.

```
minimega:/tmp/minimega/minimega$ vm kill all
minimega:/tmp/minimega/minimega$ vm flush
minimega:/tmp/minimega/minimega$ vm launch kvm test1,test2,test3
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | test1 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | RUNNING | true     | /home/ubuntu/tinycore.iso
```

Let's combine the two.

```
minimega:/tmp/minimega/minimega$ vm kill all
minimega:/tmp/minimega/minimega$ vm flush
minimega:/tmp/minimega/minimega$ vm launch kvm test[1-3],a[10-12]
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | RUNNING | true     | /home/ubuntu/tinycore.iso
```

And kill by string expansion as well

```
minimega:/tmp/minimega/minimega$ vm kill a[10-12]
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | RUNNING | true     | /home/ubuntu/tinycore.iso
minimega:/tmp/minimega/minimega$ vm kill test1
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | RUNNING | true     | /home/ubuntu/tinycore.iso
minimega:/tmp/minimega/minimega$ vm kill test2,test3
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | QUIT    | true     | /home/ubuntu/tinycore.iso
```

If you try and start them all again it will fail with a `vm start all`.

```
minimega:/tmp/minimega/minimega$ vm start all
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | QUIT    | true     | /home/ubuntu/tinycore.iso
```

This is by design. You must reference them by name or string expansion.

```
minimega:/tmp/minimega/minimega$ vm start a10
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | QUIT    | true     | /home/ubuntu/tinycore.iso
minimega:/tmp/minimega/minimega$ vm start test[1-3]
minimega:/tmp/minimega/minimega$ .columns name,state,snapshot,cdrom vm info
host   | name  | state   | snapshot | cdrom
ubuntu | a10   | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | a11   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | a12   | QUIT    | true     | /home/ubuntu/tinycore.iso
ubuntu | test1 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test2 | RUNNING | true     | /home/ubuntu/tinycore.iso
ubuntu | test3 | RUNNING | true     | /home/ubuntu/tinycore.iso
```

## Additional Information

Notice that when we use the `vm launch` command, we specify that we want to launch a `kvm` type VM. minimega currently supports the `kvm` type (described below) and the `container` type (described in the [containers module](module-26.md)).

All VM types are configured using the `vm config` API. Many configuration parameters are common to all VM types, such as `memory`, `net`, and `uuid`. Others are specific to one type of VM, such as `disk`, which is specific for KVM type VMs, and `filesystem`, which is specific to `container` type VMs.

When launching a VM, only the configuration parameters used by that VM type are used. For example, it is safe to have `vm config disk` specified when launching a `container` type VM because the `container` type doesn't use that configuration parameter.

### VM Types

#### KVM Virtual Machines

minimega supports booting `kvm` type VMs by using QEMU/KVM. When launching `kvm` type VMs, minimega will use the configuration provided in the `vm config` API to generate command line arguments for QEMU. Additional configuration, such as creating network taps and assigning them to openvswitch bridges occurs at launch time.

minimega manages running `kvm` type VMs, including providing an interface to the VM's QMP socket. When a VM quits or crashes, minimega will reflect this in the `state` column of `vm info`.

#### KVM-Specific Configuration Parameters

Most `vm config` parameters are common to all VM types, though some are specific to `KVM` and `container` instances. See the [minimega API](https://www.sandia.gov/minimega/minimega-api/) for documentation on specific configuration parameters.

Configuration parameters specific to `KVM` instances:

- `vcpus` - Set the number of virtual CPUs to allocate for a VM.
- `append` - Add an append string to a kernel set with vm kernel.
- `qemu` - Set the QEMU process to invoke.
- `qemu-override` - Override parts of the QEMU launch string by supplying a string to match, and a replacement string.
- `qemu-append` - Add additional arguments to be passed to the QEMU instance.
- `migrate` - Assign a migration image, generated by a previously saved VM to boot with.
- `disk` - Attach one or more disks to a vm.
- `cdrom` - Attach a cdrom to a VM.
- `cpu` - Set the virtual CPU architecture.
- `kernel` - Attach a kernel image to a VM.
- `initrd` - Attach an initrd image to a VM.
- `serial` - Specify serial ports.
- `virtio-serial` - Specify virtio-serial ports.

#### Technical Details

minimega uses QEMU version 1.6 or greater. When launching `kvm` type VMs, the following occurs, in order:

- A new VM handler is created within minimega, which is populated with a copy of the VM configuration
- An instance directory for the VM is created (by default /tmp/minimega/N, where N is the VM ID), and the configuration is written to disk
- If this is a new VM, checks are performed to ensure there are no networking or disk conflicts
- Any specified network taps are created and attached to openvswitch
- QEMU arguments are created and patched with any fields specified with `vm config qemu-override`
- QEMU is started
- CPU affinity is applied if enabled
- Handlers are created to watch the state of QEMU and kill QEMU on demand
- A QMP connection is established, and QMP capabilities are enabled
- After a successful QMP connection, the VM is put into the BUILDING state, otherwise QEMU is killed and the VM is put in the ERROR state
- minimega returns control to the user

A number of QEMU arguments are hardcoded, such as using the host CPU architecture (for kvm support), and enabling memory ballooning. In rare circumstances some of these arguments need to by removed or modified. The `vm config qemu-override` API provides a mechanism to patch the QEMU argument string before launching VMs.
