# Chapter 11: Background traffic with protonuke

A network with nothing on it is a poor testbed. In this lab you fill the router
sandwich with plausible traffic. protonuke serves web and mail on `vm-left`. A
second protonuke on `vm-right` browses and sends mail to it through the router.
You watch the load from the minimega prompt. Along the way, you use `cc send` to
push a binary into a guest and `cc background` to start daemons. You use
`cc process list` and `vm top` to see what the daemons do.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) for the
sandwich itself and [Chapter 8](08-cc.md) for `cc`. Afterwards, read the
[protonuke](../../articles/protonuke.md) guide for the protocols and flags that
this lab does not use.

## Rebuild the sandwich with fixed addresses

The clients of chapter 3 take whatever lease the router gives them. This is fine
for a demo, but it is awkward for a VM that must name the other VM. Give each
client a fixed MAC address in its netspec and a static lease on the router. Then
`vm-left` is always `10.0.0.10`, and `vm-right` is always `10.0.1.10`:

```minimega
minimega$ clear namespace sandwich
minimega$ namespace sandwich
minimega[sandwich]$ clear cc responses
minimega[sandwich]$ vm config kernel miniccc.kernel
minimega[sandwich]$ vm config initrd miniccc.initrd
minimega[sandwich]$ vm config memory 3072
minimega[sandwich]$ vm config net net_left,00:00:00:00:01:01
minimega[sandwich]$ vm launch kvm vm-left
minimega[sandwich]$ vm config net net_right,00:00:00:00:02:01
minimega[sandwich]$ vm launch kvm vm-right
```

You launch and describe the router as in chapter 3, plus one `static` line per
subnet:

```minimega
minimega[sandwich]$ router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
minimega[sandwich]$ router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
```

The script at the end of the chapter has the complete sequence. Every later
lab in Part II starts the same way. After `vm start all` and a short wait, make
sure that the addresses are correct:

```minimega
minimega[sandwich]$ .annotate false .columns name,state,ip vm info
name     | state   | ip
router   | RUNNING | [10.0.0.1, 10.0.1.1]
vm-left  | RUNNING | [10.0.0.10]
vm-right | RUNNING | [10.0.1.10]
```

## Get protonuke into the guests

The client image that you built in chapter 2 contains miniccc but not protonuke.
The packages install protonuke as `/usr/bin/protonuke` on the host. The Docker
image has it in `/opt/minimega/bin`, and a source build has it in `bin/`.
`cc send` can deliver any file in the files directory of minimega,
`/tmp/minimega/files` by default. Copy protonuke there and send it to every
client. The `cp` command tries the package path first and falls back to the
`/opt/minimega/bin` path of the Docker image. If you built from source, change
the path. The command runs under `sh -c`, so the `||` reaches the shell:

```minimega
minimega[sandwich]$ shell sh -c "cp /usr/bin/protonuke /tmp/minimega/files/protonuke 2>/dev/null || cp /opt/minimega/bin/protonuke /tmp/minimega/files/protonuke"
minimega[sandwich]$ cc send protonuke
```

Sent files land in `/tmp/miniccc/files/` inside the guest with the same
permissions that they had on the host. miniccc looks in that directory for a
command that is not on the `PATH` of the guest. So from now on, the guests can
run `protonuke` by name. If you prefer to put it in the image, copy the binary
into the vmbetter overlay before you build. vmbetter copies the overlay into the
image root, so `misc/vmbetter_configs/miniccc_overlay/usr/local/bin/protonuke`
becomes `/usr/local/bin/protonuke` in the guest.

## Serve on vm-left

One protonuke process with `-serve` runs a server for every protocol that you
enable. Start it on `vm-left` as a background command, because it never returns:

```minimega
minimega[sandwich]$ cc filter name=vm-left
minimega[sandwich]$ cc background protonuke -serve -http -https -smtp
```

