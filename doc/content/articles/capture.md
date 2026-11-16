# Capture and instrumentation

An experiment is only useful if you can see what happened in it. This page covers the built-in ways to observe traffic: writing PCAP from a VM interface or a whole bridge, recording netflow from a bridge to a file or a collector, mirroring a VM's traffic into a monitor VM that runs its own analysis tools, and putting a Zeek sensor on that mirror. You need it whenever the question is "what did these VMs actually send?".

It assumes you know how VLANs, taps and bridges fit together ([Host networking](networking.md)). Instrumenting the guests themselves, by running tools inside them and collecting their output, is the job of [Command and control](cc.md).

## Choosing a method

| You want | Use |
|---|---|
| Every byte one VM interface sends and receives, as PCAP | `capture pcap vm` |
| Everything on a bridge, as PCAP | `capture pcap bridge` |
| Who talked to whom and how much, compactly | `capture netflow` |
| A VM of your own to run tcpdump, Zeek, Suricata or Snort on live traffic | `tap mirror` |
| Logs and files produced inside a guest | [cc](cc.md) |

PCAP and netflow are taken on the host by Open vSwitch, so the guests cannot tell they are being watched. A mirror delivers the same frames to another VM, which is what you want when the analysis tool is easier to run in a guest than on the host, or when the capture should survive as part of a saved experiment.

## PCAP

`capture pcap vm` captures one interface of one VM; `capture pcap bridge` captures everything crossing a bridge, every VLAN included. A relative file name lands in the iomeshage files directory (`/tmp/minimega/files` by default) of the host that runs the VM or owns the bridge:

```minimega
minimega$ capture pcap vm web 0 web.pcap
minimega$ capture pcap bridge mega_bridge everything.pcap
minimega$ capture
bridge      | type | interface | mode | compress | path
mega_bridge | pcap | web:0     |      |          | /tmp/minimega/files/web.pcap
mega_bridge | pcap |           |      |          | /tmp/minimega/files/everything.pcap
```

Two settings apply to captures started afterwards, not to ones already running. `capture pcap snaplen <bytes>` limits how much of each packet is kept, 1600 by default, which is enough for headers plus a bit of payload; set it larger to keep whole frames. `capture pcap filter <bpf>` applies a `pcap-filter` expression, for example `capture pcap filter "tcp port 80"`. Either command without an argument prints the current value; until you set a snaplen that query prints `0`, meaning the 1600-byte default is applied when a capture starts.

Stop captures by what they capture:

```minimega
capture pcap delete vm web         # every interface of web
capture pcap delete vm web 0       # just interface 0
capture pcap delete vm all
capture pcap delete bridge mega_bridge
capture pcap delete bridge all
clear capture pcap                 # all pcap captures in the namespace
clear capture                      # all captures of every kind
```

minimega keys captures with a counter that is never reused and always drops a capture from its list when you stop it, even if stopping failed, so one stuck capture can never block or be confused with another. When the VM or bridge is on another host, use `file get` (see [File management](file.md)) to pull the PCAP back to the head node.

## Netflow

Netflow records flows rather than packets: for each conversation, the endpoints, ports, protocol, byte and packet counts and timing. It is far smaller than PCAP and often all you need. minimega attaches a netflow exporter to a bridge and writes the records to a file, to a TCP or UDP collector, or to several at once:

```minimega
capture netflow mode ascii            # or raw (binary), the default
capture netflow gzip true             # compress file output
capture netflow bridge mega_bridge flows.nf
capture netflow bridge mega_bridge udp collector.example.org:2055
minimega$ capture
bridge      | type    | interface | mode  | compress | path
mega_bridge | netflow |           | ascii | true     | /tmp/minimega/files/flows.nf
mega_bridge | netflow |           | ascii | false    | udp:collector.example.org:2055
```

`mode` and `gzip` are read when a writer is created and shown per writer in the listing. `capture netflow timeout <seconds>` sets the active-flow timeout, 10 seconds by default, after which a long conversation is emitted as a record and started afresh; Open vSwitch keeps one netflow object per bridge, so the timeout is shared by everything on that bridge. `capture netflow delete bridge <bridge>` or `... bridge all` stops the writers, and `clear capture netflow` stops them all.

Raw files are the compact form. Convert them to text on any machine with `nfcat`, which ships with minimega:

```bash
$ nfcat flows.nf > flows.txt
$ nfcat -gunzip flows.nf.gz > flows.txt
```

## Captures and namespaces

`capture pcap vm` and the `capture pcap` and `capture netflow` settings follow the namespace: they run on every host in the namespace and `capture` lists the results from all of them. Captures on a bridge (`capture pcap bridge`, `capture netflow bridge ...`, their deletes, and `capture netflow timeout`) only act on the host you are attached to, and a bridge carries every namespace that uses it, so a bridge capture on a shared host contains other people's traffic. Capture from VM interfaces when namespaces share hardware.

