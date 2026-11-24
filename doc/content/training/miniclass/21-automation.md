# Chapter 21: Automation

A program can send everything that you typed at the `minimega$` prompt. In this
chapter you drive minimega in three ways: from a shell with `minimega -e`, from
Python with the generated bindings, and directly over the command socket. The
command socket shows what the first two ways do. The main part is a Python
script that builds the router sandwich and prints a table of its VMs. At the
end, you have the script running, and you know how to extend it.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md), command
files from [Chapter 9](09-save-replay-cleanup.md), and namespaces from
[Chapter 16](16-namespaces.md). You need Python 3.10 or newer on the
minimega host.

## Three ways in

minimega listens on a UNIX socket, `/tmp/minimega/minimega` by default.
`minimega -attach` (the attached prompt), `minimega -e`, miniweb, and the Python
bindings are all clients of that socket. They send the same commands and receive
the same responses. The right client depends on how much logic sits around the
commands:

- none, or a straight line of commands: a `.mm` file and `read`
  ([Chapter 9](09-save-replay-cleanup.md))
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

`-namespace` prepends `namespace <name>` to the command, so a script does not
have to. Each `-e` is its own connection, and a connection costs a few
milliseconds. This is fine for a loop over a handful of VMs:

```bash
#!/bin/bash
MM="sudo minimega -namespace sandwich -e"

for vm in vm-left vm-right; do
    $MM cc filter name=$vm
    $MM cc exec uname -a
done
$MM clear cc filter
```

For structured output, ask for JSON. Parse it with `jq`:

```bash
$ sudo minimega -namespace sandwich -e .json true vm info | jq '.[0].Tabular'
```

Here `.json true` applies to this command only. If you type it at the prompt
without a command, it changes the setting for the session.

## The Python bindings

### Installing

The module is a single file, `minimega.py`. It is generated from the same
command definitions as the CLI, so every command is a method. It is published on
PyPI with the same version number as the release:

```bash
$ pip install minimega
```

The packages also install the module as `/opt/minimega/lib/minimega.py`. If the
build host had Python, the packages also install a wheel under
`/opt/minimega/lib/dist/`. The Docker image has `/opt/minimega/lib` too. Install
the version that matches your daemon. On connect, the module runs `version` and
prints
`WARNING: API was built using a different version of minimega` if they
differ. A source build regenerates the file with `./scripts/doc.bash`.

### Connecting and calling

```python
import minimega

mm = minimega.connect(namespace="sandwich")
```

`connect(path='/tmp/minimega/minimega', raise_errors=True, debug=False,
namespace=None)` opens the socket and returns an object with one method for each
command. A command's words joined with underscores give the method name. A dash
in a word also becomes an underscore. For example, `vm info` is `mm.vm_info()`,
`vm launch kvm <name>` is `mm.vm_launch_kvm(name)`, and `vm config android-avd`
is `mm.vm_config_android_avd(...)`. The command
`router <vm> dhcp <listen> range <low> <high>` is
`mm.router_dhcp_range(vm, listen, low, high)`, and `clear vm config` is
`mm.clear_vm_config()`.

Arguments are positional in pattern order, and optional arguments default to
`None`. The module turns every value into a string and joins the values with
spaces. You therefore pass exactly what you type:
`mm.vm_config_networks("net_left net_right")`, `mm.vm_config_memory(3072)`.
`help(minimega)` lists every method with the help text that you know from
`help <command>`. The [Python API reference](../../reference/python.md) is the
same listing online.

The connection holds the socket open. When you are done, call `mm.close()`.
Alternatively, open the connection with
`with minimega.connect(namespace="sandwich") as mm:`. When the block ends, the
socket closes. Use one connection per process, because a connection cannot
be pickled or shared with `multiprocessing` workers.

Two commands are deliberately missing: `namespace` and `clear namespace`. Bind
the connection to a namespace with the `namespace` parameter, as above.
Alternatively, scope a block of calls with
`with mm.namespace("sandwich") as ns:`. To destroy a namespace, run
`minimega -e clear namespace <name>` from the shell.

