# Namespaces

Namespaces let you configure and launch VMs across a cluster without deciding
which host each one runs on, and they keep experiments separate from each
other so several people can share the same hosts. Every VM, tap, capture,
VLAN alias, and `vm config` template belongs to exactly one namespace, and the
`vm`, `host`, `cc`, and `capture` commands operate on the active namespace.

The design goal was to leave the single-host API alone: a script that runs an
experiment on your laptop runs unchanged on a hundred-node cluster once the
namespace contains those nodes. This page covers the `namespace` and `ns`
commands, queueing and the scheduler, and how the other APIs behave inside a
namespace. Building the cluster itself (the mesh and `deploy`) is covered in
[Cluster setup](cluster.md).

## The default namespace

minimega starts in the `minimega` namespace. It is special in two ways: it is
recreated automatically if you delete it, and on creation it contains only the
local node. To run VMs on other nodes from the default namespace you must add
them with `ns add-hosts`.

The prompt shows the active namespace. You can also start the command line or
`-e` with a namespace preselected: `minimega -attach -namespace foo` and
`minimega -e -namespace foo vm info` prepend `namespace foo` to every command
(see [Running minimega](running.md)).

## The `namespace` command

`namespace <name>` creates the namespace if needed and makes it active:

```minimega
minimega[minimega]$ namespace foo
minimega[foo]$
```

From then on commands apply to resources that belong to `foo`. In a cluster, a
new namespace contains every node in the mesh **except** the local node, which
is treated as the head node; with no mesh peers it contains only the local
node.

`clear namespace` returns to the default namespace without deleting anything:

```minimega
minimega[foo]$ clear namespace
minimega[minimega]$
```

`namespace` alone lists the namespaces:

```minimega
minimega[minimega]$ namespace
namespace | vlans    | active
foo       |          | false
minimega  | 101-4096 | true
```

