# Python bindings


<a id="TOC_1."></a>

## Introduction

minimega includes Python bindings for its CLI that interact with minimega using
the domain socket. These bindings are automatically generated and allow the
user to drive minimega with Python scripts.

<a id="TOC_2."></a>

## General Usage

The `pyapigen` tool generates `minimega.py` which can be imported into scripts.
The Python bindings include docstrings for every minimega command -- see
`help(minimega)` or the [generated Python API reference](../reference/python.md)
for a full listing. Here's a simple example of a script to launch 10 VMs,
display their info, and then kill them:

```python
import minimega

with minimega.connect(debug=True) as mm:
    print('launching vms')
    mm.vm_launch_kvm(10)

    print('vm info:')
    minimega.print_rows(mm.vm_info())

    print('starting vms')
    mm.vm_start('all')

    print('vm info:')
    minimega.print_rows(mm.vm_info())

    print('killing vms')
    mm.vm_kill('all')

    print('flushing vms')
    mm.vm_flush()
```

The `connect` method wraps `minimega.minimega.__init__`:

```python
help(minimega.connect):

connect(path='/tmp/minimega/minimega', raise_errors=True, debug=False, namespace=None)
    Connect to the minimega instance with UNIX socket at <path> and return
    a new minimega API object. See help(minimega.minimega) for an
    explanation of the other parameters.

help(minimega.minimega.__init__):

__init__(self, path, raise_errors, debug, namespace)
    Connects to the minimega instance with UNIX socket at <path>. If
    <raise_errors> is set, the Python APIs will raise an Exception whenever
    minimega returns a response with an error. If <debug> is set, debugging
    information will be printed. The <namespace> parameter allows you to
    "bind" the minimega object to a particular namespace (see
    help(minimega.minimega.namespace) for more info on namespaces).
```

By default, the domain socket is only read/writable by root so the Python
script must run as root (or the domain socket can be chmod'd appropriately).

A `minimega` client owns a live UNIX socket. Use `close()` when done, or prefer
`with minimega.connect(...) as mm:` so the socket is closed automatically when
the block exits. The context manager does not suppress exceptions raised inside
the block.

If `connect()` opens the socket but fails during its initial version check, it
closes the socket before re-raising the error.

### Threading and multiprocessing

A `minimega` client object owns a live socket connection, lock, and response
state. One connection is serial: all responses from one command must be read
before the next command can be issued. If the client is closed while streamed
responses are pending, the connection is closed and unread responses are
discarded with the socket. Use `minimega.discard(mm)` to drain pending responses
when the response contents are not needed and the connection will continue to be
used.

Use one connection per process. Do not pickle a connected client or share it
across `multiprocessing` workers. Instead, call `minimega.connect(...)` inside
each worker process.

The `namespace()` helper creates a shallow wrapper around the same underlying
connection. It is a convenience for namespace scoping, not a separate transport
or process-isolation boundary. Namespace wrappers share the parent connection's
socket, lock, and response state, so they should not outlive the parent
connection context.

<a id="TOC_3."></a>

## Running commands

As shown above, the Python bindings include a `print_rows` function that reads
all the responses from a minimega command. Due to the serial nature of the
minimega connection, all responses must be read before another command can be
issued. By default, commands return the first response and set a flag if there
are more responses to read (resp['More']). If `More` is true, users can call
`streamResponses` on the minimega instance which returns a generator for
additional responses. The helper, `minimega.discard`, can be used to discard
these if no action is required in the Python script (and errors are checked via
`raise_errors`).

All commands have arguments based on the underlying CLI which are checked by
the Python bindings. If an argument is invalid, a `ValueError` will be raised
with a description of the error.

For command-and-control operations, use `cc_filtered(...)` to scope a group of
`cc` commands and automatically clear the filter when the block exits:

```python
with minimega.connect() as mm:
    with mm.cc_filtered("uuid=abc"):
        mm.cc_exec_once("pkill iperf3")
```

`cc_filtered` rejects blank filters, raises if setting the filter returns any
error, and clears the filter even when the block raises.

<a id="TOC_4."></a>

## namespaces

The Python bindings support namespaces in several ways. First, users may
specify a namespace when connecting to minimega which runs all commands from
that connection in that namespace. Second, users can use `with` statements to
run commands in a particular namespace:

```python
with minimega.connect() as mm:
    with mm.namespace("foo") as ns:
        ns.vm_launch_kvm(1)

    with mm.namespace("foo") as ns:
        with mm.namespace("bar") as ns2:
            for resp in ns.vm_info():
                for row in resp.get("Tabular", []) or []:
                    ns2.vm_launch_kvm(row[1])
```
