# Module 21 - Networking - Taps and Bridges

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-21-taps-and-bridges/).

## Introduction

We have been using taps and bridges. They have mostly been created automatically for us, and I haven't really explained what they are. Let's fix that.

## Taps

TUN/TAPs are Linux's way of receiving and transmitting packets virtually. I like to think of a tap as a means to hard wire connections in a network diagram.

The tap command in minimega creates a tuntap interface on a VLAN and bridge. It is able to do so by interfacing with OpenVSwitch.

You can print your existing taps using `ifconfig -a`

```
root@ubuntu:~# ifconfig -a
~snip~
mega_bridge Link encap:Ethernet  HWaddr f0:bf:97:dc:5c:5a
          inet addr:192.168.1.104  Bcast:192.168.1.255  Mask:255.255.255.0
          inet6 addr: fe80::f2bf:97ff:fedc:5c5a/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:226860 errors:0 dropped:5 overruns:0 frame:0
          TX packets:217460 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1
          RX bytes:485376723 (485.3 MB)  TX bytes:83997257 (83.9 MB)

mega_tap2 Link encap:Ethernet  HWaddr ca:3e:9b:e6:bf:7c
          inet6 addr: fe80::c83e:9bff:fee6:bf7c/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:338 errors:0 dropped:0 overruns:0 frame:0
          TX packets:8 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:113904 (113.9 KB)  TX bytes:648 (648.0 B)

mega_tap3 Link encap:Ethernet  HWaddr ca:43:14:0c:1d:03
          inet6 addr: fe80::c843:14ff:fe0c:1d03/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:333 errors:0 dropped:0 overruns:0 frame:0
          TX packets:8 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000
          RX bytes:113886 (113.8 KB)  TX bytes:648 (648.0 B)

ovs-system Link encap:Ethernet  HWaddr b6:a0:17:bf:6e:95
          BROADCAST MULTICAST  MTU:1500  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)
~snip~
```

As well as `ovs-vsctl show`

```
root@ubuntu:~# ovs-vsctl show
b6be4600-26d0-4e40-bcb5-b7cf6be5826c
    Bridge mega_bridge
        Port "mega_tap2"
            tag: 100
            Interface "mega_tap2"
        Port "eth0"
            Interface "eth0"
        Port "mega_tap3"
            tag: 200
            Interface "mega_tap3"
        Port mega_bridge
            Interface mega_bridge
                type: internal
    ovs_version: "2.5.0"
```

You can create a tap using the `tap` command. By default it creates a tap with the prefix `mega_tap` on the bridge `mega_bridge`

```
minimega:/tmp/minimega/minimega$ tap create 111
ubuntu: mega_tap8
```

You can optionally specify a name to ignore the prefix

```
tap create 112 name mytap
```

You can also specify the bridge to not be `mega_bridge`

```
tap create 112 bridge notmega_bridge
tap create 113 bridge notmega_bridge name mytapnotonmega
```

You can delete taps by number

```
tap delete 2
```

or all

```
tap delete all
```

On `nuke`, taps also get deleted. (If this fails, you can delete `mega_bridge` with the command `ovs-vsctl del-br mega_bridge`)

```
nuke
```

You can create taps with an IP address

```
tap create 114 ip 1.1.1.1/24 my1tap
```

You can create taps with an IP address from DHCP

```
tap create 114 ip dhcp my1dhcptap
```

Full options:

```
tap
tap <create,> <vlan>
tap <create,> <vlan> name <tap name>
tap <create,> <vlan> <dhcp,> [tap name]
tap <create,> <vlan> ip <ip> [tap name]
tap <create,> <vlan> bridge <bridge>
tap <create,> <vlan> bridge <bridge> name [tap name]
tap <create,> <vlan> bridge <bridge> <dhcp,> [tap name]
tap <create,> <vlan> bridge <bridge> ip <ip> [tap name]
tap <delete,> <id or all>
```

### Special Note on Taps

Taps connect your server into your experiment. Be careful doing this as you might be merging networks you don't want to.

For example:

Going back to the DNSMASQ module if you ss`h` into 10.0.0.1 from a VM you should be able to connect to your server hosting the virtual machines that is running minimega.

You can also use this to tunnel out of the experiment. If you were to run this command on your client:

```
ssh -L 1234:proxyserver:80 ubuntu@myserver
```

a tunnel to the proxy server via your network tap would be opened; this means that if you set your virtual machine's browser proxy to 127.0.0.1:1234 your virtual machine would be able to reach the outside and surf the web.

### Tap Mirroring

Tap mirroring can allow VMs to passively inspect traffic from other VMs to perform network monitoring. This article describes the basic setup.

#### Environment

