# Chapter 21: Automation

Everything you have typed at the `minimega$` prompt can be sent by a
program. In this chapter you drive minimega three ways: from a shell with
`minimega -e`, from Python with the generated bindings, and, so you know
what both of those are doing, directly over the command socket. The
centrepiece is a Python script that builds the router sandwich and prints a
table of its VMs; at the end you have it running and know how to extend it.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md), command
files from [Chapter 9](09-save-replay-cleanup.md), and namespaces from
[Chapter 16](16-namespaces.md). You need Python 3.6 or newer on the
minimega host.

## Three ways in

minimega listens on a UNIX socket, `/tmp/minimega/minimega` by default. The
interactive prompt, `minimega -attach`, `minimega -e`, miniweb, and the
Python bindings are all clients of that socket, sending the same commands
and receiving the same responses. Which one to use depends on how much logic
sits around the commands:

- none, or a straight line of commands: a `.mm` file and `read`
  ([Chapter 9](09-save-replay-cleanup.md));
- a loop or a condition in a shell script: `minimega -e`;
- anything that needs to look at the responses: Python.

## From the shell

`minimega -e` sends the rest of its command line to the running daemon,
prints the rendered response, and exits:

```bash
$ sudo minimega -e vm info
$ sudo minimega -e .columns name,state vm info
$ sudo minimega -namespace sandwich -e .columns name,state,ip vm info
$ sudo minimega -e read /srv/miniclass/scripts/03-router-sandwich.mm
```

`-namespace` prepends `namespace <name>` to the command, so a script does
not have to. Each `-e` is its own connection, which costs a few
milliseconds; that is fine for a loop over a handful of VMs:

```bash
#!/bin/bash
MM="sudo minimega -namespace sandwich -e"

for vm in vm_left vm_right; do
    $MM cc filter name=$vm
    $MM cc exec uname -a
done
$MM clear cc filter
```

For structured output ask for JSON and parse it with `jq`:

```bash
$ sudo minimega -namespace sandwich -e .json true vm info | jq '.[0].Tabular'
```

`.json true` here applies to this command only; typed at the prompt without
a command it changes the setting for the session.

## The Python bindings

### Installing

The module is a single file, `minimega.py`, generated from the same command
definitions as the CLI, so every command is a method. It is published on
PyPI with the same version number as the release:

```bash
$ pip install minimega
```

The packages also install it as `/opt/minimega/lib/minimega.py` and, when
the build host had Python, a wheel under `/opt/minimega/lib/dist/`; the
Docker image has `/opt/minimega/lib` too. Install the version that matches
your daemon: on connect the module runs `version` and prints
`WARNING: API was built using a different version of minimega` if they
differ. A source build regenerates the file with `./scripts/doc.bash`.

### Connecting and calling

```python
import minimega

mm = minimega.connect(namespace="sandwich")
```

`connect(path='/tmp/minimega/minimega', raise_errors=True, debug=False,
namespace=None)` opens the socket and returns an object with one method per
command. A command's words joined with underscores give the method name,
dashes included: `vm info` is `mm.vm_info()`, `vm launch kvm <name>` is
`mm.vm_launch_kvm(name)`, `router <vm> dhcp <listen> range <low> <high>` is
`mm.router_dhcp_range(vm, listen, low, high)`, `clear vm config` is
`mm.clear_vm_config()`. Arguments are positional in pattern order, optional
ones default to `None`, and every value is turned into a string and joined
with spaces, so you pass exactly what you would type:
`mm.vm_config_networks("net_left net_right")`, `mm.vm_config_memory(2048)`.
`help(minimega)` lists every method with the help text you know from
`help <command>`, and the
[Python API reference](../../reference/python.md) is the same listing
online.

Two commands are deliberately missing: `namespace` and `clear namespace`.
Bind the connection to a namespace with the `namespace` parameter, as
above, or scope a block of calls with `with mm.namespace("sandwich") as ns:`;
destroy a namespace from the shell with `minimega -e clear namespace`.

