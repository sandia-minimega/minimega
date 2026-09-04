# Module 2 - Installing Dependencies

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-2-installing-dependencies/).

- Introduction
- [Version Requirements](#system-requirements-and-runtime-dependencies)
- Apt-get
- Installing Latest GoLang (Currently 1.17)

## Introduction

minimega [sic] compiles to a collection of binaries and needs no configuration files.

However, because minimega makes use of external programs, you'll need to have some things installed first before using it.

## System requirements and runtime dependencies

minimega is designed to be simple to deploy. It has only one runtime dependency, libpcap, which is included on almost all standard Linux distros.

minimega also has a number of external tools it executes. When you start minimega, it will check to see if each of the tools it may need are available in `$PATH`. Depending on your intended use case, you may not need every single external program.

If you plan to launch and maintain VMs, you'll need the following programs at a minimum:

- kvm - qemu-kvm with the kvm kernel module loaded (minimum version 1.6)
- ip - ip tool for manipulating devices
- ovs-vsctl - Open vSwitch switch control with daemon running and kernel module loaded (minimum version 1.11)
- ovs-ofctl - Open vSwitch openflow control with daemon running and kernel module loaded
- bird - IP packet routing daemon (minimum version 1.5.0)

To launch containers, the kernel must support OverlayFS, which was added in Linux 3.18.

We also recommend installing the following; they are not strictly necessary for basic VM use but are required for some more advanced operations:

- dhclient - dhcp client
- dnsmasq - DNS and DHCP server (minimum version 2.73)
- qemu-nbd/qemu-img - tool for interacting with qemu disk images (in the Debian package "qemu-tools")
- mkdosfs - used when creating router images
- taskset - set CPU affinity for VMs
- ntfs-3g - NTFS with write support for injecting files into NTFS images
- ssh/scp - used to deploy minimega to other nodes in a cluster
- tc - used by QoS API to set latency and bandwidth for VMs

The following debian packages should install most of the dependencies:

```
openvswitch-switch qemu-kvm qemu-utils dnsmasq ntfs-3g iproute2
```

## Apt-get

```
sudo bash
apt-get update
apt-get install libpcap-dev libreadline-dev qemu qemu-kvm openvswitch-switch dnsmasq bird build-essential tmux curl wget nano git unzip
```

## Installing Latest GoLang

Go is provided in a binary package which can be installed manually. Version 1.17 or greater is required.

```
curl -O https://go.dev/dl/go1.17.linux-amd64.tar.gz
tar xvf go1.17.linux-amd64.tar.gz
chown -R root:root ./go
mv go /usr/local
echo "export GOPATH=$HOME/work">>~/.bashrc
echo "export GOROOT=/usr/local/go">>~/.bashrc
echo "export PATH=$PATH:/usr/local/go/bin">>~/.bashrc
source ~/.bashrc
```

If using `apt-get` to install, verify that the repo provides a recent-enough version of the Go library.
