# Module 39 - Network Capture

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-39-network-capture/).

## Introduction

You can capture network traffic using minimega.

## PCAP

PCAPs contain a recording of every byte sent across the wire.

The syntax for PCAP capture is as follows:

```
capture
capture <pcap,>
capture <pcap,> bridge <bridge> <filename>
capture <pcap,> vm <vm id or name> <interface index> <filename>
capture <pcap,> <delete,> <id or all>
```

To capture PCAP on bridge `foo` to file `foo.pcap`:

```
capture pcap bridge foo foo.pcap
```

To capture PCAP on VM `foo` to file `foo.pcap`, using the 2nd interface on that VM:

```
capture pcap vm foo 0 foo.pcap
```

When run without arguments, `capture` prints all running captures. To stop a capture, use the `delete` command:

```
capture pcap delete <id>
```

To stop all captures of a particular kind, replace `<id>` with `all`. To stop all capture of all types, use `clear capture`.

You can clear the capture state using

```
clear capture pcap
```

## Netflow

Netflow summarizes the network traffic by IP address and quantity of traffic.

It can be written to a socket or file. It can be compressed with `gzip`. It can be saved as a binary file or ASCII.

```
capture
capture <netflow,>
capture <netflow,> <timeout,> [timeout]
capture <netflow,> <bridge,> <bridge>
capture <netflow,> <bridge,> <bridge> <file,> <filename>
capture <netflow,> <bridge,> <bridge> <file,> <filename> <raw,ascii> [gzip]
capture <netflow,> <bridge,> <bridge> <socket,> <tcp,udp> <hostname:port> <raw,ascii>
capture <netflow,> <delete,> <id or all>
```

For example, to capture netflow data on bridge `mega_bridge` to file in ASCII mode and with `gzip` compression:

```
capture netflow mega_bridge file foo.netflow ascii gzip
```

You can change the active flow timeout with:

```
capture netflow mega_bridge timeout <timeout>
```

With `<timeout>` in seconds.

You can stop netflow captures with d`elete`

```
capture netflow delete <id>
```

You can clear the capture state using

```
clear capture netflow
```

## Netflow Conversion

minimega netflow when saved as a binary format can be converted to ASCII using `nfcat`.

### Binary

```
# bin/nfcat foo.nf > foo.ascii
```

### Gzip

```
# bin/nfcat -gunzip foo.nf.gz > foo.ascii
```
