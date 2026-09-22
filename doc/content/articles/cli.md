# Command line and scripting

Every way of driving minimega, from the interactive prompt to the Python
bindings, sends the same commands and gets the same responses. This page
covers the prompt itself, the builtins that reshape output, variables,
command files, running commands from a shell, launching host processes from
minimega, and the command socket that programs use. It assumes a running
daemon; [Running minimega](running.md) explains how to start one and attach to
it. The complete command list is the [reference](../reference/minimega.md).

## The prompt

An interactive minimega shows the active namespace in its prompt,
`minimega[minimega]$`, and an attached one shows the socket it is connected
to, `minimega:/tmp/minimega/minimega$`; the examples on this site shorten both
to `minimega$`. The prompt has readline-style editing, history with the arrow
keys, and Tab completion that first tries command words and then environment
variables (`$HOME`), files in the files directory, and paths on the host.

Commands are words separated by whitespace. Quote an argument with `"` or `'`
when it contains spaces, and escape a character with `\`. Everything from an
unquoted `#` to the end of the line is a comment. A keyword can be shortened
to any unambiguous prefix: `vm config net` is `vm config networks` and
`vm config disk` is `vm config disks`. The output of `history` and `write`
records what you typed, so scripts written with prefixes keep working.

Most commands that set a value print it when called without one:

```minimega
minimega$ vm config memory 4096
minimega$ vm config memory
4096
minimega$ clear vm config memory
```

`help` lists every command with a one-line summary and `help <command>` prints
the full text, including the exact patterns a command accepts:

```minimega
minimega$ help vm config disks
```

## Output builtins

Commands that return tables print them as aligned columns with a `host`
column first. A family of builtins, all starting with `.`, changes what is
shown and how. Each takes `true` or `false` to change the setting for the rest
of the session, or a command to apply once:

```minimega
minimega$ .json true              # every response from now on is JSON
minimega$ .json false
minimega$ .csv true vm info       # this response only, as CSV
```

| Builtin | Effect |
|---|---|
| `.columns a,b,c <command>` | keep only the named columns, in that order |
| `.filter col=val <command>` | keep only rows whose column matches |
| `.sort` | sort rows by the first column (default true) |
| `.annotate` | prepend the `host` column (default true) |
| `.headers` | print the header row (default true) |
| `.compress` | merge identical responses from several hosts into one line (default true) |
| `.csv`, `.json` | output tables as CSV or everything as JSON |
| `.preprocess` | expand `$VAR`, `file:`, `http://` and `tar:` arguments (default true) |
| `.record` | record the command in history (default true) |
| `.alias name=expansion`, `.unalias name` | shorthand for interactive use; `.alias` alone lists them |
| `.env name [value]` | print or set the daemon's environment variables |

Builtins stack, so a one-line query can combine several:

```minimega
minimega$ .annotate false .columns name,state,ip .filter state=running vm info
name | state   | ip
vm-0 | RUNNING | [10.0.0.2]
vm-1 | RUNNING | [10.0.0.3]
```

Order matters when you combine `.columns` and `.filter`: `.columns` strips
every other column before the inner command's output reaches `.filter`, so put
`.columns` first if the filter column is one you are dropping.

`.filter` matches case-insensitively and accepts four operators: `=` and `!=`
for equality, `~` and `!~` for substring match. A column whose value is a list
or map (rendered as `[...]` or `{...}`) is matched by substring even with `=`,
so `.filter vlan=lan vm info` finds VMs on the `LAN` network. Column names may
be abbreviated to an unambiguous prefix. `host` is accepted as a filter column
even though it is not part of the table.

`.compress` only matters for commands sent to several hosts: instead of ten
identical `version` lines from `node[0-9]` you get one, prefixed with the
range of hosts that produced it. It has no effect in JSON mode. See
[Cluster setup](cluster.md).

## Variables

minimega expands `$NAME` and `${NAME}` in command arguments from the daemon's
environment before running a command. `.env` reads and writes that
environment:

```minimega
minimega$ .env IMAGES /srv/images
minimega$ vm config disks $IMAGES/debian.qc2
minimega$ vm config disks
[/srv/images/debian.qc2]
minimega$ .env IMAGES ""
```

`.env` alone lists every variable, `.env NAME` prints one, and setting an
empty value unsets it. Changes last for the daemon's lifetime and are visible
to programs it starts. The same expansion applies inside command files, which
is how one script serves several image directories or hosts. `.preprocess
false <command>` runs a command with no expansion at all.

The preprocessor also understands three prefixes on arguments that name
files: `file:name` fetches `name` from the files directory of any node in the
cluster, `http://` and `https://` URLs are downloaded into the files directory
once, and `tar:path` extracts an archive next to itself and substitutes the
extracted directory. See [File management](file.md).

## Names, ranges and wildcards

Commands that act on VMs take a target: one name or ID, a comma-separated
list, a range, or `all`:

```minimega
minimega$ vm start web0
minimega$ vm start web0,db0
minimega$ vm start web[0-4,7]
minimega$ vm start all
```

