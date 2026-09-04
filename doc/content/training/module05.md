# VM Networking

> How to configure an experiment network

<a id="TOC_1."></a>

## Overview

minimega uses software-defined networking to connect VMs on one or more virtual
networks. These networks can span a single node, a cluster, or across clusters.

See [Module 14](module14.md) for more information about VM networking on a cluster.

<a id="TOC_2."></a>

## APIs

This module covers the following APIs:

- `vm config networks`
- `vlans`
- `tap`
- `bridge`

<a id="TOC_3."></a>

## Configuring networks

When configuring a VM, the user decides the bridge, VLAN, MAC, and driver for
each VM interface. Endpoint VMs typically have a single interface while routers
may have several.

```text
vm config networks [netspec]...
```

Where netspec is:

```text
<bridge>,<VLAN>,<MAC>,<driver>
```

Only the VLAN is mandatory -- the other fields have sane defaults.

<a id="TOC_4."></a>

## What is a VLAN?

A VLAN is an extra 12-bit identifier in the MAC frame that logically separates
LANs that share the same physical network.

VMs on the same VLAN are in the same LAN. VMs in different VLANs must be
connected by a router that spans the VLANs.

The 12-bit identifier means that there are 4,094 possible VLANs. 0x000, 0xFFF,
and 0x001 are typically reserved.

<a id="TOC_5."></a>

## Configuring networks, examples

To connect a VM to VLANs 100 and 101:

```text
vm config net 100 101
```

To connect a VM to VLANs 100 and 101 with specific mac addresses:

```text
vm config net 100,00:00:00:00:00:00 101,00:00:00:00:01:00
```

To connect a VM to VLAN 100 on bridge0 and VLAN 200 on bridge1:

```text
vm config net bridge0,100 bridge1,200
```

To connect a VM to VLAN 100 on bridge0 with a specific mac:

```text
vm config net bridge0,100,00:11:22:33:44:55
```

To specify a specific driver, such as i82559c:

```text
vm config net 100,i82559c
```

<a id="TOC_6."></a>

## VLAN aliases

In the previous example, we used explicit VLAN identifiers like 100 and 101.
minimega support VLAN aliases to make them easier to work with:

```text
vm config net DMZ CORE
```

DMZ and CORE will automatically be assigned to an available VLAN identifier on
their first use. Otherwise, they will mapped to the stored value.

<a id="TOC_7."></a>

## VLAN aliases

The `vlans` API allows users to print, add, and configure VLAN aliases:

```text
minimega$ vlans
alias | vlan
CORE  | 102
DMZ   | 101
```

You can manually add a VLAN using `vlans add`:

```text
vlans add FOO 1000
```

To limit aliases to a particular identifier range, use `vlans range`.

<a id="TOC_8."></a>

## VLAN blacklist

You may also blacklist identifiers that minimega should not use for new
aliases:

```text
minimega$ vlans blacklist 1000
minimega$ vlans blacklist
vlan
1000
```

This happens automatically if you specify a VLAN identifier in
`vm config networks`:

```text
minimega$ vm config net 1001
minimega[minimega]$ vlans blacklist
vlan
1000
1001
```

<a id="TOC_9."></a>

## Creating host taps

Once VMs are running, it can be useful to access the VM network from the host.
This is achieved by creating a host tap on the DMZ VLAN:

```text
minimega$ tap create DMZ
mega_tap0
```

This interface can be given an IP using external tools or directly in minimega:

```text
tap create DMZ ip 10.0.0.1/24
```

See [this article](../articles/nat.md) for how to connect a VM to the Internet.

<a id="TOC_10."></a>

## Mirrors and trunks

Accessing the "live" VM traffic can be useful to understand what the VMs are
doing. There are two ways to do so: `tap mirror` and `bridge trunk`.

`tap mirror` mirrors one tap to another.

`bridge trunk` mirrors all traffic from the bridge to an interface.

See [this article](../articles/mirror.md) on tap mirroring for more information.

minimega also supports capturing packets. You can read up on the capture API in the [API Documentation](../reference/minimega.md).

<a id="TOC_11."></a>

## Next up…

[Module 06: Experimental network](module06.md)
