# Module 44 - Troubleshooting

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-48-troubleshooting/).

## Common Problems

### Lingering taps after nuke

Try deleting the bridge to remove all lingering taps

```
ovs-vsctl del-br mega_bridge
```

### Taps don't work as expected

Make sure you are on the proper VLAN. Sometimes people assign VLANs for actual VLANs, double-check where you are.

### minimega won't compile

Double-check you have all the dependencies

### Something crashed

- Make sure log level debug being enabled didn't cause the crash. This can fill your disk.
- Review the logs if you have any.
- Report repeatable bugs as issues on GitHub.
- Reboot the server.
- Reimage your server.
- Try using the latest stable release.

### mesh is not working properly

Make sure all the servers are running the same version.

### Unable to modprobe kvm_intel

- Make sure virtualization is enabled in the BIOS.
- Make sure your hardware supports virtualization using

```
dmesg | grep kvm
```

### VMs are really slow

- Double-check utilization with `top` and make sure you didn't start too many VMs.
- Check for possible hardware faults (failing SSDs) with `smartctl`.
- Make sure the network isn't saturated with activity

```
apt-get install nload
```

### I started a dnsmasq service, but it's not running

Enable logging and you may notice it is immediately exiting with an error message. I've found removing NetworkManager and installing `dnsmasq` from source fixes this.

### I can't connect to one of my VNC sessions from web

Make sure someone isn't directly connecting to that virtual machine with a client. NoVNC doesn't like sharing a session used by `realvnc`/`tightvnc`.

## Network Troubleshooting

When setting up an experiment, numerous issues can prevent VMs from being able to connect to one another. This article describes some troubleshooting steps that may help to resolve the issue.

### Environment

We will use the following simple test environment to concretely describe where potential connectivity issues may arise:

```
# Create three VM environment:
# A --- router --- B

# Create a router between two subnets
clear vm config
vm config kernel $images/minirouter.kernel
vm config kernel $images/minirouter.initrd
vm config net A B
vm launch kvm router

# Configure router, enable dhcp for VMs and OSPF between interfaces
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.1 range 10.0.0.2 10.0.0.2
router router route ospf 0 0
router router interface 1 20.0.0.1/24
router router dhcp 20.0.0.1 range 20.0.0.2 20.0.0.2
router router route ospf 0 1
router router

vm start router

# Create two clients, one in each subnet
clear vm config
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config net A
vm launch kvm A
vm config net B
vm launch kvm B

vm start all
```

In this environment, we assume that we wish to connect from VM A to VM B and vice versa.

### Ping

Ping is one of the most basic connectivity tests. It can be used to ping another IP address on the same subnet or on another subnet if there is a route between them. For example, a working ping from A to the router interface on the same subnet:

```
$ ping -c 3 10.0.0.1
PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
64 bytes from 10.0.0.1: icmp_seq=1 ttl=64 time=0.079 ms
64 bytes from 10.0.0.1: icmp_seq=2 ttl=64 time=0.085 ms
64 bytes from 10.0.0.1: icmp_seq=3 ttl=64 time=0.085 ms

--- 10.0.0.1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2029ms
rtt min/avg/max/mdev = 0.079/0.083/0.085/0.003 ms
```

If there ping cannot reach the destination, it would look something like:

```
$ ping -c 3 30.0.0.1
PING 30.0.0.1 (30.0.0.1) 56(84) bytes of data.
From 10.0.0.1 icmp_seq=1 Destination Host Unreachable
From 10.0.0.1 icmp_seq=2 Destination Host Unreachable
From 10.0.0.1 icmp_seq=3 Destination Host Unreachable

--- 30.0.0.1 ping statistics ---
3 packets transmitted, 0 received, +3 errors, 100% packet loss, time 2043ms
```

Ping can be used to differentiate between a subnet issue and a routing issue. If VM A can ping VM B, we are in great shape, we will likely be able to connect from VM A to B. If we cannot, we should test whether VM A can ping the router interface on the same subnet (i.e. 10.0.0.1). If it can, we should then test whether VM A can ping the router interface on the B subnet (i.e. 20.0.0.1). If we cannot, we most likely have a routing issue. If VM A cannot ping the router at all, we most likely have a subnet issue.

### Subnet Issues

There are a number of reasons that VM A might not be able to ping the router interface on the same subnet. We enumerate a few below:

```
- Mistyped VLANS: when configuring the VLANs, one or both of the VLANs were mistyped. VMs must be on the same VLAN to ensure layer 2 connectivity.
- Duplicate IPs: the IP addresses of the VMs are identical. VMs must have distinct IP addresses on the same subnet to ensure layer 3 connectivity.
- Subnets misconfigured: the IP address of the VMs are in different subnets. The broadcast address must be the same to ensure layer 3 connectivity.
```

A helpful tool to enumerate the IPs in a subnet is available on the [Go Playground](https://play.golang.org/p/m8TNTtygK0). Simply change the CIDR on line 10 and then hit run and the output will be all the IPs in the subnet. If both IPs are not in the list, then you will not have layer 3 connectivity. The last IP in the list is the broadcast IP and is typically not used by an individual VM.

`ip neighbor` can be used to list the layer 2 neighbors for an interface:

10.0.0.1 dev eth0 lladdr 00:19:7e:7d:2b:d2 REACHABLE

If the table is empty or the entry says `FAILED`, there is likely a layer 2 issue.

`tcpdump` can be used to sniff traffic on an interface. This can be helpful to see if packets are making it to the VM but are being dropped by the kernel. For example:

```
$ tcpdump -i eth0
```

If you can see pings but are not getting a response, there is likely a subnet misconfiguration or a firewall blocking pings.

### Routing issues

Routing issues can be difficult to diagnose, especially in large networks. Here, we list some basic troubleshooting ideas but it is far from exhaustive.

```
- Double check all IPs and masks
- [[http://bird.network.cz/?get_doc&v=20&f=bird-4.html][Check bird status]] (if using minirouter)
```

### Authors

The minimega authors

Created: 27 Jun 2017

Last updated: 26 May 2022
