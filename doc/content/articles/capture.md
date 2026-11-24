# Capture and instrumentation

To learn from an experiment, you must see what happened in it. This page covers the built-in ways to observe traffic. You can write PCAP from a VM interface or a whole bridge. You can record netflow from a bridge to a file or a collector. You can mirror the traffic of a VM into a monitor VM that runs its own analysis tools. You can put a Zeek sensor on that mirror. When you need to know what these VMs actually sent, use these methods.

This page assumes that you know how VLANs, taps, and bridges fit together ([Host networking](networking.md)). To instrument the guests themselves, use [Command and control](cc.md). You run tools inside the guests and collect their output.

## Choosing a method

| You want | Use |
|---|---|
| Every byte one VM interface sends and receives, as PCAP | `capture pcap vm` |
| Everything on a bridge, as PCAP | `capture pcap bridge` |
| Who talked to whom and how much, compactly | `capture netflow` |
| A VM of your own to run tcpdump, Zeek, Suricata or Snort on live traffic | `tap mirror` |
| Logs and files produced inside a guest | [cc](cc.md) |

Open vSwitch takes PCAP and netflow on the host, so the guests cannot tell that they are watched. A mirror delivers the same frames to another VM. This is useful in these cases:

- The analysis tool is easier to run in a guest than on the host.
- The capture must survive as part of a saved experiment.

## PCAP

`capture pcap vm` captures one interface of one VM. `capture pcap bridge` captures everything that crosses a bridge, on every VLAN. A relative file name goes to the iomeshage files directory (`/tmp/minimega/files` by default) on the host that runs the VM or owns the bridge:

```minimega
minimega$ capture pcap vm web 0 web.pcap
minimega$ capture pcap bridge mega_bridge everything.pcap
minimega$ .columns bridge,type,interface,mode,compress,path capture
bridge      | type | interface | mode | compress | path
mega_bridge | pcap | web:0     |      |          | /tmp/minimega/files/web.pcap
mega_bridge | pcap |           |      |          | /tmp/minimega/files/everything.pcap
```

Two settings apply to captures that you start afterwards. They do not apply to captures that already run. `capture pcap snaplen <bytes>` limits how much of each packet minimega keeps. The default is 1600, which is enough for headers plus a bit of payload. To keep whole frames, set a larger value. `capture pcap filter <bpf>` applies a `pcap-filter` expression, for example `capture pcap filter "tcp port 80"`. Each command without an argument prints the current value. Until you set a snaplen, that query prints `0`. The value `0` means that minimega applies the 1600-byte default at the start of each capture.

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

minimega identifies each capture with a counter, and it never reuses a counter value. When you stop a capture, minimega always drops it from its list, even if stopping failed. Because of this, a stuck capture cannot block another capture or be confused with it. If the VM or bridge is on another host, pull the PCAP back to the head node with `file get`. See [File management](file.md).

## Netflow

Netflow records flows, not packets. For each conversation, it records the endpoints, ports, protocol, byte and packet counts, and timing. It is far smaller than PCAP and often all you need. minimega attaches a netflow exporter to a bridge. It writes the records to a file, to a TCP or UDP collector, or to several at once:

```minimega
minimega$ capture netflow mode ascii            # or raw (binary), the default
minimega$ capture netflow gzip true             # compress file output
minimega$ capture netflow bridge mega_bridge flows.nf
minimega$ capture netflow bridge mega_bridge udp collector.example.org:2055
minimega$ .columns bridge,type,interface,mode,compress,path capture
bridge      | type    | interface | mode  | compress | path
mega_bridge | netflow |           | ascii | true     | /tmp/minimega/files/flows.nf
mega_bridge | netflow |           | ascii | true     | udp:collector.example.org:2055
```

When minimega creates a writer, it reads `mode` and `gzip`. The listing shows them for each writer. `capture netflow timeout <seconds>` sets the active-flow timeout, 10 seconds by default. After this time, a long conversation is emitted as a record and started afresh. Open vSwitch keeps one netflow object for each bridge, so everything on that bridge shares the timeout. `capture netflow delete bridge <bridge>` or `... bridge all` stops the writers, and `clear capture netflow` stops them all.

Raw files are the compact form. The example above writes gzip-compressed text. Read it with `zcat flows.nf`. Convert a raw file to text on any machine with `nfcat`, which ships with minimega. For a raw file that you wrote with `gzip true`, add `-gunzip`:

