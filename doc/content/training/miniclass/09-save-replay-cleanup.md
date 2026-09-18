# Chapter 9: Save, replay, and clean up

Everything you have typed so far lives in one minimega session. This chapter
makes it durable and disposable. You turn the session's history into a
command file and replay it, use configuration templates to keep the VM
classes tidy, checkpoint a running VM's memory and bring it back, save a
whole namespace, and then take the experiment apart cleanly, including the
things that a simple `vm kill` leaves behind. At the end the sandwich is
gone, and you can rebuild it in one command.

It assumes the sandwich from [chapter 3](03-router-sandwich.md) and the
`cc` cleanup habits from [chapter 8](08-cc.md). The guides behind the
chapter are [Command line and scripting](../../articles/cli.md),
[Saving and restoring experiments](../../articles/save-restore.md) and
[Namespaces](../../articles/namespaces.md).

## From history to a script

minimega records every valid command since the daemon started or since the
last `clear history`. `history` prints the list, and `write <file>` saves it,
which is the quickest way to turn an afternoon at the prompt into a script.
Commands that came from a file appear individually, so a written script
reproduces what actually ran, though each one is recorded with the
`namespace "sandwich"` prefix that `read` applied to it; a command with a
builtin prefix such as `.columns` is recorded as one line, prefix included;
comment lines are kept.

The history holds every chapter so far, so clear it before you start what
you intend to keep:

```minimega
minimega$ clear history
minimega$ clear namespace sandwich
minimega$ namespace sandwich
...
minimega$ write /tmp/minimega/files/sandwich.mm
```

Writing into the files directory is a habit worth keeping:
`read file:sandwich.mm` fetches a script from there by name, on any host in
a cluster. Open the file and tidy it. A command file is one command per
line; blank lines are skipped and `#` starts a comment. `shell sleep 10`
waits for a boot, and `echo` prints a progress marker. Every script in this
course was written that way:

```minimega
# the router first, so DHCP is serving before the clients ask
vm start router
shell sleep 10
echo starting the clients
vm start all
```

## Replaying

`read <file> check` compiles every line without running anything and stops
at the first one it cannot parse, which catches a typo before it costs you a
half-built experiment. Without `check`, `read` runs the commands in order,
stops at the first invalid line, and keeps going when a valid command
returns an error, so watch the output. Give it an absolute path: a relative
one is resolved against the daemon's working directory, which is `/` under
systemd.

```minimega
minimega$ read /tmp/minimega/files/sandwich.mm check
minimega$ read /tmp/minimega/files/sandwich.mm
```

VM names are unique within a namespace, so a script that launches `vm_left`
fails if a `vm_left` already exists. That is why every course script starts
with `clear namespace sandwich` and `namespace sandwich`: it makes the script
runnable from any state. A file cannot `read` another file, so a course
script is one flat file each. From a host shell,
`minimega -e read /tmp/minimega/files/sandwich.mm` does the same; `-e`
sends one command to the daemon and prints the response, which is what a
cron job or a shell loop wants.

Host programs that should keep running while a script continues belong in
`background`, the host-side counterpart of `cc background`. `background`
returns an id, `background-status` lists what is running and finished, and
`background-output <id>` shows what a finished process printed:

```minimega
minimega$ background tcpdump -i mega_tap0 -w /tmp/left.pcap
minimega$ background-status
```

Neither `shell` nor `background` runs a shell, so pipes and redirection need
`bash -c "..."`, as the chapter 8 script does to create a file.

## Templates

The `vm config` template is one description that you keep overwriting. When
an experiment has more than one class of VM, save each class under a name
and restore it before launching. `vm config save` stores a copy of the
current template, `vm config restore` brings one back, and
`vm config restore` with no name lists them.
`vm launch kvm <name> <template>` launches from a saved template without
touching the current one:

```minimega
minimega$ clear vm config
minimega$ vm config kernel miniccc.kernel
minimega$ vm config initrd miniccc.initrd
minimega$ vm config memory 2048
minimega$ vm config save client
minimega$ clear vm config
minimega$ vm config kernel minirouter.kernel
minimega$ vm config initrd minirouter.initrd
minimega$ vm config save minirouter
minimega$ vm config restore
client
minirouter
minimega$ vm config restore client
minimega$ vm config networks net_left
minimega$ vm launch kvm vm_left
```

Templates hold no runtime state; they are the `vm config` lines of a class,
and they live in memory, per namespace, until the daemon exits. To keep
them, keep them in a script.

## Checkpointing a running VM

`vm save <vm>` writes a running KVM VM's memory and device state to a
`.state` file, and a copy of each of its disks to `.hdd`, `.hdd.1`, and so
on, so a new VM can later resume exactly where this one was. The sandwich VMs boot from a
kernel and initrd and have no disk, so only the state file is written.
Without a filename the files go under `saved/` in the files directory:

```minimega
minimega$ vm save vm_left
minimega$ vm save
id | name    | status | complete (%)
1  | vm_left | active | 42.17
```

The command returns once the disk copy is done while the memory save
continues; `vm save` alone shows the saves in flight, and the `status`
column ends as `completed` or `failed`. Saving pauses the VM, and it stays
`PAUSED` until you `vm start` it. A 2 GB VM writes up to 2 GB of state, so
check the free space under the files directory first.

To resume the checkpoint, describe a VM the same way as the one you saved
and point `vm config state` at the file. The memory, vCPUs, machine type,
serial ports and interfaces must match, because QEMU restores the device
state into them:

