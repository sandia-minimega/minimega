# Testing with minitest

minitest is the functional test runner for minimega. It connects to a running
minimega, replays command files from a test directory, records what came back,
and compares that with a saved expectation. This page explains the test
layout, how to run the suite, and how to write a test that stays stable. You
need it when you change minimega and want to show that command behaviour did
not move, or when you contribute a new command. It assumes a source build; see
[Installation](installing.md).

## How it works

A test is an extensionless file of minimega commands, exactly the kind of
script you could `read` into minimega. minitest sends each line over the
command socket in `-base`, collects the responses, and writes the transcript to
`<test>.got`. It then compares that transcript byte for byte with
`<test>.want` and prints `got != want for <test>` when they differ. A test
with no `.want` file is reported as an error at the default log level. There
is no other output by default, and a mismatch does not change the exit
status, so automation has to inspect the output or diff the two files itself.

## Running the suite

Run minitest on an isolated Linux host as root, with minimega already running
from the same base directory (`/tmp/minimega` by default).

```bash
$ sudo ./bin/minitest -dir tests -run '^vm_uuid$'
$ diff -u tests/vm_uuid.want tests/vm_uuid.got
```

| Flag | Default | Purpose |
|---|---|---|
| `-dir` | `tests` | Directory containing the tests. Files run first, then each subdirectory. |
| `-base` | `/tmp/minimega` | minimega base directory; must match the daemon's `-base`. |
| `-run` | | Regular expression; only files whose name matches are run. |
| `-level`, `-logfile`, `-v` | `error`, none, `true` | Logging. `-level info` announces each test as it starts. |

Files ending in `.got` or `.want`, hidden files, and the special files
described below are never treated as tests. Without `-run`, every other file
under `tests/` runs, which launches VMs and needs the images described under
[Images](#images).

## Transcript format

`tests/vm_uuid` checks that minimega rejects duplicate UUIDs. This is an
excerpt; the file goes on to repeat the checks with `ns queueing true`:

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

Each input line is echoed with a `## ` prefix, followed by the rendered output
of the command. Errors are prefixed with `E: ` and sorted when a command
returns several. Blank lines are preserved, and comments are echoed but
produce no output.

## prolog, epilog, enter, and exit

A test directory can contain four special files that are never run as tests.

- `prolog` runs before every test in the directory. `tests/prolog` contains
  `.annotate false` and `clear history`, so that host names do not appear in
  the output and each test starts with an empty history.
- `epilog` runs after every test. `tests/epilog` contains `clear all`,
  `clear plumb`, and `clear pipe`, which resets the local instance without
  restarting it. Some state survives, notably the VM ID counter, so tests
  should not depend on it.
- `enter` runs once before the first test in the directory and `exit` once
  after the last. Their transcripts go to `enter.got` and `exit.got` but are
  not compared with anything. `tests/distributed/` uses them to build and tear
  down its environment.

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

Dynamically allocated values (VM IDs, UUIDs, tap names, uptime, host names)
change from run to run, so a test that prints all of `vm info` will never
match twice. Use `.columns` and `.filter` to keep only the fields the test is
about, as `tests/vm_lifecycle` and `tests/taps_lifecycle` do:

```minimega
vm launch kvm foo[0-2]
.columns name,state vm info
vm start foo0
.columns name,state vm info
```

The workflow for a new test is: write the command file, run it with `-run`,
read the `.got` transcript line by line, and only then copy it to `.want`.
Never accept a `.got` file you have not reviewed. Add a comment line at the top
of each block so the transcript explains itself, as `vm_uuid` does.

## Distributed tests

`tests/distributed/` exercises mesh and scheduling behaviour without a real
cluster. Its `enter` file builds a "miniception" environment in a namespace of
that name: a uminirouter container serving DHCP and DNS, and three KVM VMs
booted from the miniception kernel and initrd that receive a minimega binary
and the uminiccc filesystem over `cc`, start minimega, and are joined to the
mesh with `mesh dial`. The tests themselves run in the `distributed`
namespace; that directory's `prolog` enters it and its `epilog` sends
`clear all` across the mesh and clears the namespace. `exit` collects the
nested minimega logs with `cc recv` and kills the VMs. Run it on its own with
`-dir tests/distributed` when the images are available, and expect several
minutes of built-in waits.

## See also

- [Command line and scripting](cli.md) for `.columns`, `.filter`, and the other builtins
- [Building images with vmbetter](vmbetter.md)
- [Tools overview](../tools.md)
