# Namespaces

Namespaces let you configure and launch VMs across a cluster without deciding
which host runs each VM. Namespaces also keep experiments separate, so several
people can share the same hosts. Every VM, tap, capture, VLAN alias, and
`vm config` template belongs to exactly one namespace. The `vm`, `host`, `cc`,
and `capture` commands operate on the active namespace.

The design goal was to leave the single-host API alone. After the namespace
contains those nodes, a script that runs an experiment on your laptop runs
unchanged on a hundred-node cluster. This page covers the `namespace` and `ns`
commands, queueing and the scheduler, and how the other APIs behave inside a
namespace. [Cluster setup](cluster.md) covers how to build the cluster itself
(the mesh and `deploy`).

## The default namespace

minimega starts in the `minimega` namespace. It is special in two ways. If you
delete it, minimega recreates it automatically. At creation, it contains only
the local node. To run VMs on other nodes from the default namespace, you must
add the nodes with `ns add-hosts`.

The prompt shows the active namespace. You can also start the command line or
`-e` with a namespace preselected. `minimega -attach -namespace foo` and
`minimega -e -namespace foo vm info` prepend `namespace foo` to every command
(see [Running minimega](running.md)).

## The `namespace` command

If the namespace does not exist, `namespace <name>` creates it. The command also
makes the namespace active:

```minimega
minimega[minimega]$ namespace foo
minimega[foo]$
```

From then on, commands apply to resources that belong to `foo`. In a cluster, a
new namespace contains every node in the mesh except the local node. minimega
treats the local node as the head node. If the mesh has no peers, the namespace
contains only the local node.

`clear namespace` returns to the default namespace without deleting anything:

```minimega
minimega[foo]$ clear namespace
minimega[minimega]$
```

`namespace` alone lists the namespaces:

```minimega
minimega[minimega]$ namespace
host | namespace | vlans    | active
mm0  | foo       |          | false
mm0  | minimega  | 101-4096 | true
```

