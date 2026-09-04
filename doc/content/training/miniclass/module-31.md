# Module 31 - Deploying to Multiple Nodes

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-31-deploying-to-multiple-nodes/).

## Introduction

minimega so far has been shown to manage VMs on a single machine. minimega can in fact support large clusters of machines.

This guide covers the basics of setting up a cluster to run minimega and the process of launching minimega across a cluster. You'll need minimega, either compiled from source or downloaded as a tarball. See the [article on installing minimega](../../articles/installing.md) for information on how to fetch and compile minimega. Although you only need the minimega tree on **one** node, you do need the external dependencies installed on every individual node, so make sure to install those.

Here are some additional docs on doing so:

- [minimega.org/articles/cluster.article](http://minimega.org/articles/cluster.article)
- [minimega.org/articles/namespaces.article](http://minimega.org/articles/namespaces.article)
- [minimega.org/articles/file.article](http://minimega.org/articles/file.article)
- [minimega.org/articles/usage.article](http://minimega.org/articles/usage.article)

### Suggestions:

#### Multiple networks

When using a clustered environment it is strongly recommended to use multiple networks on separate hardware. Have a network for managing your machines and have another for running experiments.

This way virtual machine traffic won't impact management traffic and vice versa.

#### Environment

Install Ubuntu 16.04 with minimega's dependencies on four more machines.

Once completed remote into the host you want to be the "head" node

## Host Configuration:

Before we can get our minimega cluster setup, we need to configure the hosts. We assume that all the nodes can talk to each other via some hostname, if thats not the case, lets get that setup. In our example, we'll use the nodes m1-m5 that are all on the 192.168.1.0/24 network.

### Hosts file

Modify the `/etc/hosts` file to have entries for all your machines to ensure they can all communicate with each other.

```
echo 192.168.1.100 m1 >> /etc/hosts
echo 192.168.1.101 m2 >> /etc/hosts
echo 192.168.1.102 m3 >> /etc/hosts
echo 192.168.1.103 m4 >> /etc/hosts
echo 192.168.1.104 m5 >> /etc/hosts
```

### Ping check

Test you can ping those machines

```
ping m1
<control+c>
ping m2
<control+c>
ping m3
<control+c>
ping m4
<control+c>
ping m5
<control+c>
```

### Enable Passwordless Login

Now that you can ping the machines, lets make our lives easier by reducing the need for passwords. Cluster administration is much easier if you have it set up so you don't have to type a password to log in. You can do password-less login or ssh key authentication:

#### Password-less login:

If your cluster is not accessible to the public, is to simply turn on password-less root login. The following script should set it up:

```
sed -i 's/nullok_secure/nullok/' /etc/pam.d/common-auth
sed -i 's/PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
sed -i 's/PermitEmptyPasswords no/PermitEmptyPasswords yes/' /etc/ssh/sshd_config
passwd -d root
```

#### SSH Login:

Another option is SSH key based login:

Generate a public key

```
root@ubuntu:~/new/minimega# ssh-keygen
Generating public/private rsa key pair.
Enter file in which to save the key (/root/.ssh/id_rsa):
Enter passphrase (empty for no passphrase):
Enter same passphrase again:
Your identification has been saved in /root/.ssh/id_rsa.
Your public key has been saved in /root/.ssh/id_rsa.pub.
<snip>
```

Install the key on `m2`

```
scp /root/.ssh/id_rsa.pub ubuntu@m2:a
ssh ubuntu@m2
sudo mkdir -p /root/.ssh && sudo cp a /root/.ssh/authorized_keys && rm a
exit
```

Install the key on `m3`

```
scp /root/.ssh/id_rsa.pub ubuntu@m3:a
ssh ubuntu@m3
sudo mkdir -p /root/.ssh && sudo cp a /root/.ssh/authorized_keys && rm a
exit
```

Install the key on `m4`

```
scp /root/.ssh/id_rsa.pub ubuntu@m4:a
ssh ubuntu@m4
sudo mkdir -p /root/.ssh && sudo cp a /root/.ssh/authorized_keys && rm a
exit
```

Install the key on `m5`

```
scp /root/.ssh/id_rsa.pub ubuntu@m5:a
ssh ubuntu@m5
sudo mkdir -p /root/.ssh && sudo cp a /root/.ssh/authorized_keys && rm a
exit
```

#### Quick and dirty

another way to push all the keys to other hosts is to use sshpass

Install `sshpass`

```
apt-get install -y sshpass
```

Open `keys.sh` in `nano`

```
nano keys.sh
```

Copy and paste the following, changing the password, sequence numbers, and prefix

```
mypass="password"
for i in `seq 2 5`;
do
    sshpass -p $mypass scp /root/.ssh/id_rsa.pub ubuntu@m$i:a
    sshpass -p $mypass ssh -t ubuntu@m$i "echo $mypass | sudo -S mkdir -p /root/.ssh && sudo cp a /root/.ssh/authorized_keys && rm a"
done
[control+x][control+y][enter]
```

Make `keys.sh` executable

```
chmod +x keys.sh
```

Run `keys.sh`

```
./keys.sh
```

### SSH check

Test you can SSH into those machines without a password.

```
ssh root@m2
exit
ssh root@m3
exit
ssh root@m4
exit
ssh root@m5
exit
```

### Unique hostnames

minimega requires each server to have a unique hostname. Lets ensure each hostname is as expected.

```
ssh root@m2 "echo m2 > /etc/hostname && hostname m2"
ssh root@m3 "echo m3 > /etc/hostname && hostname m3"
ssh root@m4 "echo m4 > /etc/hostname && hostname m4"
ssh root@m5 "echo m5 > /etc/hostname && hostname m5"
```

### Valid hostnames

minimega also requires that each hostname be valid amongst each other, lets setup the hosts on all the other boxes.

```
ssh root@m2 "echo 192.168.1.100 m1 >> /etc/hosts"
ssh root@m2 "echo 192.168.1.101 m2 >> /etc/hosts"
ssh root@m2 "echo 192.168.1.102 m3 >> /etc/hosts"
ssh root@m2 "echo 192.168.1.103 m4 >> /etc/hosts"
ssh root@m2 "echo 192.168.1.104 m5 >> /etc/hosts"

ssh root@m3 "echo 192.168.1.100 m1 >> /etc/hosts"
ssh root@m3 "echo 192.168.1.101 m2 >> /etc/hosts"
ssh root@m3 "echo 192.168.1.102 m3 >> /etc/hosts"
ssh root@m3 "echo 192.168.1.103 m4 >> /etc/hosts"
ssh root@m3 "echo 192.168.1.104 m5 >> /etc/hosts"

ssh root@m4 "echo 192.168.1.100 m1 >> /etc/hosts"
ssh root@m4 "echo 192.168.1.101 m2 >> /etc/hosts"
ssh root@m4 "echo 192.168.1.102 m3 >> /etc/hosts"
ssh root@m4 "echo 192.168.1.103 m4 >> /etc/hosts"
ssh root@m4 "echo 192.168.1.104 m5 >> /etc/hosts"

ssh root@m5 "echo 192.168.1.100 m1 >> /etc/hosts"
ssh root@m5 "echo 192.168.1.101 m2 >> /etc/hosts"
ssh root@m5 "echo 192.168.1.102 m3 >> /etc/hosts"
ssh root@m5 "echo 192.168.1.103 m4 >> /etc/hosts"
ssh root@m5 "echo 192.168.1.104 m5 >> /etc/hosts"
```

#### Quick method

These functions can be shrunk down using custom bash scripts, so you don't have to type these out every time.

Checking `ssh`

```
for i in `seq 2 5`;
do
    ssh root@m$i "echo $i - success"
done
```

Setting hostnames

```
for i in `seq 2 5`;
do
    ssh root@m$i "echo m$i > /etc/hostname && hostname m$i"
done
```

Creating `/etc/hosts`

```
for i in `seq 2 5`;
do
    ssh root@m$i "echo 192.168.1.100 m1 >> /etc/hosts"
    ssh root@m$i "echo 192.168.1.101 m2 >> /etc/hosts"
    ssh root@m$i "echo 192.168.1.102 m3 >> /etc/hosts"
    ssh root@m$i "echo 192.168.1.103 m4 >> /etc/hosts"
    ssh root@m$i "echo 192.168.1.104 m5 >> /etc/hosts"
done
```

### On Node Names:

minimega works best if all nodes have the same prefix followed by a number; this also makes it easier to write shell scripts for administering the cluster. For example, one of our minimega production clusters is called "The Country Club Cluster", so the nodes are named `ccc1`, `ccc2`, `ccc3`, and so on. We recommend against "themed" naming schemes, such as `dopey`, `sleepy`, `grumpy`.

## Network Configuration

Now that all our machines can talk, lets setup minimega for the experiment network. There are multiple ways to accomplish this but we will walk through connecting via physical interfaces in this section. For connecting hosts through Vxlans see the mini-mesh tool:

In order to have VMs on different host nodes talk to each other, we need to make a change to the networking. In short, we will use Open vSwitch to set up a bridge and add our physical ethernet device to that bridge. The bridge will then be able to act as the physical interface (get an IP, serve ssh, etc.) but will **also** move VLAN-tagged traffic from the VMs to the physical network.

If you are in a hurry, you can skip the Background section and go straight to Configuring Open vSwitch.

### Background: Open vSwitch

minimega uses Open vSwitch to manage networking. Open vSwitch is a software package that can manipulate virtual and real network interfaces for advanced functionality beyond standard Linux tools. It can set up vlan-tagged virtual interfaces for virtual machines, then trunk vlan-tagged traffic up to the physical switch connected to the node running minimega.

If your switch supports IEEE 802.1q vlan tagging (and most should), then vlan tagged interfaces with the same tag number should be able to see other interfaces with that tag number, even on other physical nodes. So if you have lots of VMs running across a cluster, as long as they were all configured with the same virtual network via `vm config net`, they will all be able to communicate. If configured correctly, Open vSwitch and your switch hardware will interpret the vlan tag and switch traffic for that vlan as if on an isolated network.

It is also possible to have multiple, isolated vlans running on several nodes in a cluster. That is, you can have nodes A and B both running VMs with vlans 100 and 200, and Open vSwitch and your switch hardware will isolate the two networks, even though the traffic is going to both physical nodes.

If software defined networking and setting up Open vSwitch is new to you, check out the [Open vSwitch website](http://openvswitch.org) for more information.

### Configuring Open vSwitch for cluster operation

minimega by default does **not** bridge any physical interfaces to the virtual switch. In order to allow multiple nodes to have VMs on the same vlan, you must attach a physical interface from each node to the virtual bridge in trunking mode. Doing so will disallow the physical interface from having an IP, you will need to assign an IP (or request one via DHCP) for the new virtual bridge we create.

By default, minimega uses a bridge called `mega_bridge`. If such a bridge already exists, minimega will use it. We will therefore set up a bridge that includes the physical ethernet device.

Let us assume each cluster node has a single physical interface called eth0 and gets its IP via DHCP. We will demonstrate two different ways of setting up the bridge, with the same results: a bridge called `mega_bridge` with `eth0` attached to it.

**NOTE**: NetworkManager may interfere with both methods. We strongly recommend against using NetworkManager, it is unnecessary in a cluster environment (and usually in a desktop environment, too)

#### Shell commands

You can create the bridge manually using the following commands, although **adding eth0 will cause the device to stop responding to the network until you assign an IP to the bridge**, so **do not run these commands over ssh**:

```
$ ovs-vsctl add-br mega_bridge

# This will drop you from the network
$ ovs-vsctl add-port mega_bridge eth0

# Now we can get an IP for the bridge instead
$ dhclient mega_bridge
```

ou can add those commands to e.g. `/etc/rc.local` so they will run on bootup.

#### /etc/network/interfaces

An alternative supported by some distributions is to configure the bridge via `/etc/network/interfaces`. You can add an entry to the file like this:

```
allow-ovs mega_bridge
iface mega_bridge inet dhcp
    ovs_type OVSBridge
    ovs_ports eth0
```

After editing the file, running `service networking restart` should leave you with a `mega_bridge` device that has a DHCP address assigned. It should also come up correctly at boot.

## Deploy minimega

As mentioned above, you only need the minimega tree on one node of the cluster-we'll call this the head node. Using the `deploy` api, minimega can copy itself to other nodes in the cluster, launch itself, and discover the other cluster members to form a mesh. The `deploy` api requires password-less root SSH logins for each node, see the Intro section for more information.

### Start minimega on the head node

On the head node, we launch minimega by hand. Command line flags passed to this instance will be used on all the other instances we deploy across the cluster. The flags we're concerned with are:

```
-degree: specifies the number of other nodes minimega should try to connect to. This is the most important flag! 3 or 4 is a good value.
-context: a string that will distinguish your minimega instances from any others on the network. Your username is usually a good choice
-nostdin: specifies that this particular minimega should not accept input from the terminal; we will send it commands over a socket instead.
```

Given those flags, you might start minimega like this (as root):

```
minimega -degree 3 -context miniclass -nostdin &
```

Every machine does not need to connect to everyone else; usually you will not want to use more than 3 for the degree value.

Messages sent on the mesh will forward until they reach their destined host.

### Start minimega on the other nodes

Now that minimega is running on the head node, we can connect to it over the Unix socket:

```
$ /opt/minimega/bin/minimega -attach
```

This will give you a minimega prompt where we'll enter the command to launch on the other nodes:

```
deploy launch m[2-5]
```

The `deploy` command copies the current minimega binary to the nodes you specify using `scp`, then launches them with `ssh` using the same set of command line flags as the minimega instance that ran `deploy`. It occurs much like the script below:

```
ssh -o StrictHostKeyChecking=no m2 nohup /tmp/minimega_deploy_1497399602 \
-attach=false -base=/tmp/minimega -ccport=9002 -cgroup=/sys/fs/cgroup -cli=false \
-context=miniclass -degree=3 -e=false -filepath=/tmp/minimega/files -force=false \
-level=error -logfile= -msa=10 -nostdin=true -panic=false -pipe= -port=9000 -v=true \
-verbose=true -version=false > /dev/null 2>&1 &
```

Instead of using `deploy` this can be also done manually by running this.

```
for i in `seq 2 5`;
do
    scp /home/ubuntu/minimega/bin/minimega root@m$i:/tmp/minimega_deploy
    ssh root@m$i "nohup /tmp/minimega_deploy -degree 3 -context miniclass -nostdin > /dev/null 2>&1 &"
done
```

After a minute or so, the other instances of minimega should have located each other and created a communications mesh. You can check the status like this:

```
$ mesh status
host | size | degree | peers | context   | port
m1   | 5    | 3      | 3     | miniclass | 9000
```

To list each machines connected machines run `mesh list`

```
$ mesh list
```

`mesh status` shows general information about the communications mesh, including "mesh size", the number of nodes in the mesh. Because it shows a mesh size of 10, we know our entire 10-node cluster is in the mesh.

`mesh list` lists each mesh node and the nodes to which it is connected. Note that because we specified `-degree 3`, each node is connected to 3 others. Some nodes may be connected to more than 3 nodes, but each should have at least 3 connections.
