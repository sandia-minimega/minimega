# Chapter 9: Save, replay, and clean up

Everything that you typed so far lives in one minimega session. This chapter
makes that work durable and disposable.

First, you turn the history of the session into a command file and replay it.
Then you use configuration templates to keep the VM classes tidy. You
checkpoint the memory of a running VM and bring it back. You save a whole
namespace. Finally, you take the experiment apart cleanly, including the
things that a simple `vm kill` leaves behind. At the end the sandwich is
gone, and you can rebuild it in one command.

This chapter assumes the sandwich from [chapter 3](03-router-sandwich.md) and
the `cc` cleanup habits from [chapter 8](08-cc.md). The guides behind the
chapter are [Command line and scripting](../../articles/cli.md),
[Saving and restoring experiments](../../articles/save-restore.md), and
[Namespaces](../../articles/namespaces.md).

## From history to a script

minimega records every valid command since the daemon started or since the
last `clear history`. `history` prints the list, and `write <file>` saves it.
This is the quickest way to turn an afternoon at the prompt into a script.

Commands that came from a file appear individually, so a written script
reproduces what actually ran. minimega records each of these commands with a
`namespace` prefix, except the `namespace` command itself. The prefix names
the namespace that is active when `read` runs the line, for example
`namespace "sandwich"`. minimega records a
command with a builtin prefix such as `.columns` as one line, prefix included.
minimega does not record comment lines.

The history holds every chapter so far. Clear it before you start the work
that you want to keep:

```minimega
minimega$ clear history
minimega$ clear namespace sandwich
minimega$ namespace sandwich
...
minimega$ write /tmp/minimega/files/sandwich.mm
```

We recommend that you write scripts into the files directory.
`read file:sandwich.mm` fetches a script from there by name, on any host in
a cluster. Open the file and tidy it. A command file has one command on each
line. minimega skips blank lines, and `#` starts a comment. `shell sleep 10`
waits for a boot, and `echo` prints a progress marker. Every script in this
course uses these features:

```minimega
# the router first, so DHCP is serving before the clients ask
vm start router
shell sleep 10
echo starting the clients
vm start all
```

## Replaying

`read <file> check` compiles every line without running anything. It stops at
the first line that it cannot parse. This catches a typo before the typo costs
you a half-built experiment.

Without `check`, `read` runs the commands in order and stops at the first
invalid line. If a valid command returns an error, `read` keeps going, so
watch the output. Give `read` an absolute path. minimega resolves a relative
path against the working directory of the daemon, which is `/` under systemd.

```minimega
minimega$ read /tmp/minimega/files/sandwich.mm check
minimega$ read /tmp/minimega/files/sandwich.mm
```

VM names are unique within a namespace. If a `vm-left` already exists, a
script that launches `vm-left` fails. For this reason, every course script
starts with `clear namespace sandwich` and `namespace sandwich`. These
commands make the script runnable from any state. A file cannot `read`
another file, so each course script is one flat file.

From a host shell, `minimega -e read /tmp/minimega/files/sandwich.mm` does the
same. `-e` sends one command to the daemon and prints the response. A cron job
or a shell loop uses this form.

If a host program must keep running while a script continues, start it with
`background`. `background` is the host-side counterpart of `cc background`.
`background` returns an id. `background-status` lists what is running and
finished. `background-output <id>` shows what a finished process printed:

```minimega
minimega$ background tcpdump -i mega_tap0 -w /tmp/left.pcap
minimega$ background-status
```

Neither `shell` nor `background` runs a shell, so pipes and redirection need
`bash -c "..."`. The chapter 8 script does this to create a file.

## Templates

The `vm config` template is one description that you keep overwriting. If an
experiment has more than one class of VM, save each class under a name.
Restore the class before you launch it.

`vm config save` stores a copy of the current template. `vm config restore`
brings one back. `vm config restore` with no name lists them.
`vm launch kvm <name> <template>` launches from a saved template and does not
change the current one:

```minimega
minimega$ clear vm config
minimega$ vm config kernel miniccc.kernel
minimega$ vm config initrd miniccc.initrd
minimega$ vm config memory 3072
minimega$ vm config save client
minimega$ clear vm config
minimega$ vm config kernel minirouter.kernel
minimega$ vm config initrd minirouter.initrd
minimega$ vm config memory 3072
minimega$ vm config save minirouter
minimega$ vm config restore
client
minirouter
minimega$ vm config restore client
minimega$ vm config net net_left
minimega$ vm launch kvm vm-left
```

Templates hold no runtime state. They are the `vm config` lines of a class.
They live in memory, per namespace, until the daemon exits. To keep them, keep
them in a script.

## Checkpointing a running VM

`vm save <vm>` writes the memory and device state of a running KVM VM to a
`.state` file. It also writes a copy of each disk to `.hdd`, `.hdd.1`, and so
on. With these files, a new VM can later resume exactly where this VM was. The
sandwich VMs boot from a kernel and initrd and have no disk, so `vm save`
writes only the state file. Without a filename, the files go under `saved/`
in the files directory:

```minimega
minimega$ vm save vm-left
minimega$ vm save
id | name    | status | complete (%)
1  | vm-left | active | 42.17
```

