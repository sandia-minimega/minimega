# VLAN Aliases


<a id="TOC_1."></a>

## Introduction

Since version 2.3, minimega supports automatic VLAN allocation and aliasing.
Previously, users would have to specify and manage VLANs by hand (e.g. DMZ =
VLAN 100). Now, minimega assigns VLANs automatically via aliases:

```text
minimega$ vm config net DMZ
minimega$ vm config net
[DMZ (2)]
```

<a id="TOC_1.1."></a>

### vlans API

In the above example, minimega automatically assigned VLAN 2 to the alias
`DMZ`. You may inspect the current VLAN assignments using the `vlans` API:

```text
minimega$ vlans
namespace | alias | vlan
          | DMZ   | 2
```

Here we can clearly see that DMZ is assigned to VLAN 2 in the default
namespace. VLAN aliases are namespace-specific. This means that you may reuse
the same aliases across experiments and they will be assigned to distinct
VLANS:

```text
minimega$ namespace dev
minimega[dev]$ vm config net DMZ
minimega[dev]$ vlans
namespace | alias | vlan
dev       | DMZ   | 3
minimega[dev]$ clear namespace
minimega$ vlans
namespace | alias | vlan
          | DMZ   | 2
dev       | DMZ   | 3
```

As shown above, the `vlans` API only prints aliases for the current namespace,
if one is active.

If users wish to alias VLANs across namespace boundaries, they may use the
following syntax:

```text
minimega$ namespace dev2
minimega[dev2]$ vm config net dev//DMZ
minimega[dev2]$ vm config net
[dev//DMZ (3)]
```

By default, minimega starts assigning VLANs from VLAN 2. If users wish to
restrict the range of VLANs that minimega allocates from, they may do using the
`vlans range` API:

```text
minimega[dev2]$ vlans range 100 200
```

Now, aliases for dev2 will be restricted to [100, 200). minimega ensures that
VLAN ranges for different namespaces do not overlap. Calling `vlans range`
without any arguments will display the user-specified ranges.

Sometimes, experiments require that some aliases map to specific VLANs. To fix
the `DMZ` alias to VLAN 999, use the `vlans add` API:

```text
minimega[dev2]$ vlans add DMZ 999
minimega[dev2]$ vlan
namespace | alias | vlan
dev2      | DMZ   | 999
```

VLANs may become blacklisted if users use the directly:

```text
minimega[dev2]$ vm config net 222
2016/03/11 08:18:33 WARN vlans.go:279: Blacklisting manually specified VLAN 222
```

A blacklisted VLAN will not be used by minimega when assigning a VLAN to a new
alias because minimega assumes that the user is doing something special with
that VLAN. Users may also use this feature to blacklist VLANs by hand:

```text
minimega[dev2]$ vlans blacklist 333
```

<a id="TOC_1.2."></a>

### clear vlans API

The `clear vlans` API is used to delete aliases. When run with no arguments and
a namespace is active, it will clear all the aliases for the namespace. When
run with no arguments and no namespace is active, it will clear all state
regarding VLAN aliases, including the blacklisted VLANs. When called with an
argument, `clear vlans` will only clear aliases whose prefix matches the
supplied argument:

```text
minimega[dev3]$ vlan
namespace | alias    | vlan
dev3      | DMZ      | 1000
dev3      | EXTERN_1 | 100
dev3      | EXTERN_2 | 200
dev3      | EXTERN_3 | 300
minimega[dev3]$ clear vlans EXTERN
minimega[dev3]$ vlan
namespace | alias | vlan
dev3      | DMZ   | 1000
```

<a id="TOC_1.3."></a>

### Other APIs

In the above examples, we showed how VLAN aliases work with the `vm config net`
API. VLAN aliases should be supported everywhere that accepts a VLAN such as
`tap` and `vm net`.

If you're unsure of the available aliases, you may try tab completion for any
VLAN field.

<a id="TOC_1.4."></a>

### Tracking VLAN aliases

namespaces assumes that the user issues commands to the head node and that
commands may be broadcasted to a cluster of nodes. To prevent total state loss
if the head node were to crash, minimega broadcasts VLAN alias assignments. If
the alias belongs to a namespace, the assignment in only broadcast to the nodes
that are part of the namespace. Otherwise, all nodes receive the update.
