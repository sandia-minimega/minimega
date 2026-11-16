# Troubleshooting

This page lists the problems that occur most often in minimega. For
each problem, it gives the checks that usually locate it. A walkthrough follows
for the most common class of problem: two VMs that cannot talk to each other.
The page assumes that you installed minimega and read
[Running minimega](running.md). The network section also assumes that you know
how to launch VMs and a [minirouter](router.md).

Two habits shorten most investigations. First, raise the log level and read
the log. `log level debug` at the prompt, or `-level debug -logfile <file>` at
startup, shows the QEMU, dnsmasq and Open vSwitch commands that minimega runs
and their output. Second, run `check` to make sure that the external tools
that minimega depends on are present and new enough.

## Common problems

### minimega will not compile

To build from source, you need the Go version that `go.mod` names (1.24 or
newer). You also need access to the Go module proxy, or a module cache that
already holds the dependencies (see
[Dependencies](developer/dependencies.md#building-offline)). Run
`scripts/build.bash`. The build also needs a C compiler and the libpcap
development headers (`libpcap-dev` on Debian and Ubuntu). minimega uses cgo in
`cmd/minimega/proc.go`, `cmd/minimega/disk.go` and `internal/bridge/qos.go`.
The gopacket pcap bindings link against libpcap. Missing C headers therefore
cause a build failure.

### minimega appears to already be running

At startup, minimega looks for the command socket `<base>/minimega`. If a
previous instance crashed, it leaves the socket behind. Start with `-recover`
to remove the socket and re-adopt VMs that the old instance was running. Start
with `-force` to remove the socket and start clean. If a live instance owns the
base directory, neither flag helps. Run `pgrep -a minimega` first to find out.
`-e` and `-attach` look for the same socket. When they fail with
`dial unix <base>/minimega: connect: ...` (no such file or connection refused),
the `-base` value usually does not match the base directory of the daemon.

### Something crashed

- Make sure that the disk is not full. `-level debug` on a busy host writes a
  lot of data. `log level error` at runtime turns it down.
- Read the log around the crash. When minimega stops on SIGINT or SIGTERM,
  `-panic` at startup makes it dump goroutine stacks. The `quit` command does
  not. A bug report needs those stacks.
- Restart with `-recover` to pick up VMs that are still running.
- If the host is in a bad state, use `nuke`. It kills every QEMU and container
  that minimega started, removes its taps and bridges, deletes the base
  directory, and then exits. It is a reset, not a routine command.
- Report reproducible crashes as issues on GitHub with the version, the
  command file and the log.

### Lingering taps or bridges

`nuke` removes the taps and bridges that minimega created. However, after a
hard crash, Open vSwitch can still hold ports that no longer have a process
behind them. Inspect and clean up with `ovs-vsctl`:

```bash
# ovs-vsctl show
# ovs-vsctl del-port mega_bridge mega_tap3
# ovs-vsctl del-br mega_bridge
```

Deleting a bridge deletes every port on it. `ip link delete mega_tap3`
removes a tap device that was already dropped from the bridge.

### Taps or VMs do not see each other

The cause is almost always a VLAN mismatch. The `vlan` column of `vm info`
shows the VLAN of each interface. `ovs-vsctl show` shows the `tag:` that Open
vSwitch gave the port. A VLAN alias (`vm config net LAN`) and a number
(`vm config net 100`) are different networks, unless the alias resolves to
that number. `vlans` lists the mapping. When traffic
crosses a GRE or VXLAN tunnel between hosts, the MTU also causes problems. Run
`ping -M do -s 1472 <ip>` from a guest. The result tells you whether
full-size frames get through.

### A VM stays in BUILDING or goes to ERROR

`vm info` shows the state. `vm flush` discards VMs in `ERROR`, so you can
relaunch them. The log has the reason. The most common reason is that QEMU
exits before minimega can reach its control socket:

```text
ERROR unable to connect to qmp socket: dial unix /tmp/minimega/0/qmp: connect:
connection refused. qemu output: Could not access KVM kernel module: No such
file or directory
failed to initialize KVM: No such file or directory
```

Everything after `qemu output:` is QEMU's own message and names the problem.
Here the `kvm` module is not loaded. Other frequent messages report these
problems:

- A disk image path that does not exist. Make sure that `-filepath` and the
  `disks` column are correct.
- A `-cpu` or `-machine` option that the installed QEMU does not support
  (`vm config qemu-append`).

### Unable to load kvm_intel or kvm_amd

```text
# modprobe kvm_intel
modprobe: ERROR: could not insert 'kvm_intel': Operation not supported
```

Hardware virtualization is disabled or unavailable. On bare metal, enable VT-x
or AMD-V in the firmware. Inside a VM, you need nested virtualization enabled
on the outer hypervisor. `dmesg | grep -i kvm` reports what the kernel found.
`ls -l /dev/kvm` must show that the file exists and that the user that runs
minimega can write to it.

### VMs are slow

- `vm top` and `top` on the host tell you whether you launched more
  VMs than the host has cores or memory for.
- `dmesg | grep -i kvm` shows KVM warnings such as unsupported CPU features
  that force emulation.
- `optimize ksm true` turns on kernel same-page merging. This reclaims memory
  across VMs that booted from the same image. `optimize hugepages <path>` and
  `optimize affinity true` help CPU-bound experiments. `optimize` with no
  arguments shows the current configuration.
- Before you blame the VMs, make sure that the disks (`smartctl`, `iostat`) and
  the network (`nload`) are not saturated.

### dnsmasq starts and immediately exits

minimega runs `dnsmasq` bound to the address of the tap that you gave it
(`--bind-interfaces --listen-address <ip>`). If another DNS server already owns
port 53 on that address or on all addresses, `dnsmasq` fails to start. The
other server is most often a system-wide `dnsmasq` or the one that
NetworkManager starts. Find the owner with `ss -ulpn 'sport = :53'`. The stub
of systemd-resolved listens only on `127.0.0.53` and usually coexists.
`dnsmasq start` still succeeds. After dnsmasq exits, minimega logs its error
message at the default error level.

### The mesh does not form

Every node must run the same minimega version. At connection time, minimega
logs a peer with a different build as `remote node version mismatch`.
`mesh status` shows the mesh `size`, your `degree`, the `peers` that you are
connected to, and the `context` and `port` in use. All nodes must share the
context and port. For discovery to work, all nodes must also be in the same
broadcast domain. Where broadcast is not available, `mesh dial <hostname>`
connects to a peer directly. Make sure that port 9000 (or your `-port`) is open
between hosts. See
[Cluster setup](cluster.md).

### VNC from the web does not connect

The web console goes through the VNC proxy of minimega for the VM (the
`vnc_port` column of `vm info`). If someone attached a VNC viewer to the same
VM, the session is shared. Some viewers do not work with a shared session.
Disconnect the other client. Also make sure that you started miniweb with the
same `-base` as minimega. The VM list comes from the daemon that miniweb can
reach.

### miniccc clients never appear

- The agent and minimega must come from the same release. At handshake,
  minimega logs a version mismatch as `mismatched miniccc version on <hostname>`
  and refuses very old agents with `miniccc client too old`. Rebuild the image
  with the current miniccc.
- `vm config backchannel` must be `true` (the default) for the serial channel
  to exist. A `vm config virtio-ports` port named `cc` collides with it.
- TCP clients connect only after `cc listen <port>` in the namespace. They must
  present the UUID of a VM in that namespace.
- Look at the log of the agent inside the guest (`/miniccc.log` in the stock
  images) and at `cc clients`. The `cc_active` column of `vm info` shows
  whether minimega received a message from the agent in the last 30 seconds.

See [Command and control](cc.md) for the details.

### Open vSwitch cheat sheet

minimega controls Open vSwitch through `ovs-vsctl`. These read-only commands
show what it built:

```bash
# ovs-vsctl show                      # bridges, ports and their VLAN tags
# ovs-vsctl list Bridge               # database records, also Port and Interface
# ovs-ofctl show mega_bridge          # OpenFlow port numbers and link state
# ovs-ofctl dump-flows mega_bridge    # flows, useful when qos or mirrors are in play
# ovs-appctl fdb/show mega_bridge     # learned MAC addresses per port and VLAN
```

### Reaching into and out of an experiment

A tap with an address puts the host on an experiment VLAN:

```minimega
minimega$ tap create 100 ip 10.0.0.1/24
```

From then on, SSH from a guest to `10.0.0.1` reaches the host. Standard SSH
port forwarding gives a guest a controlled path out. Run this command on the
guest:

```bash
$ ssh -L 1234:proxy.example.com:80 user@10.0.0.1
```

A browser in the guest that uses `127.0.0.1:1234` as its proxy reaches whatever
the host can reach. dnsmasq binds to the tap address. The `cc listen` TCP port
is open on every interface of the host. Be careful. A tap joins the network
stack of the host to the experiment.

## Network troubleshooting

When two VMs cannot connect, work outward from the closest hop. This section
uses a three-VM environment to show the steps:

```minimega
# A --- router --- B

# a minirouter between two subnets
clear vm config
vm config kernel $images/minirouter.kernel
vm config initrd $images/minirouter.initrd
vm config memory 3072
vm config net A B
vm launch kvm router
vm start router

router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.1 range 10.0.0.2 10.0.0.2
router router route ospf 0 0
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.1 range 10.0.1.2 10.0.1.2
router router route ospf 0 1
router router commit

# one client in each subnet
clear vm config
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config memory 3072
vm config net A
vm launch kvm A
vm config net B
vm launch kvm B
vm start all
```

The goal is for A to reach B and back.

### Ping

Ping first, because it separates the two failure classes. A working ping from
A to the router's interface on A's subnet:

```text
$ ping -c 3 10.0.0.1
PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
64 bytes from 10.0.0.1: icmp_seq=1 ttl=64 time=0.079 ms
64 bytes from 10.0.0.1: icmp_seq=2 ttl=64 time=0.085 ms
64 bytes from 10.0.0.1: icmp_seq=3 ttl=64 time=0.085 ms

--- 10.0.0.1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2029ms
rtt min/avg/max/mdev = 0.079/0.083/0.085/0.003 ms
```

This is a ping that cannot get there:

```text
$ ping -c 3 10.0.2.1
PING 10.0.2.1 (10.0.2.1) 56(84) bytes of data.
From 10.0.0.1 icmp_seq=1 Destination Host Unreachable
From 10.0.0.1 icmp_seq=2 Destination Host Unreachable
From 10.0.0.1 icmp_seq=3 Destination Host Unreachable

--- 10.0.2.1 ping statistics ---
3 packets transmitted, 0 received, +3 errors, 100% packet loss, time 2043ms
```

If A can ping B, connectivity is fine and the problem is at a higher layer. If
not, ping the near interface of the router, `10.0.0.1`. If that works, ping
its far interface, `10.0.1.1`. A failure there is a routing problem. If A
cannot reach `10.0.0.1`, it is a subnet problem.

### Subnet problems

A can fail to reach a router interface on its own subnet for these reasons:

- Mistyped VLANs. Both interfaces must be on the same VLAN for layer 2
  connectivity. Compare the `vlan` column of `vm info` for both VMs.
- Duplicate IPs. Two VMs with the same address on one subnet conflict.
- Mismatched subnets. Both addresses must fall in the same network with the
  same mask. If they do not, the guests will not consider each other local. To
  make sure that they match, use `ipcalc 10.0.0.2/24` on the host or calculate
  the network by hand.

`ip neighbor` in a guest shows the layer 2 neighbors that it resolved:

```text
$ ip neighbor
10.0.0.1 dev eth0 lladdr 00:19:7e:7d:2b:d2 REACHABLE
```

An empty table, or an entry marked `FAILED`, means that ARP does not get
through and the problem is at layer 2. `tcpdump -i eth0` on the target shows
whether packets arrive at all. If you see the pings but no replies, suspect a
subnet mismatch or a guest firewall.

### Routing problems

- Make sure that every address and mask on every router interface is correct.
  A typo in one mask hides a whole subnet.
- On a minirouter, ask bird what it knows. bird runs with its control socket
  at `/bird.sock`. Use cc to reach it:

    ```minimega
    minimega$ cc filter name=router
    minimega$ cc exec birdc -s /bird.sock show protocols all
    minimega$ cc exec birdc -s /bird.sock show route
    minimega$ cc commands
    minimega$ cc responses <id>
    ```

    `cc exec` does not print the output of bird. Find the ID of each command
    with `cc commands`, and then `cc responses <id>` prints the output.

    The `router` VM in this example has OSPF configured, so the OSPF protocol
    is `up` and the neighboring subnet appears in the route table. minirouter
    uses bird 1.x. The
    [bird 1.6 documentation](https://bird.network.cz/?get_doc&v=16&f=bird.html)
    explains the output.
- `traceroute` from a guest shows where packets stop.
- A guest needs a default route or a specific route back. A one-way ping
  usually means that the reply has no route back.

## See also

- [Running minimega](running.md) for startup flags, logging and recovery.
- [Host networking](networking.md) for taps, bridges, VLANs and dnsmasq.
- [Routing with minirouter](router.md).
- [Cluster setup](cluster.md) for the mesh.
- [Command and control](cc.md) for miniccc.
- Reference: [`check`](../reference/minimega.md#check),
  [`nuke`](../reference/minimega.md#nuke),
  [`optimize`](../reference/minimega.md#optimize),
  [`mesh status`](../reference/minimega.md#mesh-status),
  [`log level`](../reference/minimega.md#log-level).
