# Getting running

> The basics of running minimega

<a id="TOC_1."></a>

## Getting started

minimega can be built from latest source or deployed from a release package.
You can follow the instructions here to get up and running: [Installing minimega](../articles/installing.md)

- There are lots of ways to run and interface with minimega!

    -  on your local machine
    -  deployed over many nodes in a cluster

minimega is designed to be simple to deploy, and fully automated by running minimega scripts (.mm files).

A few optional command line switches are available, no config files!

This allows you to use minimega directly or programmatically. Whatever fits your needs.
More on that in the [Using minimega](../articles/usage.md) article.

<a id="TOC_2."></a>

## starting minimega

note: KVM requires special permissions and minimega must be run as root unless permissions are modified. All examples shown assume root permissions.

- Launch interactively

```text
bin/minimega
```

- Launch daemon (recommended)

```text
bin/minimega -nostdin &
```

- Launch daemon on a cluster (more on this later)

```text
bin/minimega -nostdin -context <contextName> -degree 3 &
```

<a id="TOC_3."></a>

## There are multiple ways to interact with the daemon

- You can attach to the daemon using minimega's attach flag

```text
bin/minimega -attach
```

- Detach using 'disconnect' or ctrl-d

```text
minimega$ disconnect
```

- Execute a single command with the -e flag

```text
bin/minimega -e vm config
```