Leave SSH out of the server set. The client image already runs sshd on port 22,
and two servers cannot share the port. HTTP and HTTPS answer every `GET` with a
generated page and a random 3 MB image. SMTP accepts mail and drops it.

Check the server from the other side of the router. The client image has `curl`.
Leave the `http://` off the URL. minimega replaces any argument that starts with
`http://` with a local path. So `curl` never sees the address. If there is no
scheme, `curl` assumes HTTP. When `curl` runs, it is possible that protonuke
does not listen yet. So let `curl` retry a refused connection. Keep `-S`, so
that a failure prints an error instead of nothing:

```minimega
minimega[sandwich]$ cc filter name=vm-right
minimega[sandwich]$ cc exec curl -sS --retry 5 --retry-connrefused 10.0.0.10/
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ cc responses 6
```

The response is protonuke's generated page: a heading, the request URI, three
links back to the same host, a hit counter, and an `<img src=image.png>` tag.
`cc responses` prints every file under its path, `6/<uuid>/stdout:`. If `curl`
made a retry, the response also has a `stderr` file with the error of the
first attempt. Here `6` is
the command id that `cc commands` shows. The router commit used ids 1 to 3,
`cc send` used 4, and `cc background` used 5.

## Generate load from vm-right

Now run protonuke as a client on `vm-right` and point it at the server. Each
enabled protocol runs its own loop. The loop sleeps a random interval around
`-u` (1 second by default) and then performs one action against one target:

```minimega
minimega[sandwich]$ cc background protonuke -http -https -smtp -u 500ms 10.0.0.10
minimega[sandwich]$ clear cc filter
```

The target can also be a subnet, `10.0.0.0/24`. A subnet target spreads the same
rate over every address in it. The
[protonuke guide](../../articles/protonuke.md#how-it-works) explains the timing
flags (`-u`, `-s`, `-min`, `-max`) and the per-protocol behavior.

## Watch it

Three views, from the host, without touching the guests. `cc process list` shows
the processes that miniccc started in the background, with the PID that it
tracks them by:

```minimega
minimega[sandwich]$ .annotate false cc process list all
name     | uuid   | pid | command
vm-left  | <uuid> | 412 | protonuke -serve -http -https -smtp
vm-right | <uuid> | 415 | protonuke -http -https -smtp -u 500ms 10.0.0.10
```

`cc commands` lists the queued commands, how many clients answered, and whether
a command ran in the background:

```minimega
minimega[sandwich]$ .annotate false .columns id,command,responses,background cc commands
```

The third view is `vm top`. It samples host-side resource usage of every VM over
a window of seconds. `rx` and `tx` are MB/s as seen from the host. With a 3 MB
image that you fetch twice a second, you see a few MB/s between the two
clients. You see it in both directions on the router:

```minimega
minimega[sandwich]$ .annotate false .columns name,cpu,rx,tx vm top 5
name     | cpu   | rx   | tx
router   | 8.12  | 6.10 | 6.09
vm-left  | 14.60 | 0.21 | 6.08
vm-right | 11.03 | 6.07 | 0.20
```

minimega measures the figures from the host, so they differ from the figures
that the guests report. If you stop and restart the client
(`cc process killall protonuke` on `vm-right`, then the `cc background` line
again), the rates fall and rise. The next chapter captures this traffic as PCAP
and netflow.

## Stop it

miniccc tracks background processes by PID. `killall` matches a substring of the
command line on every client that the filter selects. With the filter cleared,
one command stops both ends:

```minimega
minimega[sandwich]$ cc process killall protonuke
minimega[sandwich]$ cc process list all
```

The queued commands stay in `cc commands`. minimega sends them again to a client
that reconnects. `cc delete command all` clears them, and `clear cc` resets all
cc state of the namespace.

## What you built

- The sandwich with fixed client addresses, `10.0.0.10` and `10.0.1.10`.
- A protonuke server on `vm-left` and a protonuke client on `vm-right`. They
  exchange HTTP, HTTPS, and SMTP traffic through the router.
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