```minimega
minimega$ vm kill vm_left
minimega$ vm flush vm_left
minimega$ vm config restore client
minimega$ vm config networks net_left
minimega$ vm config state saved/vm_left.state
minimega$ vm launch kvm vm_left
minimega$ vm start vm_left
minimega$ clear vm config state
```

Clear the field afterwards, or the next VM you launch tries to resume from
it too. The resumed guest keeps its address and its processes but has been
away for a while: its DHCP lease may have expired and its miniccc agent
reconnects on its own. Containers and Android VMs cannot be saved this way.

## Saving the whole namespace

`ns save <name>` does the same for every VM in the namespace at once. It
pauses them all, saves each KVM VM into `saved/<name>/`, and writes a
`launch.mm` there that recreates the experiment: it selects the namespace,
then for each VM clears the template, writes its configuration, points the
state and disk fields at the saved files, and launches it, ending with a
`vm start all` and a stop-start pair to let QEMU finish loading:

```minimega
minimega$ ns save sandwich-day1
minimega$ ns save
completed | total
3         | 3
```

```text
/tmp/minimega/files/saved/sandwich-day1/router.state
/tmp/minimega/files/saved/sandwich-day1/vm_left.state
/tmp/minimega/files/saved/sandwich-day1/vm_right.state
/tmp/minimega/files/saved/sandwich-day1/launch.mm
```

The directory must not already exist. To restore, tear the namespace down
and `read` the generated script. What a save does not contain matters as
much as what it does: host taps, VLAN aliases, queued `cc` commands and
captures are not saved, and the `router` descriptions minimega holds are
not either, although the routers themselves resume with their configuration
in memory. For a sandwich that boots in under a minute, the rebuild script
is usually the better checkpoint; `ns save` earns its cost when guests hold
state that took hours to reach.

## Cleaning up

Taking an experiment apart has layers, and each command reaches one further:

```minimega
minimega$ vm kill all
minimega$ vm flush
```

`vm kill` terminates the guests (`QUIT`), and `vm flush` removes `QUIT` and
`ERROR` VMs from `vm info`, deletes their instance directories and frees
their names. Everything else the sandwich created is still there: the VLAN
aliases, the host tap from chapter 6, the `cc` commands and responses. One
command removes all of it:

```minimega
minimega$ clear namespace sandwich
```

`clear namespace <name>` kills the namespace's VMs, stops its captures,
deletes its VLAN aliases, host taps and mounts, and does the same on every
host in a cluster. Without a name it only switches you back to the default
namespace. `clear all` runs every `clear` handler on the local instance,
which is as close to a reset as you get without restarting.

When the daemon itself has crashed and left QEMU processes, taps or bridges
behind, start a new one with `-force` and run `nuke`. It kills the QEMU and
dnsmasq processes recorded under the base directory, deletes every
`mega_tap*` interface on the host and the bridges minimega created, removes
the base directory, and exits. It is the reset button, not a routine
command, and it does not find everything: a host tap you named yourself
does not match `mega_tap*`, and a bridge that existed before minimega
started is left alone. `ovs-vsctl show` reveals what is left, and
`ovs-vsctl del-port mega_bridge <tap>` or `ip link delete <tap>` removes it.

Check before you leave a shared host: `tap` and `vm info` should be empty,
`dnsmasq` should list no instances, and `vlans` nothing you still need.

## The script

The script rebuilds the sandwich from templates, writes the session to
`sandwich.mm`, checkpoints `vm_left`, and then tears everything down. It
starts with `clear history`, so the written file contains exactly the
commands that follow:

```minimega title="09-save-replay-cleanup.mm"
--8<-- "training/miniclass/scripts/09-save-replay-cleanup.mm"
```

[Download this example](scripts/09-save-replay-cleanup.mm){ download="09-save-replay-cleanup.mm" }

## What you built

- `sandwich.mm`, a script written from the history, that rebuilds the
  experiment with `read`.
- Two templates, `client` and `minirouter`, and a `vm_left.state`
  checkpoint under `saved/`.
- A clean host: no namespace, no VMs, no taps.

## Where to read more

- [Command line and scripting](../../articles/cli.md): `read`, `write`,
  `history`, `shell`, `background`, `-e`, builtins and variables.
- [Saving and restoring experiments](../../articles/save-restore.md):
  what `vm save` and `ns save` capture and the exact restore procedure.
- [Namespaces](../../articles/namespaces.md): what `clear namespace`
  removes and what a namespace owns.
- [Running minimega](../../articles/running.md#stopping): stopping the
  daemon, `-force`, `-recover` and `nuke`.
- Reference: [`read`](../../reference/minimega.md#read),
  [`write`](../../reference/minimega.md#write),
  [`history`](../../reference/minimega.md#history),
  [`shell`](../../reference/minimega.md#shell),
  [`background`](../../reference/minimega.md#background),
  [`vm config`](../../reference/minimega.md#vm-config) (`save`, `restore`,
  `clone` and the `state` field),
  [`vm save`](../../reference/minimega.md#vm-save),
  [`ns`](../../reference/minimega.md#ns),
  [`clear namespace`](../../reference/minimega.md#clear-namespace),
  [`clear all`](../../reference/minimega.md#clear-all),
  [`nuke`](../../reference/minimega.md#nuke).

## Next

[Chapter 10: Troubleshooting walk](10-troubleshooting.md) breaks the
sandwich in the ways it usually breaks and shows where to look.