```bash
$ nfcat raw.nf > flows.txt
$ nfcat -gunzip raw.nf.gz > flows.txt
```

## Captures and namespaces

`capture pcap vm` and the `capture pcap` and `capture netflow` settings follow the namespace. They run on every host in the namespace. `capture` lists the results from all of them. Captures on a bridge (`capture pcap bridge`, `capture netflow bridge ...`, their deletes, and `capture netflow timeout`) act only on the host that you are attached to. A bridge carries every namespace that uses it. Because of this, a bridge capture on a shared host contains the traffic of other people. When namespaces share hardware, capture from VM interfaces.

## Mirroring traffic to a monitor VM

A mirror tells Open vSwitch to copy every frame that enters or leaves one port to another port. Point the mirror at a VM that has tcpdump, Zeek, or an IDS installed. That VM sees the traffic of the VMs that you study without being on their network:

```text
tap mirror <vm> <interface index> <monitor vm> <interface index>
tap mirror <source tap> <destination tap> [bridge]
```

The first form addresses interfaces by VM name and position. The second form addresses them by tap name, as shown in the `tap` column of `vm info` or by `tap`. With the second form, a host tap can also be the source or the destination. Both ends must be on the same bridge. Because of this, both VMs must be on the same host. A destination can receive only one mirror, but a source can feed several destinations. To remove mirrors, use `clear tap mirror <destination tap>`, `clear tap mirror <vm> <index or all>`, or `clear tap mirror` for all of them. `clear tap` removes them together with the host taps.

```minimega title="mirror.mm"
--8<-- "articles/capture/mirror.mm"
```

[Download this example](capture/mirror.mm){ download="mirror.mm" }

The example launches the monitor on VLAN `0`, which is untagged. The monitor has a port on the bridge without joining `lan`. The mirror delivers the frames. When `tcpdump -i eth0` runs on the monitor, a ping from A to B shows up on both sides:

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

Use a mirror in these cases:

- The monitoring VM must be part of the saved experiment.
- You want the analysis to run in real time.
- The traffic volume is more than you want to write to the disk of the host.

## A Zeek sensor on the mirror

Replace the plain monitor with a VM that runs [Zeek](https://zeek.org/) on `eth0` and writes its logs to a disk that you can read afterwards. minimega ships `misc/vmbetter_configs/bro.conf`, a [vmbetter](vmbetter.md) configuration from before the project was renamed. It installs the `bro` package. The `init` in its overlay starts miniccc. If a disk is attached, `init` partitions `/dev/sda`, formats `/dev/sda1`, and mounts it on `/bro`. Then `init` starts `bro -i eth0` there.

To build a current sensor, do these steps:

1. Copy the configuration.
2. Remove the `bro` package. In `postbuild`, add the package repository of the Zeek project and install `zeek` from it. `debootstrap` cannot install the package, because it is not in the Debian archive.
3. Change the paths and the command in `init` to `zeek`.
4. Launch the sensor in place of the monitor, with a dedicated, non-snapshot disk so that the logs survive the VM:

```minimega
disk create qcow2 zeek.qc2 10G
vm config net 0
vm config kernel zeek.kernel
vm config initrd zeek.initrd
vm config snapshot false
vm config disks zeek.qc2
vm launch kvm monitor
vm start monitor
tap mirror A 0 monitor 0
```

Each sensor needs its own disk image. Without a disk image, the logs stay in memory, and a busy network will exhaust the memory. The `init` script of the sensor formats the disk at each boot, so copy the logs before you start the sensor again. When the experiment ends, mount the image on the host to collect the logs. Use the same NBD steps as for any qcow2 (see [Disk images](disk-images.md)):

```bash
# modprobe nbd max_part=10
# qemu-nbd -c /dev/nbd0 zeek.qc2
# mount /dev/nbd0p1 /mnt
# cp -a /mnt/. /path/to/results/
# umount /mnt
# qemu-nbd -d /dev/nbd0
```

Alternatively, omit the disk. Then pull the logs over [cc](cc.md) while the VM runs.

## See also

- [Host networking](networking.md): taps, bridges and VLANs.
- [Command and control](cc.md): running tools inside guests and collecting their output.
- [File management](file.md): getting capture files off remote hosts.
- [Namespaces](namespaces.md): how captures are scoped.
- Reference: [`capture`](../reference/minimega.md#capture), [`clear capture`](../reference/minimega.md#clear-capture), [`tap`](../reference/minimega.md#tap), [`clear tap`](../reference/minimega.md#clear-tap).