With `raise_errors=True`, the default, any command minimega rejects raises
`minimega.Error` with minimega's message. A bad argument raises
`ValueError` before anything is sent, and the socket errors
(`FileNotFoundError`, `PermissionError`, `ConnectionRefusedError`) come
from `connect()` when the daemon is not there or you are not root.

### Reading responses

Every method returns a list of response dictionaries, one per host that
answered, with `Host`, `Response` (plain text), `Header` and `Tabular` (a
table), `Error`, and `Data`. Two helpers make tables easy:
`minimega.print_rows(resps)` prints every row, and `minimega.as_dict(resp)`
turns one response's table into a list of dictionaries keyed by column name,
which is how the script below reads `vm info`. Cells are strings, exactly as
the prompt renders them, so a VM's `vlan` is `[net_left (101)]`, not a
list.

A few commands stream more than one batch of responses. The method returns
the first and sets `mm.moreResponses`; read the rest with
`mm.streamResponses()` or drop them with `minimega.discard(mm)`, because
the socket is serial and the next call fails until they are consumed.

### The script

The script builds the Chapter 3 sandwich in the `sandwich` namespace and
prints one line per VM. Compare it with `03-router-sandwich.mm`: the
commands are the same, in the same order, with `shell sleep` replaced by
`time.sleep` and the final `.columns` by a loop over `as_dict`.

```python title="21-automation.py"
--8<-- "training/miniclass/scripts/21-automation.py"
```

[Download this script](scripts/21-automation.py){ download="21-automation.py" }

Run it as root, because the socket is root-owned, and clean up with `-e`:

```bash
$ sudo python3 21-automation.py
router     RUNNING   [net_left (101), net_right (102)]  [10.0.0.1, 10.0.1.1]
vm_left    RUNNING   [net_left (101)]                   [10.0.0.2]
vm_right   RUNNING   [net_right (102)]                  [10.0.1.2]
$ sudo minimega -e clear namespace sandwich
```

Three things in it are worth copying into your own scripts. `build()` and
`report()` take the connection as an argument, so the same functions work
on a connection bound to any namespace. `report()` skips responses whose
`Tabular` is empty, which is what an empty namespace or a host with no VMs
returns. And `main()` separates the socket errors, which mean minimega is
not reachable, from `minimega.Error`, which means minimega refused a
command; both messages tell you which.

From here a script can do what a `.mm` file cannot: wait for a `cc`
response instead of sleeping a fixed time, retry a launch that failed,
launch as many VMs as `host` says fit, or pull `vm info` into a data
frame. Because the calls are the commands, anything you can do at the
prompt you can do here, and the reference for both is the same.

## The socket underneath

When you need a language other than Python, or want to know what the
bindings are hiding, the protocol is small. A client connects to the UNIX
socket and writes one JSON object per request:

```json
{"Command": "vm info"}
```

minimega answers with one or more JSON lines of the form
`{"Resp": [...], "Rendered": "...", "More": false}`: `Resp` is the list of
per-host responses described above, `Rendered` is the text the prompt
would have printed, and `More` is `true` until the last line of a command
that streams. The same socket takes `{"Suggest": "vm sta"}` for tab
completion and `{"PlumbPipe": "<name>"}` to attach to a plumbing pipe. For
Go, `pkg/miniclient` in the repository is the client that `minimega -e`
and miniweb use, and `minimega -cli` prints the complete command grammar as
JSON for anyone generating bindings of their own.

## What you built

- A shell one-liner and a loop that drive minimega with `-e`, in a chosen
  namespace, with JSON output for `jq`.
- The router sandwich built by a Python script through the bindings, with
  `vm info` read back as dictionaries.
- A picture of the command socket that all of these use, and where to find
  the Go client and the grammar.

## Where to read more

- [Python API](../../articles/python.md): installation on every platform,
  namespaces with `with`, errors, and regenerating the bindings.
- [Python API reference](../../reference/python.md)
- [Command line and scripting](../../articles/cli.md): `-e`, the builtins
  such as `.json` and `.columns`, command files, and the socket protocol.
- [Running minimega](../../articles/running.md) for the `-namespace` flag
  and the socket's location under a different `-base`.
