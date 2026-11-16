# Saving and restoring experiments

minimega can write the memory state and disk contents of a running KVM VM to
files and later boot a new VM from them, picking up exactly where the saved
one left off. `vm save` does this for one VM; `ns save` does it for every VM
in a namespace and records a command file that relaunches them. This page
explains both, what they capture, and how to bring a saved experiment back.

You should be comfortable with [VM lifecycle](vm-lifecycle.md) and with
[Namespaces](namespaces.md) before reading on. Saving is different from
snapshot mode and from the `disk` API, which manage disk images rather than
running state; see [Disk images and the disk API](disk-images.md) for those.

!!! note "Changed in 3.0"
    `vm migrate` and `vm snapshot` were replaced by `vm save`, `ns snapshot`
    by `ns save`, and `vm config migrate` by `vm config state`. The old names
    still work but are deprecated and will be removed.

## What is saved

`vm save` is supported for KVM VMs only. For each VM it produces two files:

- `<name>.hdd`: the contents of the VM's first disk at the moment of the
  save, written by QEMU as a full backup. Additional disks are written with a
  numeric suffix (`<name>.hdd.1`, and so on). VMs that boot from a kernel and
  initrd or from a CD-ROM have no disk to save, and only the state file is
  written.
- `<name>.state`: the VM's memory and device state, written by QEMU's
  migration machinery.

The state file only makes sense together with the disk it was saved with:
resuming it against a different image is undefined. Saving pauses the VM, and
it stays `PAUSED` afterwards until you `vm start` it.

Containers and Android VMs have no runtime save. `vm save` refuses them, and
`ns save` records their configuration only, so they relaunch fresh. Guest
network state, in-flight `cc` commands, captures, host taps, and VLAN aliases
are not part of a save either.

!!! warning
    A save writes the full memory of every VM plus a copy of every disk. For
    an experiment of any size that is a lot of data; check the free space
    under the files directory first.

## Saving one VM

```text
vm save <vm name> [filename]
```

```minimega
minimega$ vm save web1
minimega$ vm save web1 experiments/day1/web1
```

Without a filename the files land in the files directory (`-filepath`) as
`saved/<vm name>.hdd` and `saved/<vm name>.state`. A relative filename is
also placed under the files directory; an absolute one is used as is. Missing
directories are created, and existing files with the same names are
overwritten.

The command blocks while the disk is copied, then starts the memory save and
returns. `vm save` with no arguments reports the memory saves still in
flight:

```minimega
minimega$ vm save
id | name | status | complete (%)
3  | web1 | active | 42.17
```

The `status` column is `active`, `completed`, or `failed`.

## Restoring one VM

Configure a VM the same way as the one you saved (memory, vCPUs, machine type,
serial ports, and network interfaces must match, because QEMU restores the
device state), point `vm config state` at the state file and `vm config disks`
at the saved disk, then launch and start it:

```minimega
minimega$ clear vm config
minimega$ vm config memory 4096
minimega$ vm config vcpus 2
minimega$ vm config networks LAN
minimega$ vm config disks saved/web1.hdd
minimega$ vm config state saved/web1.state
minimega$ vm launch kvm web1
minimega$ vm start web1
```

`vm config state` is a normal template field: it is read from the files
directory unless the path is absolute, and it is only used when the VM is
launched. With snapshot mode on (the default), the new VM writes to an overlay
and the saved `.hdd` stays intact, so the same save can be restored as many
times as you like. Clear the field afterwards (`clear vm config state`) so the
next VM you launch does not try to resume from it.

## Saving a whole namespace

```text
ns save [name]
```

`ns save <name>` pauses every VM in the active namespace with `vm stop all`,
calls `vm save` for each KVM VM, and writes everything to one directory:

```text
<filepath>/saved/<name>/web1.hdd
<filepath>/saved/<name>/web1.state
<filepath>/saved/<name>/web2.hdd
<filepath>/saved/<name>/web2.state
...
<filepath>/saved/<name>/launch.mm
```

An absolute `<name>` is used as the directory instead. The directory must not
already exist. In a cluster each VM is saved by the host it runs on, into that
host's files directory, and `launch.mm` refers to the files with the `file:`
prefix so they can be fetched from wherever they ended up (see
[File management](file.md)).

`launch.mm` selects the namespace, records its queueing setting, and then
for each VM clears the template, writes the VM's configuration, overrides the
disk and state paths, and launches it by name. Abbreviated:

```minimega
namespace "exp"

ns queueing false

clear vm config
vm config memory 4096
vm config networks LAN
vm config state file:saved/exp/web1.state
vm config disks file:saved/exp/web1.hdd
vm launch kvm "web1"

clear vm config
vm config filesystem /images/containerfs
vm launch container "svc1"

vm start all
# the save process saves the VMs in a paused state, so do a stop/start
shell sleep 10
vm stop all
vm start all
```

Only the first disk of a VM is replaced by its saved copy; any further disks
keep their original paths in the script. Containers and Android VMs appear
with their configuration only, and a warning is logged for each.

Like `vm save`, `ns save` returns once the disk copies are done while the
memory saves continue in the background. `ns save` with no arguments
summarizes them across the namespace:

```minimega
minimega$ ns save
completed | total
2         | 3
```

The VMs are left paused. Start them again with `vm start all`, or tear the
experiment down once the saves have completed.

## Restoring a namespace

Replay the generated script:

```minimega
minimega$ read /tmp/minimega/files/saved/exp/launch.mm
```

Because the script starts with `namespace "exp"`, it creates or selects that
namespace. Kill and flush the original VMs first if they still exist, since
VM names must be unique within a namespace. The trailing stop/start pair in
the script resumes the guests after QEMU has finished loading their state.

## Configuration templates are not saves

`vm config save` and `vm config restore` store and recall *templates* held in
memory by the current namespace: which image, how much memory, which networks.
They contain no runtime state and are lost when minimega exits. `vm save` and
`ns save` capture a running VM. Use templates to describe VM classes in a
script, and saves to checkpoint an experiment that has been running.

Similarly, `disk snapshot` and `disk commit` operate on image files whether or
not a VM is running, and `vm config snapshot` decides whether a VM writes to an
overlay or to the image itself. None of them capture memory.

## See also

- [VM lifecycle](vm-lifecycle.md)
- [Namespaces](namespaces.md)
- [Disk images and the disk API](disk-images.md)
- [File management](file.md) for the files directory and the `file:` prefix
- Reference: [`vm save`](../reference/minimega.md#vm-save),
  [`vm config`](../reference/minimega.md#vm-config) (the `state` field),
  [`ns`](../reference/minimega.md#ns)
