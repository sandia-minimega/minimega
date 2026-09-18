# Python API

minimega ships Python bindings: a single module named `minimega`, generated
from the same command definitions as the CLI. Every minimega command is a
method on a connection object; the module talks to a running daemon over its
UNIX command socket and returns the parsed JSON responses. This page shows how
to install the module, connect, run commands, read responses, work with
namespaces, and handle errors. It assumes a running minimega
([Running minimega](running.md)) and Python 3.6 or newer.

For the method-by-method listing see the
[Python API reference](../reference/python.md), or run `help(minimega)` after
importing the module.

## Installing

**From PyPI.** Every minimega release publishes the module as `minimega`, with
the same version number as the release:

```bash
$ pip install minimega
```

The bindings are generated from a specific minimega revision. On connect the
module runs `version` and prints
`WARNING: API was built using a different version of minimega` if the daemon
does not match, so install the release that matches your daemon.

**From the packages.** The `.deb` and `.rpm` install the repository's `lib/`
directory as `/opt/minimega/lib`, which contains `minimega.py`. Put that
directory on the module path, or copy the file next to your script:

```bash
$ PYTHONPATH=/opt/minimega/lib python3 script.py
```

`scripts/build.bash` also builds an sdist and a wheel into `lib/dist/` when
Python 3 is present on the build host, so check `/opt/minimega/lib/dist/` for
a wheel you can `pip install`.

**In Docker.** The image includes `/opt/minimega/lib` together with the
`README.md` and `VERSION` files that its `setup.py` needs, so inside the
container:

```bash
$ pip install /opt/minimega/lib
```

or copy the module out with
`docker cp <container>:/opt/minimega/lib/minimega.py .`.

**From source.** `./scripts/build.bash` regenerates `lib/minimega.py` by
running `pyapigen` against the freshly built `bin/minimega`, and
`./scripts/doc.bash` does the same. Never edit the generated file by hand.

## Connecting

```python
import minimega

mm = minimega.connect()
```

`connect(path='/tmp/minimega/minimega', raise_errors=True, debug=False, namespace=None)`
returns a `minimega.minimega` object bound to one socket connection.

- `path` is the command socket, `<base>/minimega`. Change it when the daemon
  runs with a different `-base`.
- `raise_errors` makes every method raise `minimega.Error` as soon as a
  response carries an error. Set it to `False` to inspect the `Error` field of
  each response yourself.
- `debug` prints each command as it is sent and each response as it arrives.
- `namespace` runs every command from this connection inside that namespace
  (see [Namespaces](#namespaces)).

There is no timeout parameter: a call blocks until minimega answers, and
minimega answers commands in the order it receives them.

minimega normally runs as root and creates the socket with root ownership, so
run scripts as root, or change the ownership or permissions of the socket file
for the account that runs them.

## Running commands

A command's words joined with underscores give the method name, with dashes
also becoming underscores:

| Command | Method |
|---|---|
| `vm info` | `mm.vm_info(summary=None)` |
| `vm launch kvm <name or count> [config]` | `mm.vm_launch_kvm(name, config=None)` |
| `vm config memory [value]` | `mm.vm_config_memory(value=None)` |
| `clear vm config` | `mm.clear_vm_config()` |
| `ns queueing [true,false]` | `mm.ns_queueing(true_or_false=None)` |
| `background-status [id]` | `mm.background_status(id=None)` |

Arguments are positional in the order of the command pattern; optional ones
default to `None`. The values are converted to strings and joined with
spaces, so pass what you would type at the prompt: `mm.vm_launch_kvm(10)`,
`mm.vm_launch_kvm("web[0-3]")`, `mm.vm_config_networks("LAN")`. Where a
pattern offers a fixed set of words, the method checks the value and raises
`ValueError("invalid value for ...")`; a combination of arguments that matches
no variant raises `ValueError("invalid argument combination")`. Nothing is
sent in either case.

Two commands are deliberately not generated: `namespace` and
`clear namespace`. Use the `namespace` parameter or the context manager
described below; the `ns` commands (`mm.ns_hosts()`, `mm.ns_queueing()`,
`mm.ns_schedule()`, and so on) are all available.

### Responses

Every method returns a list of response objects, one per host that answered.
Each is a dictionary with the fields `Host`, `Response` (plain text),
`Header` and `Tabular` (a table, for commands such as `vm info`), `Error`, and
`Data` (structured data for the commands that provide it). Two helpers make
tables easier to handle:

```python
minimega.print_rows(mm.vm_info())          # print every row of every response

for resp in mm.vm_info():
    for vm in minimega.as_dict(resp):      # rows as dicts keyed by column name
        print(vm["name"], vm["state"], vm["ip"])
```

Some commands return more than one batch of responses. The method returns the
first batch and sets `mm.moreResponses`; read the rest with
`mm.streamResponses()`, a generator, or throw them away with
`minimega.discard(mm)`. The connection is serial, so calling another method
while responses are pending raises
`Error('more responses to be read from last command')`.

### A complete example

The script below configures three KVM VMs on one VLAN, launches and starts
them, prints their name, state, and IPv4 address, and cleans up. Adjust the
disk path to an image on your host.

```python
import minimega

mm = minimega.connect()

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

Bind a connection to a namespace when you open it, and every command is sent
as `namespace <name> <command>`:

```python
lab = minimega.connect(namespace="lab")
lab.vm_launch_kvm(2)
```

Or scope a block of commands with `with`, which returns a copy of the
connection bound to the namespace. Copies share the socket, so commands still
run one at a time, and the `with` block does not delete the namespace when it
ends:

```python
mm = minimega.connect()

with mm.namespace("lab") as lab:
    lab.vm_launch_kvm(2)
    lab.vm_start("all")

with mm.namespace("lab") as lab, mm.namespace("copy") as copy:
    for resp in lab.vm_info():
        for vm in minimega.as_dict(resp):
            copy.vm_launch_kvm(vm["name"])
```

## Errors

- `minimega.Error` is raised for any command that minimega rejects, when
  `raise_errors` is on, with minimega's error text as the message. It is also
  raised when the socket closes unexpectedly or a write fails.
- `ValueError` is raised for bad arguments before anything is sent.
- The standard socket exceptions (`FileNotFoundError`, `PermissionError`,
  `ConnectionRefusedError`) come from `connect()` when the socket does not
  exist, is not writable, or has no daemon behind it.

```python
try:
    mm.vm_start("nosuchvm")
except minimega.Error as e:
    print("minimega:", e)
```

With `raise_errors=False`, check the `Error` field of each response instead:

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
form `{"Resp": [...], "Rendered": "...", "More": false}`, where `Resp` is the
list of responses described above, `Rendered` is the text the CLI would have
printed, and `More` says whether another line follows. The same socket
accepts `{"Suggest": "vm sta"}` for tab completion and
`{"PlumbPipe": "<name>"}` to attach to a plumbing pipe. Go programs should use
`pkg/miniclient` rather than reimplementing the framing. See
[Command line and scripting](cli.md).

## Regenerating the bindings

The bindings are produced by `pyapigen` from `minimega -cli`, which prints
every command pattern and help text as JSON. After changing or adding commands,
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
