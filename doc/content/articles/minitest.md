# Testing with minitest

minitest is the functional test runner for minimega. It connects to a running
minimega, replays command files from a test directory, records what came back,
and compares that with a saved expectation. This page explains the test layout,
how to run the suite, and how to write a test that stays stable. If you change
minimega and want to show that the behavior of a command did not change, use
minitest. If you contribute a new command, also use minitest. This page assumes
a source build. See [Installation](installing.md).

## How it works

A test is an extensionless file of minimega commands. You can `read` the same
kind of script into minimega. minitest sends each line over the command socket
in `-base`, collects the responses, and writes the transcript to `<test>.got`.
It then compares that transcript byte for byte with `<test>.want`. If the two
differ, it prints `got != want for <test>`.

minitest reports a test with no `.want` file as an error at the default log
level. It prints no other output by default. A mismatch does not change the exit
status. Because of this, automation must inspect the output or diff the two
files itself.

## Running the suite

Run minitest as root on an isolated Linux host. Make sure that minimega is
already running from the same base directory (`/tmp/minimega` by default).

```bash
$ sudo ./bin/minitest -dir tests -run '^vm_uuid$'
$ diff -u tests/vm_uuid.want tests/vm_uuid.got
```

| Flag | Default | Purpose |
|---|---|---|
| `-dir` | `tests` | Directory containing the tests. Files run first, then each subdirectory. |
| `-base` | `/tmp/minimega` | minimega base directory. It must match the daemon's `-base`. |
| `-run` | | Regular expression. minitest runs only files whose name matches. |
| `-level`, `-logfile`, `-v` | `error`, none, `true` | Logging. `-level info` announces each test as it starts. |

minitest never treats files ending in `.got` or `.want`, hidden files, or the
special files described below as tests. Without `-run`, every other file under
`tests/` runs. This launches VMs and needs the images described under
[Images](#images).

## Transcript format

`tests/vm_uuid` makes sure that minimega rejects duplicate UUIDs. This is an
excerpt. The file goes on to repeat the checks with `ns queueing true`:

```minimega
# Try to launch two VMs with the same UUID
vm config uuid f3b8b039-2f26-43d0-908c-b0de030a44ed
vm launch kvm 2

# Launch VM with specified UUID
vm config uuid a5082fed-bc6f-4f77-8c1b-692ce5ef6302
vm launch kvm 1

# Try to launch another without clearing the UUID
vm launch kvm 1
```

The expectation, `tests/vm_uuid.want`, starts like this:

```text
## # Try to launch two VMs with the same UUID
## vm config uuid f3b8b039-2f26-43d0-908c-b0de030a44ed
## vm launch kvm 2
E: cannot launch multiple VMs with a pre-configured UUID

## # Launch VM with specified UUID
## vm config uuid a5082fed-bc6f-4f77-8c1b-692ce5ef6302
## vm launch kvm 1

## # Try to launch another without clearing the UUID
## vm launch kvm 1
E: vm already exists with UUID `a5082fed-bc6f-4f77-8c1b-692ce5ef6302`
```

In the transcript, each input line has a `## ` prefix and is followed by the
rendered output of the command. Errors have the prefix `E: `. When a command
returns several errors, the errors are sorted. Blank lines are preserved, and
comments are echoed but produce no output.

## prolog, epilog, enter, and exit

A test directory can contain four special files that are never run as tests.

- `prolog` runs before every test in the directory. `tests/prolog` contains
  `.annotate false` and `clear history`, so that host names do not appear in
  the output and each test starts with an empty history.
- `epilog` runs after every test. `tests/epilog` contains `clear all`,
  `clear plumb`, and `clear pipe`. These commands reset the local instance
  without restarting it. Some state survives, for example the VM ID counter, so
  tests must not depend on it.
- `enter` runs once before the first test in the directory. `exit` runs once
  after the last test. Their transcripts go to `enter.got` and `exit.got`.
  minitest does not compare them with anything. `tests/distributed/` uses them
  to build and tear down its environment.

## Images

Several tests launch VMs from files under a directory referred to as
`$images`. minimega expands environment variables in command arguments, so
set `images` in the environment of the minimega daemon, not of minitest:

```bash
$ sudo env images=/var/lib/images ./bin/minimega -nostdin &
```

The names the tests use are:

| File | Built from |
|---|---|
| `$images/minicccfs` | `misc/vmbetter_configs/miniccc_container.conf` (container filesystem) |
| `$images/minirouterfs` | `misc/vmbetter_configs/minirouter_container.conf` (container filesystem) |
| `$images/miniccc.kernel`, `$images/miniccc.initrd` | `misc/vmbetter_configs/miniccc.conf` |
| `$images/miniception.kernel`, `$images/miniception.initrd` | `misc/vmbetter_configs/miniception.conf` |
| `$images/uminicccfs.tar.gz`, `$images/uminirouterfs` | `misc/uminiccc/build.bash` and `misc/uminirouter/build.bash` (busybox-based container filesystems) |

See [Building images with vmbetter](vmbetter.md) for the vmbetter builds.

## Writing stable tests

Dynamically allocated values change from run to run. These values are VM IDs,
UUIDs, tap names, uptime, and host names. Because of this, a test that prints
all of `vm info` will never match twice. To keep only the fields that the test
is about, use `.columns` and `.filter`, as `tests/vm_lifecycle` and
`tests/taps_lifecycle` do:

```minimega
vm launch kvm foo[0-2]
.columns name,state vm info
vm start foo0
.columns name,state vm info
```

To write a new test, do these steps:

1. Write the command file.
2. Run it with `-run`.
3. Read the `.got` transcript line by line.
4. Only after you read it, copy it to `.want`.

Do not accept a `.got` file that you did not review. Add a comment line at the
top of each block so that the transcript explains itself, as `vm_uuid` does.

## Distributed tests

`tests/distributed/` exercises mesh and scheduling behavior without a real
cluster. Its `enter` file builds a "miniception" environment in a namespace of
that name. The environment has a uminirouter container that serves DHCP and DNS.
It also has three KVM VMs that boot from the miniception kernel and initrd. The
VMs receive a minimega binary and the uminiccc filesystem over `cc`. Then they
start minimega and are joined to the mesh with `mesh dial`.

The tests themselves run in the `distributed` namespace. The `prolog` of that
directory enters the namespace. Its `epilog` sends `clear all` across the mesh
and clears the namespace. `exit` collects the nested minimega logs with
`cc recv` and kills the VMs.

When the images are available, run it on its own with `-dir tests/distributed`.
Expect several minutes of built-in waits.

## See also

- [Command line and scripting](cli.md) for `.columns`, `.filter`, and the other builtins
- [Building images with vmbetter](vmbetter.md)
- [Tools overview](../tools.md)