`vm launch kvm web[0-4]` creates five VMs with those names, and the same
range syntax addresses hosts in `mesh send node[1-8] ...`. `all` applied to
`vm start` only starts VMs that are building or paused, while naming a VM
explicitly also restarts one that has quit or errored; the
[`vm start`](../reference/minimega.md#vm) help has the details.

`clear` is a family of its own: `clear vm config` resets every configuration
field (or one, as in `clear vm config disks`), `clear tap` removes host taps,
`clear log` resets logging, `clear history` empties the history, `clear
namespace <name>` destroys a namespace, and `clear all` runs every clear
handler at once. `help clear` lists them.

## Command files

A command file is a text file of commands, one per line, conventionally named
`.mm`. Blank lines are skipped and `#` starts a comment; comment lines are
kept in the history so a saved script keeps its notes.

```minimega title="lab.mm"
--8<-- "articles/cli/lab.mm"
```

[Download this example](cli/lab.mm){ download="lab.mm" }

Run it with `read`:

```minimega
minimega$ read /srv/lab/lab.mm check
minimega$ read /srv/lab/lab.mm
```

`read <file> check` compiles every line without running anything and stops at
the first line it cannot parse, which catches typos before they cost you a
half-built experiment. Without `check`, `read` runs the commands in order,
stops at the first invalid line, and keeps going when a valid command returns
an error, so watch the output. `read` remembers the namespace that was active
when it started and runs every command there, following any `namespace`
command it encounters; a file cannot `read` another file. Give `read` an
absolute path: a relative one is resolved against the daemon's working
directory, which is `/` under systemd. `read file:lab.mm` fetches the script
from the files directory instead.

`write <file>` saves the history to a file, which is the quickest way to turn
an interactive session into a script:

```minimega
minimega$ history
minimega$ write /srv/lab/lab.mm
```

`history` shows every valid command since the daemon started or since `clear
history`. Commands run from a file appear individually rather than as the
`read` that ran them, so the written script reproduces what actually
happened. A command with a builtin prefix such as `.columns` is recorded as
one line, prefix included, and `.record false <command>` keeps a command out
of the history altogether.

## From the shell

`minimega -e` sends the rest of its command line to the running daemon,
prints the rendered response on stdout and errors on stderr, and exits:

```bash
sudo minimega -e vm info
sudo minimega -e .columns name,state vm info
sudo minimega -e .json true vm info | jq '.[0].Tabular'
sudo minimega -e read /srv/lab/lab.mm
sudo minimega -namespace lab -e vm start all
```

Quote arguments for the shell when they contain spaces or characters the
shell would expand; `-e` joins its arguments with the quoting minimega
expects. Loops and branching that a command file cannot express belong in a
shell script that calls `-e` repeatedly:

```bash
#!/bin/bash
MM="sudo minimega -e"

$MM vm config memory 1024
for img in /srv/images/*.qc2; do
    name=$(basename "$img" .qc2)
    $MM vm config disks "$img"
    $MM vm launch kvm "$name"
done
$MM vm start all
```

Each `-e` opens its own connection, so per-command overhead is a few
milliseconds; for anything heavier use the Python bindings.

## Host processes from minimega

`shell` runs a program on the host, waits for it, and returns its stdout as
the response and its stderr as the error:

```minimega
minimega$ shell ls /tmp/minimega/files
minimega$ shell ip link show mega_bridge
```

`background` starts the program and returns at once with an ID; the program's
output is logged at `info` level as it happens and stored for later:

```minimega
minimega$ background tcpdump -i mega_tap0 -w /tmp/lan.pcap
Started background process with id 1
minimega$ background-status
ID | PID   | RUNNING | ERROR | TIME_START          | TIME_END | COMMAND
1  | 41927 | true    |       | Sep 17 10:04:12 UTC |          | /usr/bin/tcpdump -i mega_tap0 -w /tmp/lan.pcap
minimega$ background-output 1
minimega$ background-error 1
minimega$ clear background-status
```

`background-output` and `background-error` return the captured streams once
the process has exited. `clear background-status` forgets finished processes;
minimega also drops the oldest finished entries on its own once fifty are
recorded.

Both commands look the program up in `PATH` and pass the remaining words as
its arguments directly: there is no shell in between, so pipes, redirections
and globs do not work. Wrap the command in `bash -c "..."` when you need them.
The program runs with the daemon's privileges, normally root.

`echo` prints its arguments, after comment stripping and variable expansion,
which is useful for progress markers in command files.

## The command socket

Everything above goes through one interface: a UNIX domain socket at
`<base>/minimega`, `/tmp/minimega/minimega` by default. A client writes one
JSON object per request with a `Command` string (or `Suggest` for
completions, or `PlumbPipe` to attach to a plumbing pipe) and reads back a
stream of JSON response objects. Each response carries the raw responses
(`Resp`, one per host, with `Header` and `Tabular` for tables, `Response` for
plain text and `Error`), the pre-rendered text (`Rendered`), and `More`,
which is `true` until the last response for the command arrives; long-running
commands interleave progress messages flagged with `Status`.

Do not write that protocol yourself. `pkg/miniclient` in the repository is
the Go client that `minimega -e` and miniweb use, and the generated Python
module wraps every command as a method; [Python bindings](python.md) shows it
in use. `minimega -cli` prints the complete command grammar as JSON for
tools that generate their own bindings.

## See also

- [Running minimega](running.md)
- [VM lifecycle](vm-lifecycle.md)
- [Python bindings](python.md)
- [Plumbing](plumbing.md) for `-pipe` and the `pipe` and `plumb` commands
- [Reference: builtins](../reference/minimega.md#builtins)
