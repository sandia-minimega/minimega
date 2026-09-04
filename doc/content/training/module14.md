# Expanding to a cluster

> How to deploy minimega across a cluster of compute nodes

<a id="TOC_1."></a>

## Overview

minimega includes several capabilities to ease scheduling VMs across a cluster.
This module covers the basics including minimega's peer discovery, VM
scheduler, and file transport.

See the [cluster article](../articles/cluster.md) for more information.

<a id="TOC_2."></a>

## APIs

- `mesh`
- `ns`
- `vm config schedule`
- `vm config coschedule`
- `vm config colocate`
- `file`

<a id="TOC_3."></a>

## meshage

Assuming that you have started minimega across several hosts on the same
subnet, minimega will automatically discovery its peers if minimega's degree is
greater than the number of peers.

```text
minimega$ mesh status
host  | mesh size | degree | peers | context  | port
node1 | 10        | 3      | 6     | minimega | 9000
```

In this cluster, there are a total of 10 hosts. This node has six peers and a
degree of ten (set via `mesh degree`).

The mesh topology can be inspected with `mesh list`.

<a id="TOC_4."></a>

## meshage commands

`mesh send` can launch commands on one or more remote minimega instances. For
example:

```text
mesh send all host
```

Would display the `host` information for each instance.

There are a few commands that cannot be run over `mesh send` including
`mesh send` and `read`.

<a id="TOC_5."></a>

## namespaces

Beyond providing weak multi-tenancy, namespaces simplify minimega's usage
across a cluster. The default namespace, `minimega`, only includes the local
node but any new namespace contains all the meshage-reachable hosts.

```text
minimega$ namespace foo
minimega$ ns hosts
node[1-10]
```

The `ns` API can be used to display, add, and remove hosts from the namespace.

<a id="TOC_6."></a>

## namespaces

Most commands are namespace-aware and will run across the hosts that are in the
current namespace:

```text
vm info
```

Returns VMs running on node[1-10] in namespace `foo`.

For more information on namespace, see [module 13](module13.md)

<a id="TOC_7."></a>

## VM scheduler

minimega uses a simple VM placement algorithm based on Least-loaded-first
(LLF). When launching a new VM, minimega queries the host stats for all the
hosts in the namespace. It then computes load case on:

- CPU commit: total CPU commit divided by number of CPUs (default)
- NIC commit: total NICs
- Mem commit: total memory commit divided by total memory

Load is determined based on static VM vCPUs, interfaces, and memory. The load
calculation can be tweaked with the `ns` API:

```text
minimega$ ns load
cpucommit
minimega$ ns load netcommit
```

<a id="TOC_8."></a>

## VM queueing

By default, minimega launches VMs immediately on `vm launch`. This can be
inefficient both in terms of scheduler overhead and the VM placement may be
suboptimal.

To make VM placement more efficient, minimega supports VM queueing. When a VM
is launched, it will be placed into a queue instead of actually launching.

When the user is ready, she may launch all the queued VMs by calling
`vm launch` with no arguments.

```text
ns queueing true
vm launch kvm 100
vm launch
```

<a id="TOC_9."></a>

## Tinkering with VM placement

minimega includes many ways to customize VM placement:

- `vm config schedule`: schedule VM on specific host
- `vm config coschedule`: limit number of VMs schedule with VM
- `vm config colocate`: schedule VM on the same host as another VM

VM placement can also be modified pre-flight using the `ns sched dry-run` API.

```text
ns sched dry-run
ns mv <vm target> <dst>
```

<a id="TOC_10."></a>

## VM networking

Typically, we trunk VLAN-tagged packets to a layer-2 switch which allows VMs on
separate hosts to communicate. This is enabled by adding a physical NIC as a
port to `mega_bridge`:

```text
ovs-vsctl add-port mega_bridge $NIC
```

<a id="TOC_11."></a>

## Creating tunnels

However, not all switches support VLANs or nested VLANs (in cases where the
hosts might already part of a VLAN such as on a cloud provider).

In these environments, we use the `bridge tunnel` API to encapsulate the
layer-2 traffic in an layer-3 packet:

```text
bridge tunnel <vxlan,gre> <bridge> <remote IP>
```

`GRE` and `VXLAN` represent two different encapsulation methods.

Tunnels can also be used to link clusters that span more than a single layer-2
switch.

<a id="TOC_12."></a>

## Transferring files

**iomeshage** is the meshage-based file transfer layer provided by minimega.

iomeshage is a distributed file transfer layer that provides a means to very quickly copy files between minimega nodes.

