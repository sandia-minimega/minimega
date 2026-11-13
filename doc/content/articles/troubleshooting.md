# Troubleshooting

This page collects the problems that come up most often when running
minimega, with the checks that usually locate them, followed by a walkthrough
for the commonest class of all: two VMs that cannot talk to each other. It
assumes you have minimega installed and have read
[Running minimega](running.md); the network half assumes you know how to
launch VMs and a [minirouter](router.md).

Two habits shorten most investigations. First, raise the log level and read
the log: `log level debug` at the prompt, or `-level debug -logfile <file>`
at startup, shows the QEMU, dnsmasq and Open vSwitch commands minimega runs
and their output. Second, run `check` to confirm the external tools minimega
depends on are present and new enough.

## Common problems

### minimega will not compile

Building from source needs the Go version named in `go.mod` (1.24 or newer)
and vendored dependencies. Run `scripts/build.bash`, which sources
`scripts/env.bash` to set `GOFLAGS=-mod=vendor`. If you build with `go build`
directly, export that variable yourself. The build also needs a C compiler
and the libpcap development headers (`libpcap-dev` on Debian and Ubuntu):
minimega uses cgo in `cmd/minimega/proc.go`, `cmd/minimega/disk.go` and
`internal/bridge/qos.go`, and the vendored gopacket pcap bindings link
against libpcap, so missing C headers are a genuine build failure.

### minimega appears to already be running

Startup checks for the command socket `<base>/minimega`. If a previous
instance crashed the socket is left behind. Start with `-recover` to remove it
and re-adopt VMs the old instance was running, or with `-force` to remove it
and start clean. Neither helps if a live instance owns the base directory;
check `pgrep -a minimega` first. The same socket is what `-e` and `-attach`
look for, so "no running instance" from those usually means a `-base` that
does not match the daemon's.

### Something crashed

- Check whether the disk is full. `-level debug` on a busy host writes a great
  deal; `log level error` at runtime turns it down.
- Read the log around the crash. `-panic` at startup makes minimega dump
  goroutine stacks when it exits, which is what a bug report needs.
- Restart with `-recover` to pick up VMs that are still running.
- If the host is in a bad state, `nuke` kills every QEMU and container
  minimega started, removes its taps and bridges and deletes the base
  directory, then exits. It is the reset button, not a routine command.
- Report reproducible crashes as issues on GitHub with the version, the
  command file and the log.

### Lingering taps or bridges

`nuke` removes the taps and bridges minimega created, but after a hard crash
Open vSwitch can still hold ports that no longer have a process behind them.
Inspect and clean up with `ovs-vsctl`:

```bash
# ovs-vsctl show
# ovs-vsctl del-port mega_bridge mega_tap3
# ovs-vsctl del-br mega_bridge
```

Deleting a bridge deletes every port on it. `ip link delete mega_tap3`
removes a tap device that has already been dropped from the bridge.

### Taps or VMs do not see each other

Almost always a VLAN mismatch. The `vlan` column of `vm info` shows what each
interface is on, and `ovs-vsctl show` shows the `tag:` Open vSwitch gave the
port. A VLAN alias (`vm config networks LAN`) and a number (`vm config networks 100`)
are different networks unless the alias resolves to that number; `vlans`
lists the mapping. The MTU also bites when traffic crosses a GRE or VXLAN
tunnel between hosts: `ping -M do -s 1472 <ip>` from a guest tells you whether
full-size frames get through.

### A VM stays in BUILDING or goes to ERROR

`vm info` shows the state; `vm flush` discards VMs in `ERROR` so you can
relaunch. The log has the reason, and the most common one is QEMU exiting
before minimega could reach its control socket:

```text
ERROR unable to connect to qmp socket: dial unix /tmp/minimega/0/qmp: connect:
connection refused. qemu output: Could not access KVM kernel module: No such
file or directory
failed to initialize KVM: No such file or directory
```

Everything after `qemu output:` is QEMU's own message and names the problem:
here the `kvm` module is not loaded. Other frequent messages are a disk image
path that does not exist (check `-filepath` and the `disks` column) and a
`-cpu` or `-machine` option the installed QEMU does not support
(`vm config qemu-append`).

### Unable to load kvm_intel or kvm_amd

```text
# modprobe kvm_intel
modprobe: ERROR: could not insert 'kvm_intel': Operation not supported
```

Hardware virtualization is disabled, or unavailable. On bare metal enable
VT-x or AMD-V in the firmware. Inside a VM you need nested virtualization
enabled on the outer hypervisor. `dmesg | grep -i kvm` reports what the
kernel found, and `ls -l /dev/kvm` must exist and be writable by the user
running minimega.

### VMs are slow

- `vm top` and `top` on the host tell you whether you simply launched more
  VMs than the host has cores or memory for.
- `dmesg | grep -i kvm` shows KVM warnings such as unsupported CPU features
  that force emulation.
- `optimize ksm true` turns on kernel same-page merging, which reclaims
  memory across VMs booted from the same image; `optimize hugepages <path>`
  and `optimize affinity true` help CPU-bound experiments. `optimize` with no
  arguments shows the current settings.
- Check the disks (`smartctl`, `iostat`) and the network (`nload`) for
  saturation before blaming the VMs.

### dnsmasq starts and immediately exits

