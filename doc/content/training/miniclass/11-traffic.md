# Chapter 11: Background traffic with protonuke

A network with nothing on it is a poor testbed. In this lab you fill the
router sandwich with plausible traffic: protonuke serves web and mail on
`vm_left`, a second protonuke on `vm_right` browses and sends mail to it
through the router, and you watch the load from the minimega prompt. Along
the way you use `cc send` to push a binary into a guest, `cc background` to
start daemons, and `cc process list` and `vm top` to see what they are doing.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) for
the sandwich itself and [Chapter 8](08-cc.md) for `cc`. Read the
[protonuke](../../articles/protonuke.md) guide afterwards for the protocols
and flags this lab does not touch.

## Rebuild the sandwich with fixed addresses

The clients of chapter 3 take whatever lease the router hands out, which is
fine for a demo but awkward when one VM has to name the other. Give each
client a fixed MAC address in its netspec and a static lease on the router so
that `vm_left` is always `10.0.0.10` and `vm_right` is always `10.0.1.10`:

```minimega
minimega$ clear namespace sandwich
minimega$ namespace sandwich
minimega[sandwich]$ vm config kernel miniccc.kernel
minimega[sandwich]$ vm config initrd miniccc.initrd
minimega[sandwich]$ vm config memory 2048
minimega[sandwich]$ vm config networks net_left,00:00:00:00:01:01
minimega[sandwich]$ vm launch kvm vm_left
minimega[sandwich]$ vm config networks net_right,00:00:00:00:02:01
minimega[sandwich]$ vm launch kvm vm_right
```

The router is launched and described as in chapter 3, plus one `static` line
per subnet:

```minimega
minimega[sandwich]$ router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
minimega[sandwich]$ router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
```

The script at the end of the chapter has the complete sequence. Every later
lab in Part II starts the same way. After `vm start all` and a short wait,
confirm the addresses:

```minimega
minimega[sandwich]$ .annotate false .columns name,state,ip vm info
name     | state   | ip
router   | RUNNING | [10.0.0.1, 10.0.1.1]
vm_left  | RUNNING | [10.0.0.10]
vm_right | RUNNING | [10.0.1.10]
```

## Get protonuke into the guests

The client image built in chapter 2 contains miniccc but not protonuke. The
packages install protonuke as `/usr/bin/protonuke` on the host (the Docker
image has it in `/opt/minimega/bin`, a source build in `bin/`), and `cc send`
can deliver any file that sits under minimega's files directory,
`/tmp/minimega/files` by default. Copy it there and send it to every client:

```minimega
minimega[sandwich]$ shell cp /usr/bin/protonuke /tmp/minimega/files/protonuke
minimega[sandwich]$ cc send protonuke
```

Sent files land in `/tmp/miniccc/files/` inside the guest with the same
permissions they had on the host, and miniccc looks in that directory for a
command that is not on the guest's `PATH`, so from now on `protonuke` is a
command the guests can run by name. If you would rather bake it into the
image, copy the binary into the vmbetter overlay before building; the
overlay is copied into the image root, so
`misc/vmbetter_configs/miniccc_overlay/usr/local/bin/protonuke` becomes
`/usr/local/bin/protonuke` in the guest.

## Serve on vm_left

One protonuke process with `-serve` runs a server for every protocol you
enable. Start it on `vm_left` as a background command, since it never
returns:

```minimega
minimega[sandwich]$ cc filter name=vm_left
minimega[sandwich]$ cc background protonuke -serve -http -https -smtp
```

Leave SSH out of the server set: the client image already runs sshd on port
22, and two servers cannot share it. HTTP and HTTPS answer every `GET` with a
generated page and a random 3 MB image; SMTP accepts mail and drops it.

Check it from the other side of the router. The client image has `curl`:

```minimega
minimega[sandwich]$ cc filter name=vm_right
minimega[sandwich]$ cc exec curl -s http://10.0.0.10/
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ cc responses 6
```

