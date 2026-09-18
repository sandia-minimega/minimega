# Chapter 15: Plumbing

Plumbing moves lines of text between guests, host programs and minimega
itself, over the cc channel rather than the experiment network. In this lab
`vm_left` pings `vm_right` and writes the output into a named pipe;
`vm_right` reads the same pipe and logs it; a pipeline on the host rewrites
the lines on their way to a second pipe; and a via hands each reader of a
pipe its own random sample. None of the traffic touches `net_left` or
`net_right`, which is the point: measurement and control stay out of band.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) and
[Chapter 8](08-cc.md); `cc background` and `cc filter` are used throughout.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses). The
script at the end rebuilds it.

## A first pipe

A pipe is a named point that any number of readers and writers attach to
from anywhere in the cluster. It carries newline-delimited messages, not a
byte stream; a message is delivered at once to the readers present and
discarded if there are none. Pipes belong to the namespace they are used in,
which is why the listing shows `sandwich//hello`.

Attach a reader from a second terminal on the host, then write from the
prompt:

```bash
$ minimega -pipe sandwich//hello
```

```minimega
minimega[sandwich]$ pipe hello "the sandwich says hello"
minimega[sandwich]$ .annotate false pipe
name            | mode | readers | writers | count | via | previous
sandwich//hello | all  | 1       | 0       | 1     |     | the sandwich says hello
```

The reader prints the line. `count` is the number of messages the pipe has
carried and `previous` the last one. The reader exits when you close its
standard input; `echo hi | minimega -pipe sandwich//hello` is the same program as a
writer.

## From one VM to the other

Guests reach pipes two ways. A program started with `cc exec` or
`cc background` can have its standard streams attached to pipes by prefixing
the command with `stdin=`, `stdout=` or `stderr=` pairs. And any guest shell
can run `miniccc -pipe <name>`, which talks to the running agent through its
socket under `/tmp/miniccc` and connects the shell's standard input and
output to the pipe until the pipe closes. Use the first form on `vm_left`,
to ping `vm_right` forever and send the output to a pipe called `pings`, and
the second on `vm_right`, to read that pipe into a file:

```minimega
minimega[sandwich]$ cc filter name=vm_left
minimega[sandwich]$ cc background stdout=pings ping -i 1 10.0.1.10
minimega[sandwich]$ cc filter name=vm_right
minimega[sandwich]$ cc background sh -c "miniccc -pipe pings > /tmp/pings.log"
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc exec tail -n 3 /tmp/pings.log
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ cc responses 6
minimega[sandwich]$ clear cc filter
```

The quoted string reaches the guest as a single argument, so the shell
redirect happens inside the guest. The response is the last three lines of
ping output, as received on `vm_right` through the cc channel; the ICMP went
one way over the router, the text came back the other way through minimega.
The pipe listing shows one writer, one reader and a rising count:

```minimega
minimega[sandwich]$ .annotate false pipe
name            | mode | readers | writers | count | via | previous
sandwich//pings | all  | 1       | 1       | 14    |     | 64 bytes from 10.0.1.10: icmp_seq=14 ttl=63 time=0.412 ms
```

Without a shell, `cc background stdin=pings tee /tmp/pings.log` on
`vm_right` does the same job: minimega feeds the pipe straight into `tee`'s
standard input. A host shell attaches with `minimega -pipe sandwich//pings`, so a
third reader on the host would see every line as well.

## A pipeline on the host

`plumb` connects pipes through external programs, in the order you would
write a shell pipeline. Anything that is not an executable on the host's
`PATH` is taken to be a pipe name; the programs run on the host where you
issue the command, the pipes are distributed as usual:

```minimega
minimega[sandwich]$ plumb pings "sed -u s/^/left:/" tagged
minimega[sandwich]$ .annotate false plumb
pipeline
sandwich//pings sed -u s/^/left:/ sandwich//tagged
minimega[sandwich]$ .annotate false pipe
name             | mode | readers | writers | count | via | previous
sandwich//pings  | all  | 2       | 1       | 31    |     | 64 bytes from 10.0.1.10: icmp_seq=31 ttl=63 time=0.398 ms
sandwich//tagged | all  | 0       | 1       | 17    |     | left:64 bytes from 10.0.1.10: icmp_seq=31 ttl=63 time=0.398 ms
```

