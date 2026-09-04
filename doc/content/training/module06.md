# Experimental network

> How to leverage minimega's tools for augmenting your network topology

<a id="TOC_1."></a>

## VM Network for experiments

In [module 05](module05.md) we looked at how to configure network information like VLANs for your VMs. We recommend reviewing that module before working through this one.

In this module, we'll look at:

- configuring an experiment network
- building the topology
- the minimega tools to accomplish this.

The first step is assigning IPs to your VMs, and there are numerous ways we can do that.

Of course, you can always manually specify the IPs for each and every VM, but that would quickly become impractical.

Fortunately, minimega offers several options for automating this process.

<a id="TOC_2."></a>

## Automated IP assignment - DHCP with DNSMASQ

DHCP is a quick way of assigning IPs to your VMs. The minimega toolset includes DNSMASQ which can serve as a DHCP server for your experimental network. Let's look at how we can set that up.

DNSMASQ will run on the same host as the VMs, not on a VM itself. Thus, we need a way to allow the host to access the experiment network, and we can do that using a tap:

```text
tap create 100 ip 10.0.0.1/24
```

Let's unpack that command a bit:

- 'tap create' invokes the tap api in minimega to create a new tap
- '100' is the vlan we specify - as always this can be any value between 1-4096

    -  the specified vlan must match the vlan of the VMs you are serving IPs to!

- 'ip 10.0.0.1/24' allows us to specify the ip address we want to assign the tap, which we will need for DNSMASQ

<a id="TOC_3."></a>

##

We covered taps in the previous module. Head back to [Module 05](module05.md) if you need a review.

In addition to creating the tap, we also need to define an environment to network together.
To reiterate, be sure to have the VMs exist on the same VLAN as the tap, to ensure communication with DHCP.

```text
vm config net 100
vm config disk foo.qc2
vm launch kvm foo[1-10]
```

With the tap in place and the VMs defined and launched, let's launch DNSMASQ

```text
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
```

This command tells DNSMASQ to listen on 10.0.0.1, distributing IPs in range 10.0.0.2 through 10.0.0.254

<a id="TOC_4."></a>

## Putting it all together

From start to finish, here are the commands needed to accomplish automating the assignment of IPs using DNSMASQ as a DHCP server running on the host.

```minimega title="dnsmasq.mm"
--8<-- "training/module06_content/dnsmasq.mm"
```

[Download this example](module06_content/dnsmasq.mm){ download="dnsmasq.mm" }

<a id="TOC_5."></a>

## Taking it Further

You can run multiple DHCP servers on multiple VLANs.

```text
tap create 100 ip 10.0.0.1/24
tap create 200 ip 20.0.0.1/24
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
dnsmasq start 20.0.0.1 20.0.0.2 20.0.0.254
```

Print all running DNSMASQ DHCP servers with the command by itself:

```text
minimega$ dnsmasq
host   | ID | Listening Address | Min      | Max        | Path                            | PID
ubuntu | 1  | 20.0.0.1          | 20.0.0.2 | 20.0.0.254 | /tmp/minimega/dnsmasq_264204396 | 3610
```

Stop with the kill command

```text
minimega$ dnsmasq kill 1
ubuntu | 0  | 10.0.0.1          | 10.0.0.2 | 10.0.0.254 | /tmp/minimega/dnsmasq_826235649 | 3216
```

<a id="TOC_6."></a>

## other commands

- Starting dnsmasq with a configuration file
- Set up a static IP allocation for a VM with a specified MAC address
- Add a DNS entry resolving a domain to an ip
- Add a DHCP option

See [the minimega API](../reference/minimega.md) for more details on dnsmasq, or help in minimega:

```text
minimega$ help dnsmasq
```

Note: DNSMASQ runs on the host, which means the host itself becomes part of your experiment's network. For most experiments, it's better to use minirouter (or another dedicated VM) to serve DHCP instead, so the host stays isolated from the experiment network. See the next section for details.

<a id="TOC_7."></a>

## minirouter

minirouter is a simple tool, run in a VM, that orchestrates various router functions such as DHCP, DNS, IPv4/IPv6 assignments, and, of course, routing. The minirouter tool is interfaced by minimega's router API and the minimega distribution provides a prebuilt minirouter container image.

minirouter currently supports several protocols and capabilities including DHCP, DNS, router advertisements, static routes, OSPF, and BGP. It can route in excess of 40 gigabits per second when running as a container.

minirouter can run on bare metal, as a container, or a KVM image.

vmbetter can be used to create and deploy a minirouter image (kernel/initrd pair or container filesystem). For more information on using vmbetter, see [module 2.5: Better vmbetter](module02_5.md)

<a id="TOC_8."></a>

## Running minirouter without an image

minirouter is simply a Linux binary that can run on any Linux system. You do not specifically need to build an image to run it, although it is more convenient.

To use minirouter, you must have the miniccc agent running, and minirouter must be able to access the miniccc tool and files directory (see minirouter -h for default paths).

minirouter uses iptool, dnsmasq, dhclient, and bird, all of which must be installed but not already running. minirouter must run as root.

Beyond these few requirements, minirouter should run on most linux systems.

<a id="TOC_9."></a>

##

In this module, we will use minirouter to act as DHCP to assign IPs and also to assign static IPs to VMs.

VMs running the minirouter tool must have miniccc running as well (this is already configured in the prebuilt minirouter image).

for more information on miniccc, minimega's command and control tool, see [module 07](module07.md)

<a id="TOC_10."></a>

## Starting minirouter