The response is protonuke's generated page: a heading, the request URI, three
links back to the same host, a hit counter, and an `<img src=image.png>` tag.
Every `cc responses` file is printed under its path, `6/<uuid>/stdout:`, where
`6` is the command id shown by `cc commands` (the router commit used 1 to 3, `cc send` 4, and `cc background` 5).

## Generate load from vm_right

Now run protonuke as a client on `vm_right`, pointed at the server. Each
enabled protocol runs its own loop that sleeps a random interval around `-u`
(1 second by default) and then performs one action against one target:

```minimega
minimega[sandwich]$ cc background protonuke -http -https -smtp -u 500ms 10.0.0.10
minimega[sandwich]$ clear cc filter
```

The target can also be a subnet, `10.0.0.0/24`, which spreads the same rate
over every address in it. The [protonuke guide](../../articles/protonuke.md#how-it-works)
explains the timing flags (`-u`, `-s`, `-min`, `-max`) and the per-protocol
behaviour.

## Watch it

Three views, from the host, without touching the guests. `cc process list`
shows the processes miniccc started in the background, with the PID it
tracks them by:

```minimega
minimega[sandwich]$ .annotate false cc process list all
name     | uuid   | pid | command
vm_left  | <uuid> | 412 | protonuke -serve -http -https -smtp
vm_right | <uuid> | 415 | protonuke -http -https -smtp -u 500ms 10.0.0.10
```

`cc commands` lists what has been queued, how many clients answered, and
whether a command was backgrounded:

```minimega
minimega[sandwich]$ .annotate false .columns id,command,responses,background cc commands
```

And `vm top` samples host-side resource usage of every VM over a window of
seconds. `rx` and `tx` are MB/s as seen from the host, so with a 3 MB image
fetched twice a second you see a few MB/s between the two clients and both
directions on the router:

```minimega
minimega[sandwich]$ .annotate false .columns name,cpu,rx,tx vm top 5
name     | cpu   | rx   | tx
router   | 8.12  | 6.10 | 6.09
vm_left  | 14.60 | 0.21 | 6.08
vm_right | 11.03 | 6.07 | 0.20
```

The figures are measured from the host and differ from what the guests
report. Stopping and restarting the client (`cc process killall protonuke`
on `vm_right`, then the `cc background` line again) shows the rates fall and
rise. The next chapter captures this traffic as PCAP and netflow.

## Stop it

Background processes are tracked by PID; `killall` matches a substring of the
command line on every client the filter selects, so with the filter cleared
one command stops both ends:

```minimega
minimega[sandwich]$ cc process killall protonuke
minimega[sandwich]$ cc process list all
```

The queued commands stay in `cc commands` and would be re-sent to a client
that reconnects; `cc delete command all` clears them, and `clear cc` resets
the namespace's cc state entirely.

## What you built

- The sandwich with fixed client addresses, `10.0.0.10` and `10.0.1.10`.
- A protonuke server on `vm_left` and a protonuke client on `vm_right`,
  exchanging HTTP, HTTPS and SMTP traffic through the router.
- The habit of watching an experiment from the host: `cc process list`,
  `cc commands`, `vm top`.

## Where to read more

- [protonuke](../../articles/protonuke.md): every flag and what each
  protocol does on the client and server side.
- [Command and control](../../articles/cc.md#background-commands):
  background commands, [process control](../../articles/cc.md#process-control)
  and [sending files](../../articles/cc.md#sending-files).
- [VM lifecycle](../../articles/vm-lifecycle.md#vm-top) for `vm top`.
- Reference: [`cc`](../../reference/minimega.md#cc),
  [`vm top`](../../reference/minimega.md#vm-top).

## Script

```minimega title="11-traffic.mm"
--8<-- "training/miniclass/scripts/11-traffic.mm"
```

[Download this example](scripts/11-traffic.mm){ download="11-traffic.mm" }