- Command Port

    -  There is also a unix domain socket that accepts JSON encoded commands located at `<base>/minimega`.
    -  For more information, visit the
       [minimega documentation](https://sandia-minimega.github.io/minimega/).

<a id="TOC_4."></a>

## Stopping minimega

- minimega can be stopped gracefully using the 'quit' command

    -  note: when attached to the daemon, you will need to input quit twice

```text
minimega$ quit
sh$ bin/minimega -e quit
```

- If minimega proccesses are still running after the console has been killed, use pkill

```text
pkill minimega
```

- Finally, the minimega api documents a nuke command.

    -  After a crash, the VM state on the machine can be difficult to recover from. Nuke attempts to kill all instances of QEMU, remove all taps and bridges, and removes the temporary minimega state on the harddisk. This should be run with caution.

```minimega title="nuke.mm"
--8<-- "training/module02_content/nuke.mm"
```

[Download this example](module02_content/nuke.mm){ download="nuke.mm" }

<a id="TOC_5."></a>

## CLI

Now that you have minimega up and running let's get familiar with the CLI.

- Help - the most important command in minimega

```minimega title="help.mm"
--8<-- "training/module02_content/help.mm"
```

[Download this example](module02_content/help.mm){ download="help.mm" }

- Use `help <command>` for more information on particular commands
- Tab expansion is your friend!
- Tab expansion also works on local filesystem files

<a id="TOC_6."></a>

## Output Rendering

With minimega you can manipulate the way data is printed.

- output in `.csv` format

```text
.csv true
```

- output in JSON format

```text
.json true
```

- Set either option to `false` to turn it off

```text
.csv false
.json false
```

If you only want to see certain columns or vms you can tell minimega to only print those.

```text
.columns name,state,ip vm info
```

<a id="TOC_7."></a>

## Example Output

```text
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .csv true host
host,name,cpus,load,memused,memtotal,bandwidth,vms,vmsall
ubuntu,ubuntu,1,0.00 0.00 0.00,190 MB,2000 MB,0.0/0.0 (rx/tx MB/s),0,0
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .csv true
minimega:/tmp/minimega/minimega$ host
host,name,cpus,load,memused,memtotal,bandwidth,vms,vmsall
ubuntu,ubuntu,1,0.00 0.00 0.00,190 MB,2000 MB,0.0/0.0 (rx/tx MB/s),0,0
minimega:/tmp/minimega/minimega$ .csv false
minimega:/tmp/minimega/minimega$ host
host   | name   | cpus | load           | memused | memtotal | bandwidth            | vms | vmsall
ubuntu | ubuntu | 1    | 0.00 0.00 0.00 | 190 MB  | 2000 MB  | 0.0/0.0 (rx/tx MB/s) | 0   | 0
minimega:/tmp/minimega/minimega$ .json true host
[{"Host":"ubuntu","Response":"","Header":["name","cpus","load","memused","memtotal","bandwidth","vms","vmsall"],"Tabular":[["ubuntu","1","0.00 0.00 0.00","190 MB","2000 MB","0.0/0.0 (rx/tx MB/s)","0","0"]],"Error":""}]
minimega:/tmp/minimega/minimega$ .columns memtotal,bandwidth host
host   | memtotal | bandwidth
ubuntu | 2000 MB  | 0.0/0.0 (rx/tx MB/s)
```

<a id="TOC_8."></a>

## built-in commands

the `vm info` command is the primary way of seeing information about your VMs. However,
there are many columns of information that get printed by default, and looking through
all of that information can be cumbersome.

minimega has a variety of built-in commands that allow you to shape the output as you need.
Let's look at .columns, .filter, .annotate, and .sort and see how you can leverage these
commands individually and in conjunction with each other.

<a id="TOC_9."></a>

## .columns

The .columns command allows you to specify which columns you would like to see when running vm info

Column names are comma-separated.

For example, to display only the vm name and state, run:

```text
.columns name,state vm info
```

notice we appended vm info onto the command. .columns must be run in conjunction with
vm info or similar command.

<a id="TOC_10."></a>

## .filter

The .filter command filters tabular data based on the value in a particular column.
For example, to search for vms in a particular state use:

```text
.filter state=running vm info
```

Filters can also be inverted:

```text
.filter state!=running vm info
```

Filters are case insensitive and may be stacked:

```text
.filter state=RUNNING .filter vcpus=4 vm info
```

If the column value is a list or an object (i.e. "[...]", "{...}"), then

Substring matching can be specified explicity:

```text
.filter state~run vm info
.filter state!~run vm info
```

<a id="TOC_11."></a>

## .sort and .annotate

The .sort command allows you to set whether the returned tabular information is
sorted by the value in the first column. .sort does not need to be run in conjunction
with another command, and will affect all subsequent commands:

```text
.sort true
```

The .annnotate command will hide the host name in output when used.

```text
.annotate false
```

set to true to see the host name again.

<a id="TOC_12."></a>

## Stacking built-in commands

Built-in commands can be used in conjunction with eachother to further refine output.

For example, to isolate the name and state of the VM and filter by a running state, run

```text
.columns name,state .filter state=running vm info
```

However, these commands are not always interchangeable. For example, the following is
acceptable:

```text
.columns name,state .filter vcpus=4 vm info
```

While the following is not:

```text
.filter vcpus=4 .columns name,state vm info
```

This is because .columns strips all columns except for name and state from the
tabular data.

<a id="TOC_13."></a>

## .alias (recommended for interactive mode only)

```minimega title="dot.mm"
--8<-- "training/module02_content/dot.mm"
```

[Download this example](module02_content/dot.mm){ download="dot.mm" }

<a id="TOC_14."></a>

## Setting and Unsetting Variables

minimega uses variables and they can be set by calling the applicable command and setting the variable.
Notice how the 'Disk Paths' variable is not set:

```text
minimega$ vm config
VM configuration:
...
Disk Paths:         []
...
```

Let's set it now.

```text
minimega$ vm config disk mydisk.img
minimega$ vm config
VM configuration:
...
Disk Paths:         [mydisk.img]
...
```

<a id="TOC_15."></a>

##

The disk path is now set in the vm configuration. Unset using the clear command.

```text
minimega$ vm config disk
myhost: [mydisk.img]
minimega$ clear vm config disk
minimega$ vm config
VM configuration:
...
Disk Paths:         []
...
```

- The clear command clears the value for any setting in any api:

    -  clear vm config
    -  clear cc filter
    -  `clear router <vm>`
    -  clear tap
    -  see 'help clear' for more

<a id="TOC_16."></a>

## Next Up...

[Module 2.5: Better vmbetter](module02_5.md)
