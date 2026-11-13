# powerbot

powerbot turns cluster nodes on and off. It speaks IPMI through `ipmitool`,
drives networked power distribution units (PDUs) over their telnet interfaces,
and understands minimega's node range syntax, so `powerbot cycle ccc[1-10]`
reboots ten netbooted nodes at once. Read this page when you are setting up a
cluster and want to reboot nodes without walking to the rack; it belongs with
[Cluster setup](cluster.md).

powerbot was written for the hardware used at Sandia, so it supports Tripp
Lite and Server Tech PDUs plus any node with an IPMI controller. Adding a PDU
type is a small Go change; see [Adding a PDU type](#adding-a-pdu-type).

## Usage

```text
powerbot on <nodelist>
powerbot off <nodelist>
powerbot cycle <nodelist>
powerbot status <nodelist>   # not implemented for Tripp Lite outlets
powerbot temp <nodelist>     # IPMI only: temperature sensors
powerbot info <nodelist>     # IPMI only: full sensor list
powerbot                     # status of every configured node
```

Node lists use the range syntax from the rest of minimega: `ccc1`,
`ccc[1-10]`, or `ccc[1-3,7,10-12]`. The prefix must match the `prefix` line of
the configuration file.

For each node, powerbot tries IPMI first when the node has IPMI columns in the
configuration, and falls back to the node's PDU outlet if IPMI is not
configured or the `ipmitool` call fails. `temp` and `info` only work over IPMI;
nodes without it are skipped with a message.

| Flag | Default | Purpose |
|---|---|---|
| `-config` | `/etc/powerbot.conf` | Configuration file. |
| `-pdu` | `false` | Skip IPMI and use the PDU for every node. |
| `-level`, `-logfile`, `-v` | `error`, none, `true` | Logging. powerbot also logs to syslog under the tag `powerbot`. |

## Installation

The packages and the Docker image install the binary as
`/opt/minimega/bin/powerbot`; a source build puts it in `bin/powerbot`. Copy it
somewhere on your path if you like. IPMI needs `ipmitool` installed on the
machine that runs powerbot.

Write the configuration to `/etc/powerbot.conf` or point `-config` at it. Two
examples ship in the repository: `cmd/powerbot/powerbot.conf` (Tripp Lite plus
IPMI) and `cmd/powerbot/servertech.conf`. The first is reproduced here:

```text
# Prefix for your nodes. Sorry, need this for ranges
prefix	ccc

# device specification
# device	<name>	<type>	<host>	<port>	<username>	<password>
device	p1	tripplite	pdu 5214	localadmin	localadmin

# IPMI: path to ipmitool - uncomment if a path needs to be specified
# ipmi <path/to/ipmitool>

#  node listing  ##     PDU     ##             IPMI                ##
# node <nodename> <pdu> <outlet> [<IP Address> <Username> <Password>]
#                ##             ##                                 ##
node ccc1  p1 4
node ccc2  p1 5
node ccc3  p1 6
node ccc4  p1 7
node ccc5  p1 9
node ccc6  p1 10 172.17.1.6 ipmi1 miniMEGA
node ccc7  p1 11 172.17.1.7 ipmi1 miniMEGA
node ccc8  p1 12 172.17.1.8 ipmi1 miniMEGA
node ccc9  p1 13
node ccc10 p1 14
node ccc11 p1 15
node ccc12 p1 17
node ccc13 p1 18
node ccc14 p1 19
node ccc15 NA NA 172.17.1.15 ipmi1 miniMEGA
node ccc16 NA NA 172.17.1.16 ipmi1 miniMEGA
node ccc17 NA NA 172.17.1.17 ipmi1 miniMEGA
node ccc18 NA NA 172.17.1.18 ipmi1 miniMEGA
node ccc19 NA NA 172.17.1.19 ipmi1 miniMEGA
```

## Configuration file

Each line starts with a keyword; fields are separated by whitespace and blank
lines and `#` comments are ignored.

- `prefix <string>`: the part of every hostname before the number. With
  `prefix ccc` the nodes are `ccc1`, `ccc2`, and so on. One prefix per file;
  to control a second cluster, write a second file and pass `-config`.
- `device <name> <type> <host> <port> <username> <password>`: one PDU. The
  name is internal to powerbot. The type is `tripplite` or `servertech` (see
  [Supported PDUs](#supported-pdus)). The host may be a hostname or an IP
  address, and the port is the PDU's telnet CLI port. A `device` line with the
  wrong number of fields is ignored.
- `ipmi <path>`: the `ipmitool` binary to run. Without this line powerbot
  looks for `ipmitool` on the path.
- `node <name> <pdu> <outlet> [<ip> <username> <password>]`: one physical
  machine. The first three fields map the node to an outlet on a `device`;
  outlet names follow the PDU's convention (Tripp Lite numbers them, Server
  Tech uses names such as `.AA1`). The optional last three fields are the
  node's IPMI address and credentials. For a node that only has IPMI, put
  `NA NA` in the PDU and outlet columns, as `ccc15` to `ccc19` do above.
- `loglevel <level>`: also send powerbot's log at that level to syslog.

## IPMI

For nodes with IPMI columns, powerbot runs
`ipmitool -I lanplus -H <ip> -U <username> -P <password> <command>` with
`chassis power on`, `chassis power off`, `chassis power cycle`, or
`chassis power status`, and for `temp` and `info` with `sdr type Temperature`
and `sdr elist full`. A node whose IPMI call fails is added to the PDU pass if
it has an outlet. Use `-pdu` to bypass IPMI entirely.

## Supported PDUs

- `tripplite`: Tripp Lite PDUs with the SNMPWEBCARD interface, driven through
  the telnet CLI, normally on port 5214. Written against a PDU3VSR10L2130.
  `status` is not implemented for this type; it prints `not yet implemented`.
- `servertech`: Server Tech switched CDUs, driven through the telnet CLI,
  normally on port 23. Outlets are named as the unit reports them, for
  example `.AA1` or `.AB5`.

powerbot has no daemon. It logs in to the PDU for every command and logs out
afterwards, which is simpler than keeping a session alive for commands that
happen a few times a day.

## Adding a PDU type

Implement the `PDU` interface from `cmd/powerbot/powerbot.go`:

```go
type PDU interface {
	On(map[string]string) error
	Off(map[string]string) error
	Cycle(map[string]string) error
	Status(map[string]string) error
	Temp() error // IPMI only - noop for PDUs
	Info() error // IPMI only - noop for PDUs
}
```

The map passed to `On`, `Off`, `Cycle`, and `Status` goes from node name to
outlet name (`ccc1` to `4` in the example above); most implementations only
need the outlets, but the names help when logging. `Temp` and `Info` can
simply return `nil`. Put the implementation in its own file, modelled on
`tripplite.go` or `servertech.go`, and add a constructor with the signature
`func(host, port, username, password string) (PDU, error)` to the `PDUtypes`
map in `powerbot.go`. The map key becomes the `<type>` you write in `device`
lines.

The `host` and `port` fields are passed through untouched, so a driver for a
serial-attached unit can treat `host` as a device path and ignore `port`,
`username`, and `password` by giving them placeholder values in the
configuration.

## See also

- [Cluster setup](cluster.md)
- [Tools overview](../tools.md)
