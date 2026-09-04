# Module 32 - Using Meshage

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-32-using-meshage/).

## Introduction

minimega is designed to scale by running instances on individual nodes that communicate over a message passing protocol, meshage. This protocol can be used to configure experiments in minimega.

## meshage Commands

Here are a list of the mesh commands available. Help will give you more information on each of those.

```
mesh degree      :       view or set the current degree for this mesh node
mesh dial        :       attempt to connect this node to another node
mesh dot         :       output a graphviz formatted dot file
mesh hangup      :       disconnect from a client
mesh list        :       display the mesh adjacency list
mesh send        :       send a command to one or more connected clients
mesh status      :       display a short status report of the mesh
mesh timeout     :       view or set the mesh timeout
```

meshage commands consist of a normal command prefixed by a meshage operator. For example, to send the minimega command `host name` to node `m2` from node `m1`:

```
m1# minimega -attach
$ mesh send m2 host name
m2: m2
```

Any response or error from `m2` will be printed.

The `mesh send` prefix supports grouping just like other minimega commands

```
$ mesh send m[2-5] host name
```

However the `mesh send all` command will issue the command on all nodes, but itself

```
m1# minimega -attach
$ mesh send all host name
m2: m2
m3: m3
m4: m4
m5: m5
```

## Example

```
m1# cp tinycore.iso /tmp/minimega/files/
m1# minimega -attach
$ mesh send all vm kill all
$ mesh send all vm flush
$ mesh send all clear vm config
$ mesh send all vm config cdrom file:tinycore.iso
$ mesh send all vm config memory 128
$ mesh send all vm launch kvm lin[1-3]
$ mesh send all vm start all
```

```
$ mesh send all .columns name
minimega:/tmp/minimega/minimega$ mesh send all .columns name vm info
host | name
m2   | lin1
m2   | lin2
m2   | lin3
m3   | lin1
m3   | lin2
m3   | lin3
m4   | lin1
m4   | lin2
m4   | lin3
m5   | lin1
m5   | lin2
m5   | lin3
```

## Important Notes

miniccc is distributed across the mesh.

miniplumber is distributed across the mesh.

VLANs are shared amongst different nodes regardless of mesh status.

`dnsmasq` services are shared amongst different nodes regardless of mesh status.

Only one instance of miniweb is required to be running to access VMs on a mesh, typically off the head node.

VNC Recording is only possible from nodes running miniweb and from users connecting to that miniweb.