We will use a simple environment to test the tap mirroring capability:

```
# create two VMs, each with a hardcoded UUID
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config net LAN
vm config uuid 11111111-1111-1111-1111-111111111111
vm launch kvm A
vm config uuid 22222222-2222-2222-2222-222222222222
vm launch kvm B

# create a VM to monitor the other two, also with a hardcoded UUID
vm config net 0
vm config uuid 33333333-3333-3333-3333-333333333333
vm launch kvm monitor

# start all the VMs
vm start all

# set static IP on A
cc filter uuid=11111111-1111-1111-1111-111111111111
cc exec ip addr add 10.0.0.1/24 dev eth0

# set static IP on B
cc filter uuid=22222222-2222-2222-2222-222222222222
cc exec ip addr add 10.0.0.2/24 dev eth0
```

#### Creating the mirror

The `tap mirror` API allows you to create a mirror between two existing taps. In this case, we wish to mirror either A's or B's tap to monitor's tap:

```
minimega$ .column name,tap vm info
name    | tap
A       | [mega_tap0]
B       | [mega_tap1]
monitor | [mega_tap2]
```

The command to create the mirror is then:

```
minimega$ tap mirror mega_tap0 mega_tap2
```

#### Using the mirror

`eth0` on the monitor VM should now see all the traffic that traverses `mega_tap0`. We can confirm this by running `tcpdump -i eth0` on the monitor VM while pinging VM B from VM A:

![Image of ping.png](../../assets/legacy-site/2022/04/ping.png)
![Image of tcpdump.png](../../assets/legacy-site/2022/04/tcpdump.png)

#### Using Bro

Instead of using a simple VM as the monitor, we can replace it with one running [Bro](https://www.bro.org/). minimega includes a `vmbetter` config to build a VM that runs Bro. By default, this VM listens on `eth0` and writes logs to `/bro`. In order to launch the Bro VM, we use the following minimega commands (in place of the previous commands to launch the monitor):

```
# create Bro VM to monitor the network
disk create bro.qcow2 10G
vm config net 0
vm config uuid 33333333-3333-3333-3333-333333333333
vm config kernel $images/bro.kernel
vm config initrd $images/bro.initrd
vm config snapshot false
vm config disk bro.qcow2
vm launch kvm monitor
```

The Bro VM will format and mount any provided disk to `/bro` allowing users to extract the data after the experiment completes. The VM must run with snapshot set to false so that any logs written to disk persist after the VM exits. Each VM must have its own disk image. If a disk is not provided, `/bro` is stored in memory and may cause the VM to run out of memory if there are too many log messages.

To extract files from the QCOW2 after the VM exits, users can run the following bash commands as root:

```
$ modprobe nbd # if not already loaded
$ qemu-nbd -c /dev/nbd0 /path/to/bro.qcow2
$ mount /dev/nbd0 /mnt
$ cp -a /mnt /path/to/dst/
$ umount /mnt
$ qemu-nbd -d /dev/nbd0
```

## Bridges

Bridges allow you to merge taps together. Think of bridges like switches.

Syntax:

```
bridge
bridge trunk <bridge> <interface>
bridge notrunk <bridge> <interface>
bridge tunnel <vxlan,gre> <bridge> <remote ip>
bridge notunnel <bridge> <interface>
```

When called with no arguments, `bridge` displays information about all managed bridges.

To add a trunk interface to a specific bridge, use `bridge trunk`. For example, to add interface `bar` to bridge `foo`:

```
bridge trunk foo bar
```

To create a VXLAN or GRE tunnel to another bridge, use `bridge tunnel`. For example, to create a VXLAN tunnel to another bridge with IP 10.0.0.1:

```
bridge tunnel vxlan, mega_bridge 10.0.0.1
```

When debugging networking issues be sure to check MTU settings. You can send small ping packets with `-l <size>` and `-f flags`

## OpenVSwitch

minimega interfaces to `ovs-vsctl` commands; here are some common ones:

```
ovs-vsctl add-br mybridge
ifconfig mybridge up
ip tuntap add mode tap myport
iconfig myport up
ovs-vsctl add-port mybridge myport tag=112
ovs-vsctl del-port mybridge myport
ovs-vsctl del-br mybridge
```

Note that if you delete a bridge, you delete all of its taps as well.

Other interesting commands:

Print MAC addresses:

```
ovs-appctl fdb/show mega_bridge
```

Map ports to interfaces:

```
ovs-ofctl show mega_bridge
```

View DB entries:

```
ovs-ovctl list Bridge
ovs-ovctl list Port
ovs-ovctl list Interface
```

### Authors

The minimega authors

Published: 30 May 2017

Last updated: 22 April 2022