The router API requires a VM name or ID when configuring a router. For example, to set a static IP on a running minirouter VM named 'foo':

```minimega title="router01.mm"
--8<-- "training/module06_content/router01.mm"
```

[Download this example](module06_content/router01.mm){ download="router01.mm" }

While the first command above sets the configuration for the router image, the second line actually commits the configuration by sending commands to minirouter over the command and control layer in minimega. Multiple configuration commands can be issued and then later committed with a single commit command.

<a id="TOC_11."></a>

## Interfaces

Routers often have statically assigned IP addresses and minirouter supports both IPv4 and IPv6 address specification using the interface API. For example, to add the IP 10.0.0.1/24 to the second interface on a minirouter VM:

```minimega title="router02.mm"
--8<-- "training/module06_content/router02.mm"
```

[Download this example](module06_content/router02.mm){ download="router02.mm" }

Multiple addresses can be added to the same interface as well:

```minimega title="router03.mm"
--8<-- "training/module06_content/router03.mm"
```

[Download this example](module06_content/router03.mm){ download="router03.mm" }

<a id="TOC_12."></a>

## DHCP with minirouter

minirouter supports DHCP assignment of connected clients and supports both IP range and static IP assignment. minirouter also supports several DHCP options such as setting the default gateway and nameserver.

For example, to serve the IP range 10.0.0.2 - 10.0.0.254 on a 10.0.0.0/24 network, specify the network prefix and DHCP range:

```minimega title="router04.mm"
--8<-- "training/module06_content/router04.mm"
```

[Download this example](module06_content/router04.mm){ download="router04.mm" }

All of these DHCP options can be used together in a single DHCP specification, and multiple DHCP servers can be specified on a single minirouter instance (for serving DHCP on multiple interfaces/networks).

<a id="TOC_13."></a>

## minirouter - other features

- IPv6 Router Advertisements

minirouter supports IPv6 router advertisements using the Neighbor Discovery Protocol to enable SLAAC addressing. To enable route advertisements simply provide the subnet. Only the subnet prefix is required as SLAAC addressing requires a /64 and is implied.

```minimega title="router05.mm"
--8<-- "training/module06_content/router05.mm"
```

[Download this example](module06_content/router05.mm){ download="router05.mm" }

- DNS

minirouter provides a simple mechanism to add A or AAAA records for any host/IP (including IPv6) pair. Simply specify the host and IP address of the record:

```minimega title="router06.mm"
--8<-- "training/module06_content/router06.mm"
```

[Download this example](module06_content/router06.mm){ download="router06.mm" }

<a id="TOC_14."></a>

## Routing

minirouter uses the bird routing daemon to provide routing using a variety of protocols. minirouter supports static routes, OSPF, and BGP.

Bird is a lightweight routing daemon that scales well. In our tests we were able to scale minirouter with bird to at least 40 gigabit

<a id="TOC_15."></a>

## Routing - Static Routes

minirouter makes possible adding IPv4 or IPv6 static routes by simply specifying the destination network and net-hop IP. For example, to add a static IPv4 route for the 1.2.3.0/24 network via 1.2.3.254:

```minimega title="router07.mm"
--8<-- "training/module06_content/router07.mm"
```

[Download this example](module06_content/router07.mm){ download="router07.mm" }

<a id="TOC_16."></a>

## Routing - OSPF

minirouter provides basic support for OSPF and OSPFv3 (IPv6 enabled OSPF) by specifying the OSPF area and interface to include in the area. OSPF generally supports specifying networks and many other options, which minirouter may add in the future. For now, specifying an interface (and all of the networks on that interface) is provided. Both OSPF and OSPFv3 are enabled by minirouter.

Interfaces are identified by the index in which they were added by the vm config net API. For example, to add the first and third network of the router VM to area 0 in an OSPF route:

```minimega title="router08.mm"
--8<-- "training/module06_content/router08.mm"
```

[Download this example](module06_content/router08.mm){ download="router08.mm" }

OSPF route costs can also be tuned to influence path selection. See [the OSPF cost article](../articles/ospf_cost.md) for details.

<a id="TOC_17."></a>

## Routing - the full router API

We've only covered a subset of the `router` API in this module. The router API includes commands for interfaces, DHCP, DNS, upstream DNS, default gateway, router advertisements, routes, router ID, and firewall rules (about 9 subcommands in total).

See [the minimega API](../reference/minimega.md) for the full list, or in minimega:

```text
minimega$ help router
```

<a id="TOC_18."></a>

## Connecting to the internet

It is sometimes useful to connect the experiment network to the Internet to install software or access external resources. [This article](../articles/nat.md) describes the simple case of connecting a single VM to the Internet. With minor changes, this technique can also be used to connect an entire experiment to the Internet if the single VM acts as a router.

The VM must be configured with at least one network interface.

To connect this interface to the Internet, we setup a NAT on the host machine by creating a tap. Also, we need to enable IP forwarding on the host machine. And finally, configuring iptables to enable the NAT on the host machine. You may also need to configure DNS on the VM.

<a id="TOC_19."></a>

##

```minimega title="nat.mm"
--8<-- "training/module06_content/nat.mm"
```

[Download this example](module06_content/nat.mm){ download="nat.mm" }

<a id="TOC_20."></a>

## Troubleshooting

When setting up an experiment, numerous issues can prevent VMs from being able to connect to one another.

For more information, visit the [network troubleshooting article](../articles/troubleshooting.md).

<a id="TOC_21."></a>

## Next up…

[Module 07: Command and control](module07.md)
