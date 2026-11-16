# Saving and restoring experiments

minimega can write the memory state and disk contents of a running KVM VM to
files. Later, minimega can boot a new VM from those files. The new VM continues
exactly where the saved VM stopped. `vm save` does this for one VM. `ns save`
does it for every VM in a namespace and records a command file that relaunches
the VMs. This page explains both commands, what they capture, and how to bring
a saved experiment back.

Before you read on, we recommend that you know [VM lifecycle](vm-lifecycle.md)
and [Namespaces](namespaces.md). Saving is different from snapshot mode and from
the `disk` API. Snapshot mode and the `disk` API manage disk images rather than
running state. [Disk images and the disk API](disk-images.md) explains them.

!!! note "Changed in 3.0"
    In 3.0, `vm save` replaced `vm migrate` and `vm snapshot`, `ns save`
    replaced `ns snapshot`, and `vm config state` replaced `vm config migrate`.
    The old names still work, but they are deprecated. minimega will remove
    them.

## What is saved

`vm save` supports KVM VMs only. For each VM, it produces two files:

- `<name>.hdd`: the snapshot overlay of the first disk of the VM at the moment
  of the save. QEMU copies only the overlay, so the file still depends on the
  original backing image. QEMU writes additional disks with a numeric suffix
  (for example `<name>.hdd.1`). A VM with `snapshot false` gets no `.hdd` file.
  A VM with no disk, for example one that has only a kernel and an initrd, has
  no disk to save, so QEMU writes only the state file.
- `<name>.state`: the memory and device state of the VM. QEMU writes it with its
  migration machinery.

The state file only makes sense together with the disk that was saved with it.
If you resume the state file against a different image, the result is
undefined. Saving pauses the VM, and the VM stays `PAUSED` afterward until you
`vm start` it.

Containers and Android VMs have no runtime save. `vm save` refuses them, and
`ns save` records their configuration only, so they relaunch without saved
state. A save does not include guest network state, in-flight `cc` commands,
captures, host taps, or VLAN aliases.

!!! warning Before you save, make sure that there is enough free space under the
files directory. A save writes the full memory of every VM plus the snapshot
overlay of every disk. For an experiment of any size, this is a large amount of
data.

## Saving one VM

```text
vm save <vm name> [filename]
```

```minimega
minimega$ vm save web1
minimega$ vm save web1 experiments/day1/web1
```

If you give no filename, minimega puts the files in the files directory
(`-filepath`) as `saved/<vm name>.hdd` and `saved/<vm name>.state`. A relative
filename also goes under the files directory. minimega uses an absolute filename
unchanged. minimega creates missing directories and overwrites existing files
with the same names.

The command blocks while it copies the disk. Then it starts the memory save and
returns. A VM that runs during the save can write to its disk after the disk
copy. The saved disk and the saved memory then do not agree. To prevent this,
run `vm stop <vm name>` before `vm save`. `ns save` stops all VMs first.

`vm save` with no arguments reports the memory saves that are still in
progress:

```minimega
minimega$ vm save
id | name | status | complete (%)
3  | web1 | active | 42.17
```

The `status` column is `active`, `completed`, or `failed`.

## Restoring one VM

Configure a VM the same way as the VM that you saved. Memory, vCPUs, machine
type, serial ports, and network interfaces must match, because QEMU restores
the device state. Point `vm config state` at the state file. Point
`vm config disks` at the saved disk. Then launch and start the VM:

```minimega
minimega$ clear vm config
minimega$ vm config memory 4096
minimega$ vm config vcpus 2
minimega$ vm config net LAN
minimega$ vm config disks saved/web1.hdd
minimega$ vm config state saved/web1.state
minimega$ vm launch kvm web1
minimega$ vm start web1
```

The miniccc agent in the saved memory keeps the UUID of the saved VM, and
minimega accepts an agent only when a VM has its UUID. To use `cc` with the
restored VM, also set `vm config uuid` to the UUID of the saved VM. The
`uuid` column of `vm info` shows it. The script that `ns save` writes sets
the UUID for you.

`vm config state` is a normal template field. minimega reads the file from the
files directory unless the path is absolute. minimega uses the field only at the
launch of the VM. If snapshot mode is on (the default), the new VM writes to an
overlay and the saved `.hdd` stays intact. You can therefore restore the same
save as many times as you want. Afterward, clear the field
(`clear vm config state`), so that the next VM that you launch does not try to
resume from it.

A VM with `snapshot false` has no `.hdd` file, because it writes to its image
directly. To keep that checkpoint, copy the image while the VM is paused after
the save. To restore, point `vm config disks` at the copy. The script that
`ns save` writes still names a `.hdd` file for such a VM, so change that line
to the copy.

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

minimega uses an absolute `<name>` as the directory instead. The directory must
not already exist. In a cluster, each host saves the VMs that run on it into the
files directory of that host. For a relative `<name>`, `launch.mm` refers to the
files with the `file:` prefix. Other hosts can then fetch the files from the
host that holds them (see [File management](file.md)). An absolute `<name>`
writes absolute local paths in `launch.mm`, so each host must hold the files at
those paths.

`launch.mm` selects the namespace and records its queueing setting. Then, for
each VM, it clears the template and writes the configuration of the VM. It also
overrides the disk and state paths and launches the VM by name. This is an
abbreviated example:

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

The script replaces only the first disk of a VM with its saved copy. Any further
disks keep their original paths in the script. The script alone therefore does
not restore a consistent state for a VM with more than one disk. Before you
replay the script, change its `vm config disks` line for that VM. Replace each
further path with its saved copy, for example `web1.hdd.1` for the second
disk and `web1.hdd.1.2` for the third. Containers and Android VMs appear
with their configuration only, and minimega logs a warning for each.

Like `vm save`, `ns save` returns after the disk copies finish. The memory saves
continue in the background. `ns save` with no arguments summarizes them across
the namespace:

```minimega
minimega$ ns save
completed | total
2         | 3
```

The VMs stay paused. To continue, start them again with `vm start all`.
Alternatively, tear the experiment down after the saves complete.

## Restoring a namespace

While the original VMs still exist, run `ns save` with no name. Make sure that
`completed` equals `total` and that `total` is the number of KVM VMs. A failed
save counts only in `total`. After you kill the VMs, `ns save` cannot show
their saves. Then kill and flush the original VMs, because VM names must be
unique within a namespace.

Replay the generated script:

```minimega
minimega$ read /tmp/minimega/files/saved/exp/launch.mm
```

The script starts with `namespace "exp"`, so it creates or selects that
namespace. The trailing stop/start pair in the script resumes the guests after
QEMU finishes loading their state.

## Configuration templates are not saves

`vm config save` and `vm config restore` store and recall *templates* that the
current namespace holds in memory: which image, how much memory, which networks.
They contain no runtime state. When minimega exits, it loses them. `vm save` and
`ns save` capture a running VM. Use templates to describe VM classes in a
script, and use saves to checkpoint an experiment that is running.

`disk snapshot` and `disk commit` operate on image files, whether or not a VM is
running. `vm config snapshot` decides whether a VM writes to an overlay or to
the image itself. None of them capture memory.

## See also

- [VM lifecycle](vm-lifecycle.md)
- [Namespaces](namespaces.md)
- [Disk images and the disk API](disk-images.md)
- [File management](file.md) for the files directory and the `file:` prefix
- Reference: [`vm save`](../reference/minimega.md#vm-save),
  [`vm config`](../reference/minimega.md#vm-config) (the `state` field),
  [`ns`](../reference/minimega.md#ns)