`pings` now has two readers, `miniccc -pipe` on `vm_right` and `sed` on the
host, and
each line appears on `tagged` with a prefix. `sed -u` is unbuffered; without
it, and its equivalent for other filters, output waits in the program's
buffer. Several pipelines can share a pipe, so `plumb pings "grep --line-buffered time=" rtt`
alongside the first one gives you a tree.

## Delivery modes

By default every reader gets every message. Two other modes deliver each
message to exactly one reader, which turns a pipe into a work queue: open
`minimega -pipe sandwich//tagged` in two terminals and watch the lines alternate.

```minimega
minimega[sandwich]$ pipe tagged mode round-robin
minimega[sandwich]$ pipe tagged mode random
minimega[sandwich]$ clear pipe tagged mode
```

## Vias

A via is a program run once per reader on every write, with the message on
its standard input and its output delivered to that reader instead. It is
how one write can give every reader a different value. minimega ships
`normal`, which reads a mean and prints one sample from a normal
distribution; the packages install it with the other tools under
`/opt/minimega/bin`, a source build has it in `bin/`:

```minimega
minimega[sandwich]$ pipe rtt via /opt/minimega/bin/normal -stddev 5.0
minimega[sandwich]$ pipe rtt 100
```

Two readers attached with `minimega -pipe sandwich//rtt` print two different numbers
near 100. The via runs on the host where each delivery happens.
`clear pipe rtt via` removes it. The same idea with a script of your own
turns one "start now" message into per-VM jitter, or one configuration line
into a per-client variant.

To see what flows through a pipe without attaching a reader,
`pipe pings log true` writes every message to minimega's log at the `debug`
level (`log level debug` to see it), and `clear pipe pings log` stops that.

## Clean up

Clearing a pipe closes its readers with an end of file, which is how you
stop a pipeline: `miniccc -pipe` on `vm_right` exits, `sed` exits, and the
pipeline is gone from `plumb`. The ping on `vm_left` is still running and still writing
to a pipe that nobody reads, so kill it too:

```minimega
minimega[sandwich]$ clear pipe pings
minimega[sandwich]$ cc process killall ping
minimega[sandwich]$ clear plumb
minimega[sandwich]$ clear pipe
```

`clear plumb` with a pipeline written exactly as `plumb` lists it removes
that one; alone it removes them all. `clear pipe` alone deletes every pipe
in the namespace.

!!! warning "Loops"
    A pipeline whose output reaches its own input, `plumb foo cat foo` or two
    pipelines forming a cycle, forwards the same message forever. Put a stage
    that stops on its own in the loop (`head -n 100`) or write to a different
    pipe.

## What you built

- A pipe, `pings`, carrying ping output from `vm_left` to a log file on
  `vm_right` without touching the experiment network.
- A host-side pipeline that rewrites those lines into a second pipe,
  `tagged`, and the delivery modes that turn a pipe into a queue.
- A via on `rtt` that gives each reader its own sample from one write.

## Where to read more

- [Plumbing](../../articles/plumbing.md): [pipes](../../articles/plumbing.md#pipes),
  [delivery modes](../../articles/plumbing.md#delivery-modes),
  [vias](../../articles/plumbing.md#vias),
  [pipelines](../../articles/plumbing.md#pipelines), and
  [copying a file between VMs](../../articles/plumbing.md#copying-a-file-between-vms).
- [Command and control](../../articles/cc.md#plumbing) for `stdin=`,
  `stdout=` and `stderr=` on `cc exec` and `cc background`.
- [Command line and scripting](../../articles/cli.md) for `minimega -pipe`.
- Reference: [`pipe`](../../reference/minimega.md#pipe),
  [`plumb`](../../reference/minimega.md#plumb),
  [`clear pipe`](../../reference/minimega.md#clear-pipe),
  [`clear plumb`](../../reference/minimega.md#clear-plumb).

## Script

```minimega title="15-plumbing.mm"
--8<-- "training/miniclass/scripts/15-plumbing.mm"
```

[Download this example](scripts/15-plumbing.mm){ download="15-plumbing.mm" }
