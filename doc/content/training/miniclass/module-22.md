# Module 22 - Networking - Bridged Adapters and NAT Networks

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-22-bridged-adapters-and-nat-networks/).

## Introduction

In common virtualization software, there is this concept of a Bridged Adapter and a NAT Network. You can create both of these topologies in minimega.

## Creating a Bridge

First you will need to create a bridge.

This should only be done from a machine you have physical access to as networking will fail temporarily. Depending on a number of factors it may not come back.

```
$ su -
# ovs-vsctl add-br mega_bridge
# ovs-vsctl set bridge mega_bridge stp_enable=false
# cat > createbridge.sh << EOF
#!/bin/sh
ovs-vsctl add-port mega_bridge eth0
dhclient -r eth0
dhclient mega_bridge
EOF
# chmod +x createbridge.sh
# tmux
# ./createbridge.sh
```

The server will now move the `eth0` interface to the newly created `mega_bridge` bridge, remove DHCP lease from `eth0`, and acquire a DHCP lease for `mega_bridge`.

Note: This will cause the network to disconnect and if things go wrong you will have to connect with keyboard and mouse manually to fix networking.

On reboot `mega_bridge` will not get an IP address by default and you will have to run `dhclient mega_bridge`, unless it is added to `/etc/network/interfaces`.

Do not run `nuke` until the bridge is removed as this will kill the `mega_bridge` interface and disable networking.

### Troubleshooting

Sometimes `eth0` won't want to freely give up the IP and it takes some forcing.

```
ifconfig eth0 down
ifconfig eth0 up
ip addr del 192.168.1.100 dev eth0
dhclient -r eth0
dhclient -r mega_bridge
dhclient mega_bridge
```

## Bridged Adapters

Bridged Adapters function as if you had a networking interface that was directly connected to your network.

### Cleanup

```
$ vm kill all
$ vm flush
```

Now you can start some VMs and test your Bridged Adapters by placing VMs on VLAN 0 of `mega_bridge`.

### Boot

```
# minimega -attach
vm config disk /home/ubuntu/tinycore.qcow
vm config memory 128
vm config net 0
vm launch kvm linux[1-5]
vm start all
```

Your VMs should now be able to reach your network as if they were directly connected, enabling your VMs to access the internet directly.

To undo the change you made you will need to delete the bridge and use `dhclient` to request an IP address again for your Ethernet adapter.

## Nat Network

NAT Networks have a DHCP server that is running that also acts like a router forwarding traffic onto the internet for them. This can be accomplished by combining a `dnsmasq` service and `iptables` rules.

### Cleanup

```
$ vm kill all
$ vm flush
```

Now you can start some VMs and create a NAT Network.

### Boot

```
# minimega -attach
vm config disk /home/ubuntu/tinycore.qcow
vm config memory 128
vm config net 100
tap create 100 ip 10.0.0.1/24
shell sleep 5
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
shell sleep 5
vm launch kvm linux[1-5]
shell sleep 5
vm start all
disconnect
```

Now let's create some `iptables` rules:

```
WAN=mega_bridge
sysctl -w net.ipv4.ip_foward=1
iptables -t nat -A POSTROUTING -o $WAN -j MASQUERADE
iptables -A FORWARD -i $WAN -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -A FORWARD -o $WAN -j ACCEPT
```

Your VMs should receive a 10.0.0.x IP address from `dnsmasq` and `iptables` will forward network traffic to and from the network. Enabling your VMs to have internet access through the server's connection.

If need be you can delete all the existing `iptables` rules and stop the VM connection to the internet with

```
iptables -P INPUT ACCEPT
iptables -P OUTPUT ACCEPT
iptables -P FORWARD ACCEPT
iptables -F
iptables -F -t mangle
iptables -F -t nat
```

### Special Note

With a tap interface VMs on the VLAN will be able to `ssh` into 10.0.0.1 and connect to the outside.

## Removing the Bridge

If you wish to undo the setup steps that created the bridge, you can do the following.

Note: These steps will disable network access if done incorrectly; proceed with caution.

```
$ su -
# cat > fixeth.sh << EOF
ovs-vsctl del-br mega_bridge
ifconfig eth0 down
ifconfig eth0 up
dhclient -r eth0
dhclient eth0
EOF
# chmod +x fixeth.sh
# tmux
# ./fixeth.sh
```
