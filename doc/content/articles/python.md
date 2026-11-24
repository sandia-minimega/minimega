# Python API

minimega ships Python bindings. The bindings are a single module named
`minimega`, generated from the same command definitions as the CLI. Every
minimega command, except those listed below, is a method on a connection object.
The module talks to a running daemon over its UNIX command socket and returns
the parsed JSON responses. This page shows how to install the module, connect,
run commands, read responses, work with namespaces, and handle errors. It
assumes a running minimega ([Running minimega](running.md)) and Python 3.10 or
newer.

For the method-by-method listing, see the
[Python API reference](../reference/python.md), or run `help(minimega)` after
you import the module.

## Installing

From PyPI, every minimega release publishes the module as `minimega`, with
the same version number as the release:

```bash
$ pip install minimega
```

`pyapigen` generates the bindings from a specific minimega revision. When you
connect, the module runs `version`. If the daemon does not match, the module
prints `WARNING: API was built using a different version of minimega`. Install
the release that matches your daemon.

From the packages, the `.deb` and `.rpm` install the `lib/` directory of the
repository as `/opt/minimega/lib`, which contains `minimega.py`. Put that
directory on the module path, or copy the file next to your script:

```bash
$ PYTHONPATH=/opt/minimega/lib python3 script.py
```

If Python 3 is present on the build host, `scripts/build.bash` also builds a
source distribution (sdist) and a wheel into `lib/dist/`. Look in
`/opt/minimega/lib/dist/` for a wheel that you can `pip install`.

In Docker, the image includes `/opt/minimega/lib`. This directory holds the
module and the `pyproject.toml`, `README.md`, and `VERSION` files that the
package build needs. Inside the container, run:

```bash
$ pip install /opt/minimega/lib
```

Or copy the module out with
`docker cp <container>:/opt/minimega/lib/minimega.py .`.

From source, `./scripts/build.bash` regenerates `lib/minimega.py`. It runs
`pyapigen` against the freshly built `bin/minimega`. `./scripts/doc.bash` does
the same. Do not edit the generated file by hand.

## Connecting

```python
import minimega

with minimega.connect() as mm:
    minimega.print_rows(mm.vm_info())
```

`connect(path='/tmp/minimega/minimega', raise_errors=True, debug=False, namespace=None)`
returns a `minimega.minimega` object bound to one socket connection.

- `path` is the command socket, `<base>/minimega`. If the daemon runs with a
  different `-base`, change `path`.
- `raise_errors` makes every method raise `minimega.Error` as soon as a
  response carries an error. Set it to `False` to inspect the `Error` field of
  each response yourself.
