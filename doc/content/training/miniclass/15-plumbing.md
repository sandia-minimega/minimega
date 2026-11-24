# Chapter 15: Plumbing

Plumbing moves lines of text between guests, host programs, and minimega
itself. It uses the cc channel instead of the experiment network.

In this lab, `vm-left` pings `vm-right` and writes the output into a named
pipe. `vm-right` reads the same pipe and logs it. A pipeline on the host
rewrites the lines on their way to a second pipe. A via (a program that runs
once for each reader) hands each reader of a pipe its own random sample. None
of the traffic touches `net_left` or `net_right`. Measurement and control stay
out of band.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) and
[Chapter 8](08-cc.md). The lab uses `cc background` and `cc filter`
throughout.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses). The
script at the end rebuilds it.

## A first pipe

A pipe is a named point that any number of readers and writers attach to
from anywhere in the cluster. It carries newline-delimited messages, not a
byte stream. A pipe delivers a message at once to the readers that are
present. If there are no readers, the pipe discards the message. Pipes belong
to the namespace in which you use them. For this reason, the listing shows
`sandwich//hello`.

Attach a reader from a second terminal on the host, then write from the
prompt:

```bash
$ minimega -pipe sandwich//hello
```

```minimega
minimega[sandwich]$ pipe hello "the sandwich says hello"
minimega[sandwich]$ .annotate false pipe
name            | mode | readers | writers | count | via | previous
sandwich//hello | all  | 1       | 1       | 1     |     | the sandwich says hello
```

The reader prints the line. `count` is the number of messages that the pipe
carried. `previous` is the last message. When you close its standard input,
the reader exits. `echo hi | minimega -pipe sandwich//hello` uses the same
program as a writer.

## From one VM to the other

Guests reach pipes in two ways. First, you can attach the standard streams of
a program that you start with `cc exec` or `cc background` to pipes. To do
this, prefix the command with `stdin=`, `stdout=`, or `stderr=` pairs.
Second, any guest shell can run `miniccc -pipe <name>`. This command talks to
the running agent through its socket under `/tmp/miniccc`. It connects the
standard input and output of the shell to the pipe until the standard input or
the pipe closes.

Use the first form on both clients. On `vm-left`, ping `vm-right` forever and
send the output to a pipe called `pings`. On `vm-right`, read that pipe into a
file:

```minimega
minimega[sandwich]$ cc filter name=vm-left
minimega[sandwich]$ cc background stdout=pings ping -i 1 10.0.1.10
minimega[sandwich]$ cc filter name=vm-right
minimega[sandwich]$ cc background stdin=pings tee /tmp/pings.log
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc exec tail -n 3 /tmp/pings.log
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ cc responses 6
minimega[sandwich]$ clear cc filter
```

minimega feeds the pipe straight into `tee`'s standard input, and `tee`
writes each line to the file. The response is the last three lines of
ping output, as received on `vm-right` through the cc channel. The ICMP went
one way over the router. The text came back the other way through minimega.
The `pings` row of the pipe listing shows one writer, one reader, and a rising
count:

```minimega
minimega[sandwich]$ .annotate false pipe
name            | mode | readers | writers | count | via | previous
sandwich//hello | all  | 0       | 0       | 1     |     | the sandwich says hello
sandwich//pings | all  | 1       | 1       | 14    |     | 64 bytes from 10.0.1.10: icmp_seq=14 ttl=63 tim...
```

In an interactive shell on `vm-right`, `miniccc -pipe pings > /tmp/pings.log`
does the same job. Do not start it with `cc background`. A background
command gets no standard input, so `miniccc -pipe` stops at once. To add a
third reader on the host, attach a host shell with
`minimega -pipe sandwich//pings`. This reader sees every line as well.

## A pipeline on the host

`plumb` connects pipes through external programs, in the order that you write
a shell pipeline. `plumb` treats anything that is not an executable on the
host's `PATH` as a pipe name. The programs run on the host where you issue
the command. The pipes are distributed as usual:

```minimega
minimega[sandwich]$ plumb pings "sed -u s/^/left:/" tagged
minimega[sandwich]$ .annotate false plumb
pipeline
sandwich//pings sed -u s/^/left:/ sandwich//tagged
minimega[sandwich]$ .annotate false pipe
name             | mode | readers | writers | count | via | previous
sandwich//hello  | all  | 0       | 0       | 1     |     | the sandwich says hello
sandwich//pings  | all  | 2       | 1       | 31    |     | 64 bytes from 10.0.1.10: icmp_seq=31 ttl=63 tim...
sandwich//tagged | all  | 0       | 1       | 17    |     | left:64 bytes from 10.0.1.10: icmp_seq=31 ttl=6...
```

`pings` now has two readers: `tee` on `vm-right` and `sed` on the host. Each
line appears on `tagged` with a prefix. `sed -u` is unbuffered. Without
this flag, or its equivalent for other filters, output waits in the buffer of
the program.

Several pipelines can share a pipe. If you run
`plumb pings "grep --line-buffered time=" rtt` alongside the first pipeline,
you get a tree.

## Delivery modes

By default, every reader gets every message. Two other modes deliver each
message to exactly one reader. These modes turn a pipe into a work queue. Open
`minimega -pipe sandwich//tagged` in two terminals and watch the lines
alternate.

```minimega
minimega[sandwich]$ pipe tagged mode round-robin
minimega[sandwich]$ pipe tagged mode random
minimega[sandwich]$ clear pipe tagged mode
```

## Vias

A via is a program that runs once for each reader on every write. The message
goes to the standard input of the program, and the output of the program goes
to that reader instead. With a via, one write can give every reader a
different value. minimega ships `normal`. `normal` reads a mean and prints one
sample from a normal distribution. The packages install it with the other
tools under `/opt/minimega/bin`, and a source build has it in `bin/`:

```minimega
minimega[sandwich]$ pipe rtt via /opt/minimega/bin/normal -stddev 5.0
minimega[sandwich]$ pipe rtt 100
```

Two readers attached with `minimega -pipe sandwich//rtt` print two different
numbers near 100. The via runs on the host where you issue `pipe <name> via`,
and the program must be on the `PATH` of that host.
`clear pipe rtt via` removes it. The same idea with a script of your own
turns one "start now" message into per-VM jitter, or one configuration line
into a per-client variant.

To see what flows through a pipe without attaching a reader, run
`pipe pings log true`. This writes every message to minimega's log at the
`debug` level. To see the messages, run `log level debug`.
`clear pipe pings log` stops the logging.

## Clean up

Clearing a pipe closes its readers with an end of file. This is how you stop
a pipeline. `tee` on `vm-right` exits, `sed` exits, and the pipeline is gone
from `plumb`.

But minimega refuses to delete a pipe that still has a writer
(`cannot delete named pipe with open writers`). Each `minimega -pipe` terminal
is also a writer. Close the standard input of every `minimega -pipe` terminal.
The ping on `vm-left` is still writing to `pings`. Kill the ping first, wait a
moment for the kill to arrive, and then clear the pipe. The pipeline stops when
its input pipe goes away. Then clear the other pipes of the lab by name:

```minimega
minimega[sandwich]$ cc filter name=vm-left
minimega[sandwich]$ cc process killall ping
minimega[sandwich]$ clear cc filter
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ clear pipe pings
minimega[sandwich]$ clear pipe tagged
minimega[sandwich]$ clear pipe rtt
minimega[sandwich]$ clear pipe hello
```

`clear plumb` with a pipeline written exactly as `plumb` lists it removes
that pipeline. `clear plumb` alone removes every pipeline on the host, in all
namespaces. `clear pipe` alone deletes every pipe on the host, in all
namespaces. Do not use the two short forms on a host that you share with other
experiments.

!!! warning "Loops"
    A pipeline whose output reaches its own input forwards the same message
    forever. Examples are `plumb foo cat foo` and two pipelines that form a
    cycle. To stop this, put a stage that stops on its own in the loop
    (`head -n 100`) or write to a different pipe.

## What you built

- A pipe, `pings`, carrying ping output from `vm-left` to a log file on
  `vm-right` without touching the experiment network.
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