The `vlans` column is empty until the namespace has its own VLAN range (see
[VLANs](#vlans) below).

To run a single command in another namespace without switching, prefix it
with the namespace name. These two forms are equivalent:

```minimega
minimega[minimega]$ namespace foo .columns name,state vm info
minimega[minimega]$ .columns name,state namespace foo vm info
```

`clear namespace <name>` destroys a namespace: it kills its VMs, stops its
captures, deletes its VLAN aliases and host taps, tears down any `ns bridge` it
created, and does the same on every host in the cluster. `clear namespace all`
destroys every namespace; the default one is recreated empty.

## The `ns` command

`ns` inspects and changes the *active* namespace.

### Hosts

```minimega
minimega[foo]$ ns hosts
ccc[1-5]
minimega[foo]$ ns add-hosts ccc[6-10]
minimega[foo]$ ns del-hosts ccc[1,3,5,7,9]
```

`ns add-hosts` accepts names, ranges, `localhost`, and `all` (every mesh
peer); a host must already be part of the mesh. `ns del-hosts all` empties the
host list.

### Queueing and the scheduler

Each namespace has its own `vm config` template and its own saved
configurations, so users do not clobber each other's settings, and
`vm launch <type> <name> <saved config>` works per namespace as described in
[VM lifecycle](vm-lifecycle.md#saved-configurations).

By default `vm launch` creates VMs immediately: the scheduler picks a host for
each one and launches it there. With queueing enabled, `vm launch` only adds
VMs to a queue, and the scheduler runs when you call `vm launch` with no
arguments (or `ns schedule`, which is the same thing). Queueing lets the
scheduler see the whole experiment before placing anything:

```minimega
minimega[foo]$ ns queueing true
minimega[foo]$ vm launch kvm a
minimega[foo]$ vm launch kvm b
minimega[foo]$ ns queue
... configuration of a and b ...
minimega[foo]$ vm launch
minimega[foo]$ ns schedule status
start               | end                 | state     | launched | failures | total | hosts
02 Jan 06 15:04 MST | 02 Jan 06 15:04 MST | completed | 2        | 0        | 2     | 1
```

`ns queue` shows what is waiting, `ns flush` discards the queue without
launching anything, and `ns schedule status` lists every scheduler run so far.
Queued VMs do not appear in `vm info` until they are launched.

### Load

The scheduler places each VM on the least loaded host. `ns load` shows or
changes how load is measured, from the `host` statistics of every host in the
namespace:

- `cpucommit` (the default): total vCPU commit divided by the number of CPUs
- `memcommit`: total memory commit divided by total memory
- `netcommit`: total number of network interfaces

The commits count every VM on the host, in any namespace, so the scheduler
avoids hosts that are already busy with other experiments. A host that has
reached its `coschedule` limit is always sorted last.

### Placement hints

Three `vm config` fields pin VMs to hosts or to each other:

- `vm config schedule <host>` puts the VM on that host.
- `vm config coschedule <n>` limits how many *other* VMs may share the host:
  `0` means the VM runs alone, `-1` (the default) means no limit.
- `vm config colocate <vm>` puts the VM on the same host as a VM that is
  already launched or queued.

```minimega
minimega$ vm config schedule ccc50
minimega$ vm config coschedule 0
minimega$ vm launch kvm solo
```

launches `solo` on `ccc50` and keeps every other VM off that host, while

```minimega
minimega$ vm launch kvm a
minimega$ vm config colocate a
minimega$ vm config coschedule 1
minimega$ vm launch kvm b
```

puts `b` next to `a` and lets nothing else join them. `schedule` and
`colocate` cannot be set in the same template. With `coschedule 3` on a range
such as `quad[0-3]`, do not expect the four VMs to share a host: the
least-loaded rule spreads them out unless you also `colocate` them.

### Dry runs

`ns schedule dry-run` computes a placement for the queued VMs without
launching them, `ns schedule mv` edits it, `ns schedule dump` prints it, and
`ns schedule` then launches according to it:

```minimega
minimega$ ns queueing true
minimega$ vm launch kvm vm[0-3]
minimega$ ns schedule dry-run
vm  | dst
vm0 | mm0
vm1 | mm1
vm2 | mm2
vm3 | mm3
minimega$ ns schedule mv vm0 mm1
minimega$ ns schedule mv vm[1-2] mm0
minimega$ ns schedule dump
vm  | dst
vm0 | mm1
vm1 | mm0
vm2 | mm0
vm3 | mm3
minimega$ ns schedule
```

Only named VMs can be moved. VMs launched by count (`vm launch kvm 4`) get
their names when they are launched.

### Private bridges

`ns bridge <name> [vxlan,gre]` creates a bridge on every host in the namespace
and connects them with a full mesh of GRE (the default) or VXLAN tunnels. Each
namespace uses its own tunnel key, so the bridges stay isolated:

```minimega
minimega$ namespace foo
minimega$ ns bridge foo
minimega$ vm config networks foo,LAN
minimega$ vm launch kvm 2
```

Bridge names themselves are not namespaced, so prefix them with the namespace
name by convention. `ns del-bridge <name>` destroys the bridge;
`clear namespace <name>` does it for you.

### Running commands on every host

`ns run <command>` runs a command on every host in the namespace, including
the head node when it is a member, and collects the responses. It replaces
`mesh send all` for namespace-scoped work. `read`, `mesh send`, `vm launch`,
and a nested `ns run` cannot be run this way.

```minimega
minimega[foo]$ ns run host
```

### Saving a namespace

`ns save <name>` pauses every VM in the namespace, saves the memory state and
disk of each KVM VM, and writes a `launch.mm` that relaunches the experiment.
Containers and Android VMs are recorded as configuration only, because
`vm save` supports KVM VMs alone. `ns save` with no arguments reports
progress. See [Saving and restoring experiments](save-restore.md) for the file
layout and the restore procedure.

## Other commands inside a namespace

### `vm`

All `vm` commands are namespace aware: they are sent to every host in the
namespace and the responses are collected on the issuing node, so `vm info`,
`vm start`, and the rest see VMs wherever they run. VM names are unique within
a namespace across hosts (the same name can exist in different namespaces),
while IDs are only unique per host, so address VMs by name.

### VLANs

Each namespace resolves VLAN aliases separately: `LAN` in namespace `foo` is a
different VLAN from `LAN` in namespace `bar`. `vlans range <min> <max>` set in
the default namespace applies to every namespace that has not set its own
range. See [Host networking](networking.md).

### `cc`

Every namespace runs its own `cc` server. Responses are written under
`<filepath>/<namespace>/miniccc_responses/`; the default namespace uses
`<filepath>/miniccc_responses/`. See [Command and control](cc.md).

### `host`

With a namespace active, `host` is broadcast to every host in the namespace
and reports one row per host; in the default namespace with no added hosts it
reports only the local node. The VM statistics it reports count VMs in every
namespace.

### `capture`

Capturing traffic for a VM is namespace aware: the capture runs on the host
where the VM lives and the file can be fetched with `file get` when it
completes. Capturing a whole bridge is not advised in a shared cluster, since
the bridge may carry traffic from other experiments. See
[Capture and instrumentation](capture.md).

## See also

- [Cluster setup](cluster.md)
- [VM lifecycle](vm-lifecycle.md)
- [Saving and restoring experiments](save-restore.md)
- [Running minimega](running.md) for the `-namespace` flag
- Reference: [`namespace`](../reference/minimega.md#namespace),
  [`ns`](../reference/minimega.md#ns),
  [`clear namespace`](../reference/minimega.md#clear-namespace),
  [`vm config schedule`](../reference/minimega.md#vm-config-schedule),
  [`vm config coschedule`](../reference/minimega.md#vm-config-coschedule),
  [`vm config colocate`](../reference/minimega.md#vm-config-colocate)