- `debug` prints each command as it is sent and each response as it arrives.
- `namespace` runs every command from this connection inside that namespace
  (see [Namespaces](#namespaces)).

There is no timeout parameter. A call blocks until minimega answers. minimega
answers commands in the order that it receives them.

The connection owns a live UNIX socket. When you are done, call `mm.close()`,
or use `with minimega.connect(...) as mm:` as above. The `with` block closes the
socket at its end. The `with` block does not suppress exceptions raised
inside it. If `connect()` opens the socket but its initial version check
fails, it closes the socket before it raises the error again.

minimega normally runs as root and creates the socket with root ownership. Run
scripts as root. Or change the ownership or permissions of the socket file for
the account that runs the scripts.

### Threading and multiprocessing

A connection object owns a live socket connection, a lock, and response
state. One connection is serial: read all responses from one command before
you issue the next command. If you close the connection while streamed
responses are pending, the unread responses are discarded with the socket. To
drain pending responses and continue to use the connection, call
`minimega.discard(mm)`.

Use one connection per process. Do not pickle a connected client or share it
across `multiprocessing` workers: the client raises `TypeError` if you try.
Call `minimega.connect(...)` inside each worker process instead.

## Running commands

The method name is the words of the command joined with underscores. Dashes
also become underscores:

| Command | Method |
|---|---|
| `vm info` | `mm.vm_info(summary=None)` |
| `vm launch kvm <name or count> [config]` | `mm.vm_launch_kvm(name, config=None)` |
| `vm config memory [value]` | `mm.vm_config_memory(value=None)` |
| `clear vm config` | `mm.clear_vm_config()` |
| `ns queueing [true,false]` | `mm.ns_queueing(true_or_false=None)` |
| `background-status [id]` | `mm.background_status(id=None)` |

Arguments are positional in the order of the command pattern. Optional arguments
default to `None`. The module converts the values to strings and joins them with
spaces. Pass what you type at the prompt: `mm.vm_launch_kvm(10)`,
`mm.vm_launch_kvm("web[0-3]")`, `mm.vm_config_networks("LAN")`.

If a pattern offers a fixed set of words, the method makes sure that the value
is one of them and raises `ValueError("invalid value for ...")`. A combination
of arguments that matches no variant raises
`ValueError("invalid argument combination")`. In both cases, nothing is sent.

The bindings do not include `namespace`, `clear namespace`, `help`, or the dot
commands such as `.filter` and `.columns`. Use the `namespace` parameter or the
context manager described below. All `ns` commands are available, for example
`mm.ns_hosts()`, `mm.ns_queueing()`, and `mm.ns_schedule()`.

### Responses

Every method returns a list of response objects, one per host that answered.
Each is a dictionary with the fields `Host`, `Response` (plain text), `Header`,
`Tabular`, `Error`, and `Data`. `Header` and `Tabular` hold a table, for
commands such as `vm info`. `Data` holds structured data for the commands that
provide it. Two helpers make tables easier to handle:

```python
minimega.print_rows(mm.vm_info())          # print every row of every response

for resp in mm.vm_info():
    for vm in minimega.as_dict(resp):      # rows as dicts keyed by column name
        print(vm["name"], vm["state"], vm["ip"])
```

Some commands return more than one batch of responses. The method returns the
first batch and sets `mm.moreResponses`. Read the rest with
`mm.streamResponses()`, a generator. Or throw them away with
`minimega.discard(mm)`. The connection is serial. If you call another method
while responses are pending, it raises
`Error('more responses to be read from last command')`.

### Scoping cc filters

For command and control, use `cc_filtered(...)` to set a `cc filter` for a
group of `cc` commands. At the end of the block, `cc_filtered` clears the
filter automatically:

```python
with minimega.connect() as mm:
    with mm.cc_filtered("uuid=abc"):
        mm.cc_exec_once("pkill iperf3")
```

`cc_filtered` rejects blank filters, raises if setting the filter returns an
error, and clears the filter even when the block raises. It clears any
previous filter. It does not restore it.

### A complete example

The script below configures three KVM VMs on one VLAN, launches and starts
them, prints their name, state, and IPv4 address, and cleans up. Adjust the
disk path to an image on your host. The script starts and kills all VMs in the
active namespace, so run it in a namespace that has no other VMs.

```python
import minimega

with minimega.connect() as mm:
    mm.clear_vm_config()
    mm.vm_config_memory(1024)
    mm.vm_config_vcpus(1)
    mm.vm_config_disks("/tmp/minimega/files/debian.qc2")
    mm.vm_config_networks("LAN")

    mm.vm_launch_kvm(3)
    mm.vm_start("all")

    for resp in mm.vm_info():
        for vm in minimega.as_dict(resp):
            print(f'{vm["name"]:12} {vm["state"]:10} {vm["ip"]}')

    mm.vm_kill("all")
    mm.vm_flush()
```

## Namespaces

When you open a connection, you can bind it to a namespace. The connection then
sends every command as `namespace <name> <command>`:

```python
lab = minimega.connect(namespace="lab")
lab.vm_launch_kvm(2)
```

Or scope a block of commands with `with`, which returns a copy of the
connection bound to the namespace. Copies share the socket and the lock, so
commands still run one at a time. Each copy tracks its own pending-response
flag, so read all responses from a copy before you use the parent.
A copy is not a separate transport or an isolation boundary between
processes, and it must not outlive the parent connection. When the `with`
block ends, it does not delete the namespace:

```python
with minimega.connect() as mm:
    with mm.namespace("lab") as lab:
        lab.vm_launch_kvm(2)
        lab.vm_start("all")

    with mm.namespace("lab") as lab, mm.namespace("copy") as copy:
        for resp in lab.vm_info():
            for vm in minimega.as_dict(resp):
                copy.vm_launch_kvm(vm["name"])
```

## Errors

- When `raise_errors` is on, the module raises `minimega.Error` for any command
  that minimega rejects, with the error text of minimega as the message. If the
  socket closes unexpectedly or a write fails, the module also raises it.
- The module raises `ValueError` for bad arguments before it sends anything.
- The socket can be missing, not writable, or without a daemon. In these cases,
  `connect()` raises the standard socket exceptions (`FileNotFoundError`,
  `PermissionError`, `ConnectionRefusedError`).

```python
try:
    mm.vm_start("nosuchvm")
except minimega.Error as e:
    print("minimega:", e)
```

With `raise_errors=False`, inspect the `Error` field of each response instead:

```python
mm = minimega.connect(raise_errors=False)
for resp in mm.vm_start("all"):
    if resp["Error"]:
        print(resp["Host"], resp["Error"])
```

## The protocol underneath

The module is a thin wrapper around the command socket. A client connects to
`<base>/minimega` and writes one JSON object per request:
`{"Command": "vm info"}`. minimega answers with one or more JSON lines of the
form `{"Resp": [...], "Rendered": "...", "More": false}`. In this form, `Resp`
is the list of responses described above. `Rendered` is the text that the CLI
prints. `More` says whether another line follows.

The same socket accepts `{"Suggest": "vm sta"}` for tab completion and
`{"PlumbPipe": "<name>"}` to attach to a plumbing pipe. We recommend that Go
programs use `pkg/miniclient` rather than reimplement the framing. See
[Command line and scripting](cli.md).

## Regenerating the bindings

`pyapigen` produces the bindings from `minimega -cli`, which prints every
command pattern and help text as JSON. After you change or add commands,
rebuild and run:

```bash
$ ./scripts/doc.bash
```

This regenerates `lib/minimega.py` along with the command reference pages.
Commit the regenerated file with the change to the command.

## See also

- [Python API reference](../reference/python.md)
- [Command line and scripting](cli.md)
- [Namespaces](namespaces.md)
- [Tools overview](../tools.md)