minimega runs `dnsmasq` bound to the address of the tap you gave it
(`--bind-interfaces --listen-address <ip>`). It fails to start when another
DNS server already owns port 53 on that address or on all addresses, most
often a system-wide `dnsmasq` or the one NetworkManager spawns. Find the
owner with `ss -ulpn 'sport = :53'`; systemd-resolved's stub listens only on
`127.0.0.53` and usually coexists. With `log level debug` set, dnsmasq's own
error message appears in minimega's log.

### The mesh does not form

Every node must run the same minimega version; a peer with a different build
is logged as `remote node version mismatch` at connection time. `mesh status`
shows the mesh `size`, your `degree`, the `peers` you are connected to and
the `context` and `port` in use; all nodes must share the context and port,
and be in the same broadcast domain for discovery to work. Where broadcast is
not available, `mesh dial <hostname>` connects to a peer directly. Check that
port 9000 (or your `-port`) is open between hosts. See
[Cluster setup](cluster.md).

### VNC from the web does not connect

The web console goes through minimega's VNC proxy for the VM (the `vnc_port`
column of `vm info`). If someone has a VNC viewer attached to the same VM the
session is shared, and some viewers do not cope with that; disconnect the
other client. Also confirm miniweb was started with the same `-base` as
minimega; the VM list comes from the daemon it can reach.

### miniccc clients never appear

- The agent and minimega must come from the same release. A version mismatch
  is logged at handshake as `mismatched miniccc version on <hostname>`, and
  very old agents are refused with `miniccc client too old`. Rebuild the image
  with the current miniccc.
- `vm config backchannel` must be `true` (the default) for the serial channel
  to exist, and a `vm config virtio-ports` port named `cc` collides with it.
- TCP clients only connect after `cc listen <port>` in the namespace, and
  must present the UUID of a VM in that namespace.
- Look at the agent's log inside the guest (`/miniccc.log` in the stock
  images) and at `cc clients`; the `cc_active` column of `vm info` shows
  whether minimega has heard from the agent in the last 30 seconds.

See [Command and control](cc.md) for the details.

### Open vSwitch cheat sheet

minimega drives Open vSwitch through `ovs-vsctl`; these read-only commands
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

From then on SSH from a guest to `10.0.0.1` reaches the host, and standard
SSH port forwarding gives a guest a controlled path out. Run on the guest:

```bash
$ ssh -L 1234:proxy.example.com:80 user@10.0.0.1
```

and a browser in the guest configured to use `127.0.0.1:1234` as its proxy
reaches whatever the host can. The same tap is what the `cc` TCP transport
and `dnsmasq` bind to. Be deliberate about it: a tap joins the host's network
stack to the experiment.

## Network troubleshooting

When two VMs cannot connect, work outward from the closest hop. This section
uses a three-VM environment so the steps are concrete:

```minimega
# A --- router --- B

# a minirouter between two subnets
clear vm config
vm config kernel $images/minirouter.kernel
vm config initrd $images/minirouter.initrd
vm config networks A B
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
vm config networks A
vm launch kvm A
vm config networks B
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

And one that cannot get there:

```text
$ ping -c 3 10.0.2.1
PING 10.0.2.1 (10.0.2.1) 56(84) bytes of data.
From 10.0.0.1 icmp_seq=1 Destination Host Unreachable
From 10.0.0.1 icmp_seq=2 Destination Host Unreachable
From 10.0.0.1 icmp_seq=3 Destination Host Unreachable

--- 10.0.2.1 ping statistics ---
3 packets transmitted, 0 received, +3 errors, 100% packet loss, time 2043ms
```

If A can ping B, connectivity is fine and the problem is higher up. If not,
ping the router's near interface, `10.0.0.1`. If that works, ping its far
interface, `10.0.1.1`; failure there is a routing problem. If A cannot even
reach `10.0.0.1`, it is a subnet problem.

### Subnet problems

Reasons A might not reach a router interface on its own subnet:

- Mistyped VLANs. Both interfaces must be on the same VLAN for layer 2
  connectivity; compare the `vlan` column of `vm info` for both VMs.
- Duplicate IPs. Two VMs with the same address on one subnet fight over it.
- Mismatched subnets. Both addresses must fall in the same network with the
  same mask, or the guests will not consider each other local. Check with
  `ipcalc 10.0.0.2/24` on the host or by hand.

`ip neighbor` in a guest shows the layer 2 neighbours it has resolved:

```text
$ ip neighbor
10.0.0.1 dev eth0 lladdr 00:19:7e:7d:2b:d2 REACHABLE
```

An empty table, or an entry marked `FAILED`, means ARP is not getting through
and the problem is at layer 2. `tcpdump -i eth0` on the target shows whether
packets arrive at all: if you see the pings but no replies, suspect a subnet
mismatch or a guest firewall.

### Routing problems

- Check every address and mask on every router interface; a typo in one
  mask hides a whole subnet.
- On a minirouter, ask bird what it knows. bird runs with its control socket
  at `/bird.sock`, so through cc:

    ```minimega
    minimega$ cc filter name=router
    minimega$ cc exec birdc -s /bird.sock show protocols all
    minimega$ cc exec birdc -s /bird.sock show route
    ```

    The `router` VM in this example was configured with OSPF, so the OSPF
    protocol should be `up` and the neighbouring subnet should appear in the
    route table. minirouter uses bird 1.x; the
    [bird 1.6 documentation](https://bird.network.cz/?get_doc&v=16&f=bird.html)
    explains the output.
- `traceroute` from a guest shows where packets stop.
- Remember that a guest needs a default route or a specific route back; a
  one-way ping usually means the reply has nowhere to go.

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