After the disk copy is done, the command returns, and the memory save continues.
`vm save` alone shows the saves in progress. The `status` column ends as
`completed` or `failed`. Wait for `completed` before you kill the VM. If the
status is `failed`, do not use the state file. Saving pauses the VM, and the VM
stays `PAUSED` until you `vm start` it. A 3 GB VM writes up to 3 GB of state.
Before you save, make sure that the files directory has enough free space.

To resume the checkpoint, describe a VM the same way as the VM that you saved.
Point `vm config state` at the file. The memory, vCPUs, machine type, serial
ports, and interfaces must match, because QEMU restores the device state into
them.

The miniccc agent in the saved memory keeps the UUID of the old VM. minimega
accepts an agent only when a VM has its UUID. So read the UUID before you kill
the VM, and give it to the new VM with `vm config uuid`:

```minimega
minimega$ .annotate false .columns name,uuid .filter name=vm-left vm info
name    | uuid
vm-left | 5f2e8a1c-0b7d-4c3e-9a61-2d4f6e8b1c35
minimega$ vm kill vm-left
minimega$ vm flush vm-left
minimega$ vm config restore client
minimega$ vm config net net_left
minimega$ vm config uuid 5f2e8a1c-0b7d-4c3e-9a61-2d4f6e8b1c35
minimega$ vm config state saved/vm-left.state
minimega$ vm launch kvm vm-left
minimega$ vm start vm-left
minimega$ clear vm config state
minimega$ clear vm config uuid
```

Clear the two fields afterwards. Otherwise the next VM that you launch also
tries to resume from the file, and a template with a UUID launches only one
VM. The resumed guest keeps its address and its processes, but it was away
for a while. Its DHCP lease can expire while the guest is away, and its
miniccc agent reconnects on its own. You cannot save containers and Android
VMs this way.

## Saving the whole namespace

`ns save <name>` does the same for every VM in the namespace at once. It
pauses all of the VMs and saves each KVM VM into `saved/<name>/`. It also
writes a `launch.mm` file there that recreates the experiment. The script
first selects the namespace. For each VM, the script then clears the template
and writes the configuration of the VM. It points the state and disk fields at
the saved files and launches the VM. At the end, the script runs `vm start all` and a
stop-start pair. The stop-start pair lets QEMU finish loading:

```minimega
minimega$ ns save sandwich-day1
minimega$ ns save
completed | total
3         | 3
```

```text
/tmp/minimega/files/saved/sandwich-day1/router.state
/tmp/minimega/files/saved/sandwich-day1/vm-left.state
/tmp/minimega/files/saved/sandwich-day1/vm-right.state
/tmp/minimega/files/saved/sandwich-day1/launch.mm
```

The directory must not already exist. To restore, tear the namespace down
and `read` the generated script. A save does not contain host taps, VLAN
aliases, queued `cc` commands, captures, or the `router` descriptions that
minimega holds. The routers themselves resume with their configuration in
memory. If a sandwich boots in under a minute, the rebuild script is usually
the better checkpoint. If guests hold state that took hours to reach,
`ns save` is worth its cost.

## Cleaning up

Taking an experiment apart has layers. Each command removes one more layer:

```minimega
minimega$ vm kill all
minimega$ vm flush
```

`vm kill` terminates the guests (`QUIT`). `vm flush` removes `QUIT` and
`ERROR` VMs from `vm info`, deletes their instance directories, and frees
their names. Everything else that the sandwich created is still there. This
includes the VLAN aliases, the host tap from chapter 6, and the `cc` commands
and responses. One command removes all of it:

```minimega
minimega$ clear namespace sandwich
```

`clear namespace <name>` kills the VMs of the namespace, stops its captures,
and deletes its VLAN aliases, host taps, and mounts. It does the same on every
host in a cluster. If you give no name, `clear namespace` only switches you
back to the default namespace. `clear all` runs every `clear` handler on the
local instance. This is as close to a reset as you get without restarting.

If the daemon itself crashed and left QEMU processes, taps, or bridges
behind, start a new daemon with `-force` and run `nuke`. `nuke` kills the QEMU
and dnsmasq processes that minimega recorded under the base directory. It
deletes every `mega_tap*` interface on the host and the bridges that minimega
created. It removes the base directory and exits.

`nuke` is the reset button, not a routine command, and it does not find
everything. A host tap that you named yourself does not match `mega_tap*`.
minimega leaves alone a bridge that existed before minimega started.
`ovs-vsctl show` shows what is left, and
`ovs-vsctl del-port mega_bridge <tap>` or `ip link delete <tap>` removes it.

Before you leave a shared host, make sure that the output of `tap` and
`vm info` is empty. Also make sure that `dnsmasq` lists no instances and
`vlans` lists nothing that you still need.

## The script

The script rebuilds the sandwich from templates, writes the session to
`sandwich.mm`, checkpoints `vm-left`, and then tears everything down. It
starts with `clear history`, so the written file starts with the
`clear history` line, with its `namespace` prefix. Then the file holds the
commands that follow:

```minimega title="09-save-replay-cleanup.mm"
--8<-- "training/miniclass/scripts/09-save-replay-cleanup.mm"
```

[Download this example](scripts/09-save-replay-cleanup.mm){ download="09-save-replay-cleanup.mm" }

## What you built

- `sandwich.mm`, a script written from the history, that rebuilds the
  experiment with `read`.
- Two templates, `client` and `minirouter`, and a `vm-left.state`
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