By leveraging minimega's 'meshage' message passing protocol, iomeshage can exceed transfer speeds obtained with one-to-one copying.

<a id="TOC_13."></a>

## iomeshage overview

There are two ways to use iomeshage - through the file API and via an inline file: prefix available anywhere on the command line.

In order for iomeshage to locate files on remote nodes, the files **must be located in the filepath directory** provided to minimega (by default /tmp/minimega/files).

iomeshage supports transferring single files, globs (wildcard files such as foo*), and entire directories. Permissions on transferred files are preserved.

**CAUTION:** iomeshage uses filenames (including the path) as the unique identifier for that file. For example, if two nodes have a file "foo", which is different on each node, iomeshage will have undefined behavior when transferring the file.

<a id="TOC_14."></a>

## 'file' API

iomeshage can be invoked using the file API on any node. It doesn't matter which remote node the file is on, so long as it exists on at least one node. For example, to find and transfer a file 'foo' to the requesting node's filepath directory:

```text
file get foo
```

If the file exists, the command will return with no error. The file API is non-blocking - it will return immediately and enqueue the file transfer. To see the status of existing file transfers, use the file status API:

```text
minimega$ file get bigfile
minimega$ file status
host  | Filename | Temporary directory                    | Completed parts | Queued
foo   | bigfile  | /tmp/minimega/files/transfer_442933642 | 65/103          | false
```

<a id="TOC_15."></a>

##

You can also list and delete files using the file API:

```text
minimega$ file list
host  | dir | name    | size
foo   |     | bigfile | 1073741824
minimega$ file delete bigfile
minimega$ file list
minimega$
```

File transfers are always done by 'pulling' the file to the requesting node.

There is no way to transfer a file to a remote node directly.

In such cases, you will need to tell the remote node to pull the file using the mesh API.

For example, to have remote node foo pull a file bar from the mesh:

```text
mesh send foo file get bar
```

<a id="TOC_16."></a>

## file: prefix

iomeshage can also be invoked anywhere on the command line by prefixing the file you want to transfer to the local node with file:.

For example, if a remote node has a file foo.qcow2, and you want to use it locally as a disk image:

```text
vm config disk file:foo.qcow2
```

This will transfer the file foo.qcow2 to the local node, and block until the file transfer is complete.

Once complete, the path will be replaced with local reference to the file:

```text
minimega$ vm config disk file:foo.qcow2
minimega$ vm config disk
[/tmp/minimega/files/foo.qcow2]
minimega$
```

Additionally, file: can by tab completed across the mesh in the same way as bash.

<a id="TOC_17."></a>

## tar: prefix

The tar: prefix fetches and untars tarballs via meshage, typically for use as a container filesystem:

```text
minimega$ vm config filesystem tar:containerfs.tar.gz
minimega$ vm config filesystem
[/tmp/minimega/files/containerfs]
minimega$
```

If the tarball contains more than a single top-level directory, it will return an error since the filesystem path is set to the top-level directory inside the tarball.

If the tarball resides outside of the iomeshage directory, minimega will still untar the tarball if it exists on the local node running the container to the same directory where the tarball resides.

<a id="TOC_18."></a>

## http://, https:// prefix

Similar to the file: prefix, an HTTP(s) URL can be supplied anywhere minimega expects a file on disk.

minimega will block while it downloads the file to the iomeshage directory.

If the file already exists in iomeshage (on any node), the iomeshage version will be fetched instead of requesting the file from the URL.

<a id="TOC_19."></a>

## http://, https:// prefix - example

To create a VM based on an Ubuntu cloud image:

```text
minimega$ vm config disk https://uec-images.ubuntu.com/releases/14.04/release/ubuntu-14.04-server-cloudimg-amd64-disk1.img
minimega$ vm config disk
[/tmp/minimega/files/releases/14.04/release/ubuntu-14.04-server-cloudimg-amd64-disk1.img]
minimega$ file list
dir   | name              | size
<dir> | miniccc_responses | 40
<dir> | releases          | 60
minimega$ file list releases
dir   | name  | size
<dir> | 14.04 | 60
minimega$ file list releases/14.04
dir   | name    | size
<dir> | release | 60
minimega$ file list releases/14.04/release/
dir  | name                                         | size
     | ubuntu-14.04-server-cloudimg-amd64-disk1.img | 259785216
```

<a id="TOC_20."></a>

## Completed!

Congratulations! You have made it all the way through the minimega training modules.

Now you can go out and deploy your own at-scale cyber testbed.

Or contribute your own ideas and code for minimega at [our GitHub repository.](https://github.com/sandia-minimega/minimega)

Thank you!

--the minimega team