## Mirroring traffic to a monitor VM

A mirror tells Open vSwitch to copy every frame that enters or leaves one port to another port. Point it at a VM that has tcpdump, Zeek or an IDS installed and that VM sees the traffic of the VMs you are studying without being on their network:

```minimega
tap mirror <vm> <interface index> <monitor vm> <interface index>
tap mirror <source tap> <destination tap> [bridge]
```

The first form addresses interfaces by VM name and position, the second by tap name as shown in the `tap` column of `vm info` or by `tap`, so a host tap can be the source or the destination as well. Both ends must be on the same bridge, which also means both VMs must be on the same host; a destination can only receive one mirror, but a source can feed several destinations. Mirrors are removed with `clear tap mirror <destination tap>`, `clear tap mirror <vm> <index or all>`, or `clear tap mirror` for all of them; `clear tap` removes them along with the host taps.

```minimega title="mirror.mm"
--8<-- "articles/capture/mirror.mm"
```

[Download this example](capture/mirror.mm){ download="mirror.mm" }

The monitor is launched on VLAN `0`, that is untagged, so it has a port on the bridge without joining `lan`; the mirror is what delivers the frames. With `tcpdump -i eth0` running on the monitor, a ping from A to B shows up on both sides:

```text
root@A:/# ping -c 3 10.0.0.2
PING 10.0.0.2 (10.0.0.2) 56(84) bytes of data.
64 bytes from 10.0.0.2: icmp_seq=1 ttl=64 time=0.885 ms
64 bytes from 10.0.0.2: icmp_seq=2 ttl=64 time=0.595 ms
64 bytes from 10.0.0.2: icmp_seq=3 ttl=64 time=0.594 ms
```

```text
root@monitor:/# tcpdump -i eth0
listening on eth0, link-type EN10MB (Ethernet), capture size 262144 bytes
23:52:55.738688 IP 10.0.0.1 > 10.0.0.2: ICMP echo request, id 111, seq 1, length 64
23:52:55.739049 IP 10.0.0.2 > 10.0.0.1: ICMP echo reply, id 111, seq 1, length 64
23:52:56.740360 IP 10.0.0.1 > 10.0.0.2: ICMP echo request, id 111, seq 2, length 64
23:52:56.740542 IP 10.0.0.2 > 10.0.0.1: ICMP echo reply, id 111, seq 2, length 64
```

Mirrors are the right tool when the monitoring VM must be part of the saved experiment, when you want the analysis to run in real time, or when the traffic volume is more than you want to write to the host's disk.

## A Zeek sensor on the mirror

Replace the plain monitor with a VM that runs [Zeek](https://zeek.org/) on `eth0` and writes its logs to a disk you can read afterwards. minimega ships `misc/vmbetter_configs/bro.conf`, a [vmbetter](vmbetter.md) configuration from before the project was renamed: it installs the `bro` package and its overlay's `init` starts miniccc, formats and mounts `/dev/sda` on `/bro` if a disk is attached, and starts `bro -i eth0` there. To build a current sensor, copy it, change the package to `zeek` (from the Zeek project's package repository, since it is not in the Debian archive), and change the paths and command in the init to `zeek`. Then launch it in place of the monitor with a dedicated, non-snapshot disk so the logs survive the VM:

```minimega
disk create qcow2 zeek.qc2 10G
vm config networks 0
vm config kernel zeek.kernel
vm config initrd zeek.initrd
vm config snapshot false
vm config disks zeek.qc2
vm launch kvm monitor
vm start monitor
tap mirror A 0 monitor 0
```

Each sensor needs its own disk image; without one the logs stay in memory and a busy network will exhaust it. When the experiment ends, mount the image on the host to collect the logs, using the same NBD steps as any qcow2 (see [Disk images](disk-images.md)):

```bash
# modprobe nbd
# qemu-nbd -c /dev/nbd0 zeek.qc2
# mount /dev/nbd0 /mnt
# cp -a /mnt/. /path/to/results/
# umount /mnt
# qemu-nbd -d /dev/nbd0
```

Or leave the disk out of it and pull the logs while the VM runs, over [cc](cc.md).

## See also

- [Host networking](networking.md): taps, bridges and VLANs.
- [Command and control](cc.md): running tools inside guests and collecting their output.
- [File management](file.md): getting capture files off remote hosts.
- [Namespaces](namespaces.md): how captures are scoped.
- Reference: [`capture`](../reference/minimega.md#capture), [`clear capture`](../reference/minimega.md#clear-capture), [`tap`](../reference/minimega.md#tap), [`clear tap`](../reference/minimega.md#clear-tap).