The `vlans` column is empty until the namespace has its own VLAN range (see
[VLANs](#vlans) below).

To run a single command in another namespace without switching, prefix it
with the namespace name. These two forms are equivalent:

```minimega
minimega[minimega]$ namespace foo .columns name,state vm info
minimega[minimega]$ .columns name,state namespace foo vm info
```

`clear namespace <name>` destroys a namespace. The command kills its VMs, stops
its captures, and deletes its VLAN aliases and host taps. It tears down any
`ns bridge` that the namespace created. It does the same on every host in the
cluster. `clear namespace all` destroys every namespace. minimega recreates the
default namespace empty.

## The `ns` command

`ns` inspects and changes the active namespace.

### Hosts

```minimega
minimega[foo]$ ns hosts
ccc[1-5]
minimega[foo]$ ns add-hosts ccc[6-10]
minimega[foo]$ ns del-hosts ccc[1,3,5,7,9]
```

`ns add-hosts` accepts names, ranges, `localhost`, and `all` (every mesh
peer). A host must already be part of the mesh. `ns del-hosts all` empties the
host list.

### Queueing and the scheduler

Each namespace has its own `vm config` template and its own saved
configurations, so users do not overwrite the configuration of other users.
`vm launch <type> <name> <saved config>` works for each namespace, as described
in [VM lifecycle](vm-lifecycle.md#saved-configurations).

By default, `vm launch` creates VMs immediately. The scheduler picks a host for
each VM and launches it there. If queueing is enabled, `vm launch` only adds VMs
to a queue. To run the scheduler, call `vm launch` with no arguments. `ns
schedule` does the same thing. Queueing lets the scheduler see the whole
experiment before placing anything:

```minimega
minimega[foo]$ ns queueing true
minimega[foo]$ vm launch kvm a
minimega[foo]$ vm launch kvm b
minimega[foo]$ ns queue
... configuration of a and b ...
minimega[foo]$ vm launch
minimega[foo]$ ns schedule status
host | start               | end                 | state     | launched | failures | total | hosts
mm0  | 02 Jan 06 15:04 MST | 02 Jan 06 15:04 MST | completed | 2        | 0        | 2     | 1
```

`ns queue` shows what is waiting. `ns flush` discards the queue without
launching anything. `ns schedule status` lists every scheduler run so far.
Queued VMs do not appear in `vm info` until minimega launches them.

### Load

The scheduler places each VM on the least loaded host. `ns load` shows or
changes how the scheduler measures load, from the `host` statistics of every
host in the namespace:

- `cpucommit` (the default): total vCPU commit divided by the number of CPUs
- `memcommit`: total memory commit divided by total memory
- `netcommit`: total number of network interfaces

The commits count every VM on the host, in any namespace. Because of this, the
scheduler avoids hosts that are already busy with other experiments. The
scheduler always sorts a host that reached its `coschedule` limit last.

### Placement hints

Three `vm config` fields pin VMs to hosts or to each other:

- `vm config schedule <host>` puts the VM on that host.
- `vm config coschedule <n>` limits how many other VMs can share the host.
  `0` means the VM runs alone. `-1` (the default) means no limit.
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

puts `b` next to `a` and lets nothing else join them. You cannot set `schedule`
and `colocate` in the same template. Suppose that you use `coschedule 3` on a
range such as `quad[0-3]`. The four VMs do not share a host. The least-loaded
rule spreads them out, unless you also `colocate` them.

### Dry runs

`ns schedule dry-run` computes a placement for the queued VMs without
launching them. `ns schedule mv` edits the placement. `ns schedule dump` prints
it. `ns schedule` then launches the VMs according to it:

```minimega
minimega$ ns queueing true
minimega$ vm launch kvm vm[0-3]
minimega$ ns schedule dry-run
host | vm  | dst
mm0  | vm0 | mm0
mm0  | vm1 | mm1
mm0  | vm2 | mm2
mm0  | vm3 | mm3
minimega$ ns schedule mv vm0 mm1
minimega$ ns schedule mv vm[1-2] mm0
minimega$ ns schedule dump
host | vm  | dst
mm0  | vm0 | mm1
mm0  | vm1 | mm0
mm0  | vm2 | mm0
mm0  | vm3 | mm3
minimega$ ns schedule
```

You can move only named VMs. VMs that you launch by count (`vm launch kvm 4`)
get their names at launch.

### Private bridges

`ns bridge <name> [vxlan,gre]` creates a bridge on every host in the namespace
and connects them with a full mesh of GRE (the default) or VXLAN tunnels. Each
namespace uses its own tunnel key, so the bridges stay isolated:

```minimega
minimega$ namespace foo
minimega$ ns bridge foo
minimega$ vm config net foo,LAN
minimega$ vm launch kvm 2
```

Bridge names themselves are not namespaced. We recommend that you prefix them
with the namespace name. `ns del-bridge <name>` destroys the bridge.
`clear namespace <name>` does it for you.

### Running commands on every host

`ns run <command>` runs a command on every host in the namespace and collects
the responses. If the head node is a member, `ns run` includes it. `ns run`
replaces `mesh send all` for namespace-scoped work. You cannot run `read`,
`mesh send`, `vm launch`, or a nested `ns run` this way.

```minimega
minimega[foo]$ ns run host
```

### Saving a namespace

`ns save <name>` pauses every VM in the namespace. It saves the memory state and
disk of each KVM VM. It also writes a `launch.mm` that relaunches the
experiment. `ns save` records containers and Android VMs as configuration only,
because `vm save` supports KVM VMs alone. `ns save` with no arguments reports
progress. See [Saving and restoring experiments](save-restore.md) for the file
layout and the restore procedure.

## Other commands inside a namespace

### `vm`

All `vm` commands are namespace aware. minimega sends them to every host in the
namespace and collects the responses on the issuing node. Because of this,
`vm info`, `vm start`, and the rest see VMs wherever they run. VM names are
unique within a namespace across hosts. The same name can exist in different
namespaces. IDs are unique only for each host, so address VMs by name.

### VLANs

Each namespace resolves VLAN aliases separately. For example, `LAN` in
namespace `foo` is a different VLAN from `LAN` in namespace `bar`. If you set
`vlans range <min> <max>` in the default namespace, it applies to every
namespace that does not have its own range. See
[Host networking](networking.md).

### `cc`

Every namespace runs its own `cc` server. The server writes responses under
`<filepath>/<namespace>/miniccc_responses/`. The default namespace uses
`<filepath>/miniccc_responses/`. See [Command and control](cc.md).

### `host`

If a namespace is active, minimega broadcasts `host` to every host in the
namespace, and `host` reports one row for each host. In the default namespace
with no added hosts, `host` reports only the local node. The VM statistics that
it reports count VMs in every namespace.

### `capture`

Capturing traffic for a VM is namespace aware. The capture runs on the host
where the VM lives. When the capture completes, you can fetch the file with
`file get`. We recommend that you do not capture a whole bridge in a shared
cluster, because the bridge can carry traffic from other experiments. See
[Capture and instrumentation](capture.md).

VM and bridge captures accept options after the file name, such as
`snaplen=128` or `filter="tcp port 80"`. These options apply to one capture
only, so you do not have to change the namespace settings around a capture.

To capture all the traffic of a namespace on all its hosts into one file, use
`capture pcap namespace <filename>`. Each host mirrors the edge ports of the
namespace, that is, its VM, container and host taps, and sends the frames to
the node that runs the command. That node merges the frames in timestamp order.
Each frame is in the file once, also when it crosses a GRE or VXLAN tunnel.
To order the frames, that node adds the measured clock offset of each host to
the host timestamps. The file keeps the host timestamps. Frames that a host
sends before the first acknowledgment from that node have no offset
correction. Thus, keep the clocks of the hosts synchronized with chrony or
PTP. Every mesh node must run a minimega that supports namespace captures. See
`help capture pcap namespace`.

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