With `raise_errors=True`, the default, any command that minimega rejects raises
`minimega.Error` with the message of minimega. A bad argument raises
`ValueError` before the module sends anything. If the daemon is not there or you
are not root, the socket errors (`FileNotFoundError`, `PermissionError`,
`ConnectionRefusedError`) come from `connect()`.

### Reading responses

Every method returns a list of response dictionaries, one for each host that
answered. Each dictionary has the keys `Host`, `Response` (plain text), `Header`
and `Tabular` (a table), `Error`, and `Data`.

Two helpers handle tables. `minimega.print_rows(resps)` prints every row.
`minimega.as_dict(resp)` turns the table of one response into a list of
dictionaries keyed by column name. The script below uses it to read `vm info`.

Cells are strings, exactly as the prompt renders them, so the `vlan` of a VM is
`[net_left (101)]`, not a list.

A few commands stream more than one batch of responses. The method returns the
first batch and sets `mm.moreResponses`. Read the rest with
`mm.streamResponses()`, or drop them with `minimega.discard(mm)`. The socket is
serial, so the next call fails until you read or drop the rest.

### The script

The script builds the Chapter 3 sandwich in the `sandwich` namespace and prints
one line per VM. Compare the script with `03-router-sandwich.mm`. The commands
are the same, in the same order, with three differences. First, the script
starts with `mm.clear_vm_config()`. The `.mm` file uses `clear namespace`
instead, which the bindings cannot run. Second, the shell loop that waits for
the guests becomes `wait_ready()`. This function polls `vm info` until every VM
has `cc_active` true and an address. Third, the final `.columns` becomes a loop
over `as_dict`.

```python title="21-automation.py"
--8<-- "training/miniclass/scripts/21-automation.py"
```

[Download this script](scripts/21-automation.py){ download="21-automation.py" }

The earlier chapter scripts leave VMs in the `sandwich` namespace, and the
script cannot clear a namespace. Clear the namespace with `-e` before you run
the script. Run the script as root, because the socket is root-owned. Root must
be able to import the module. If you installed it with `pip` as a normal user,
use `sudo env PYTHONPATH=/opt/minimega/lib python3 21-automation.py`, which
uses the copy from the package. Then clean
up with `-e` again:

```bash
$ sudo minimega -e clear namespace sandwich
$ sudo python3 21-automation.py
router     RUNNING   [net_left (101), net_right (102)]  [10.0.0.1, 10.0.1.1]
vm-left    RUNNING   [net_left (101)]                   [10.0.0.2]
vm-right   RUNNING   [net_right (102)]                  [10.0.1.2]
$ sudo minimega -e clear namespace sandwich
```

We recommend that you copy three parts of the script into your own scripts.
`build()` and `report()` take the connection as an argument, so the same
functions work on a connection that is bound to any namespace. `report()` skips
responses whose `Tabular` is empty, which is what an empty namespace or a host
with no VMs returns. `main()` separates the socket errors from `minimega.Error`.
The socket errors mean that minimega is not reachable, and `minimega.Error`
means that minimega refused a command. Both messages tell you which.

From here, a script can do what a `.mm` file cannot. A script can wait for a `cc`
response and retry a launch that failed. It can also launch as many VMs as
`host` says fit, or pull `vm info` into a data frame. The calls are the
commands, so you can do here anything that you can do at the prompt. The
reference for both is the same.

## The socket underneath

When you need a language other than Python, or want to know what the bindings
do, the protocol is small. A client connects to the UNIX socket and writes one
JSON object per request:

```json
{"Command": "vm info"}
```

minimega answers with one or more JSON lines of the form
`{"Resp": [...], "Rendered": "...", "More": false}`. `Resp` is the list of
per-host responses described above. `Rendered` is the text that the prompt
prints. `More` is `true` until the last line of a command that streams.

The same socket takes `{"Suggest": "vm sta"}` for tab completion and
`{"PlumbPipe": "<name>"}` to attach to a plumbing pipe. For Go, `pkg/miniclient`
in the repository is the client that `minimega -e` and miniweb use.
`minimega -cli` prints the complete command grammar as JSON for anyone who
generates bindings of their own.

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
