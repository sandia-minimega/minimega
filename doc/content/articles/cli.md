# Command line and scripting

Every way of driving minimega sends the same commands and gets the same
responses. This includes the interactive prompt and the Python bindings. This
page covers the prompt itself, the builtins that change the output, variables,
and command files. It also covers how to run commands from a shell, how to
start host processes from minimega, and the command socket that programs use.
It assumes a running daemon. [Running minimega](running.md) explains how to
start one and attach to it. The complete command list is the
[reference](../reference/minimega.md).

## The prompt

An interactive minimega shows the active namespace in its prompt,
`minimega[minimega]$`. An attached minimega shows the socket that it is
connected to, `minimega:/tmp/minimega/minimega$`. The examples on this site
shorten both to `minimega$`.

The prompt has readline-style editing, history with the arrow keys, and Tab
completion. Tab completion first tries command words. Then it tries
environment variables (`$HOME`), files in the files directory, and paths on
the host.

Commands are words separated by whitespace. If an argument contains spaces,
quote it with `"` or `'`. A `\` escapes these characters only: `\r`, `\n`,
`\t`, `\\`, `\"`, `\'`, and `\#`. Everything from an
unquoted `#` to the end of the line is a comment. You can shorten a keyword to
any unambiguous prefix. For example, `vm config net` is `vm config networks`
and `vm config disk` is `vm config disks`. The output of `history` and `write`
records what you typed, so scripts written with prefixes keep working.

If you run most commands that set a value with no argument, they print the
current value:

```minimega
minimega$ vm config memory 4096
minimega$ vm config memory
4096
minimega$ clear vm config memory
```

`help` lists every command with a one-line summary. `help <command>` prints
the full text, including the exact patterns that the command accepts:

```minimega
minimega$ help vm config disks
```

## Output builtins

Commands that return tables print them as aligned columns with a `host` column
first. The builtins start with `.` and change what minimega shows and how it
shows it. The on/off builtins are `.csv`, `.json`, `.headers`, `.annotate`,
`.sort`, `.compress`, `.record`, and `.preprocess`. Each one takes `true` or
`false` to change the setting for the rest of the session. Each one also takes a
command to apply the change once:

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
| `.width [n]` | limit each column of a terminal table to `n` characters (the default is 50, and `0` shows all data) |
| `.preprocess` | expand `$VAR`, `file:`, `http://` and `tar:` arguments (default true) |
| `.record` | record the command in history (default true) |
| `.alias name=expansion`, `.unalias name` | shorthand for interactive use. `.alias` alone lists them |
| `.env name [value]` | print or set the daemon's environment variables |

Builtins stack, so a one-line query can combine several:

```minimega
minimega$ .annotate false .columns name,state,ip .filter state=running vm info
name | state   | ip
vm-0 | RUNNING | [10.0.0.2]
vm-1 | RUNNING | [10.0.0.3]
```

If you combine `.columns` and `.filter`, the order matters. `.columns` strips
every other column before the output of the inner command reaches `.filter`. If
you drop the filter column, put `.columns` first.

`.filter` matches case-insensitively and accepts four operators. The operators
`=` and `!=` test for equality. The operators `~` and `!~` test for a substring
match. If a column value is a list or map (rendered as `[...]` or `{...}`),
`.filter` matches it by substring, even with `=`. For example,
`.filter vlan=lan vm info` finds VMs on the `LAN` network. You can abbreviate
column names to an unambiguous prefix. `host` is not part of the table, but
`.filter` accepts it as a filter column.

`.compress` matters only for commands sent to several hosts. Instead of ten
identical `version` lines from `node[0-9]`, you get one line. The line has the
range of hosts that produced it as a prefix. `.compress` has no effect in JSON
mode. See [Cluster setup](cluster.md).

Pretty-printed tables limit each column to 50 characters by default. minimega
truncates longer values with `...`. This limit applies to terminal output
only. Piped output, csv mode, and JSON mode always
include all data. Use `.width` to view or change the limit. To show all data,
use `.width 0`:

```minimega
minimega$ .width 0 cc clients
```

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
empty value unsets it. Changes last for the daemon's lifetime. Programs that
the daemon starts can see the changes. The same expansion applies inside
command files. This is how one script serves several image directories or
hosts. `.preprocess
false <command>` runs a command with no expansion at all.

The preprocessor also understands three prefixes on arguments that name files.
`file:name` fetches `name` from the files directory of any node in the
cluster. The preprocessor replaces an `http://` or `https://` URL with a path
in the files directory, but it does not download the file in this release.
`tar:path` extracts an archive next to itself and substitutes the extracted
directory. See [File management](file.md).

## Names, ranges and wildcards

Commands that act on VMs take a target: one name or ID, a comma-separated
list, a range, or `all`:

```minimega
minimega$ vm start web0
minimega$ vm start web0,db0
minimega$ vm start web[0-4,7]
minimega$ vm start all
```

`vm launch kvm web[0-4]` creates five VMs with those names. The same range
syntax addresses hosts in `mesh send node[1-8] ...`. `all` applied to
`vm start` starts only VMs that are building or paused. If you name a VM
explicitly, `vm start` also restarts a VM that quit or errored. The
[`vm start`](../reference/minimega.md#vm) help has the details.

`clear` is a family of commands. `clear vm config` resets every configuration
field, or one field, as in `clear vm config disks`. `clear tap` removes host
taps. `clear log` resets logging. `clear history` empties the history. `clear
namespace <name>` destroys a namespace, and `clear all` runs every clear handler
at once. `help clear` lists them.

## Command files

A command file is a text file of commands, one per line, conventionally named
`.mm`. minimega skips blank lines, and `#` starts a comment. The history does
not keep comment lines, so a script that `write` saves loses its notes.

```minimega title="lab.mm"
--8<-- "articles/cli/lab.mm"
```

[Download this example](cli/lab.mm){ download="lab.mm" }

Run it with `read`:

```minimega
minimega$ read /srv/lab/lab.mm check
minimega$ read /srv/lab/lab.mm
```

`read <file> check` compiles every line without running anything. It stops at
the first line that it cannot parse. This catches typos before they cost you a
half-built experiment.

If you omit `check`, `read` runs the commands in order and stops at the first
invalid line. If a valid command returns an error, `read` keeps going, so
watch the output. `read` remembers the namespace that was active at the start
and runs every command there. `read` follows any `namespace` command that it
finds. A file cannot `read` another file.

Give `read` an absolute path. `read` resolves a relative path against the
working directory of the daemon, which is `/` under systemd. `read file:lab.mm`
fetches the script from the files directory instead.

`write <file>` saves the history to a file, which is the quickest way to turn
an interactive session into a script:

```minimega
minimega$ history
minimega$ write /srv/lab/lab.mm
```

`history` shows every valid command since the daemon started or since `clear
history`.
Commands that `read` runs from a file appear individually in the history. They
do not appear as the `read` command. Because of this, the written
script reproduces what actually happened. The history records a command with a
builtin prefix, such as `.columns`, as one line, with the prefix.
`.record false <command>` keeps a command out of the history.

## From the shell

`minimega -e` sends the rest of its command line to the running daemon. It
prints the rendered response on stdout and errors on stderr, and then exits:

```bash
sudo minimega -e vm info
sudo minimega -e .columns name,state vm info
sudo minimega -e .json true vm info | jq '.[0].Tabular'
sudo minimega -e read /srv/lab/lab.mm
sudo minimega -namespace lab -e vm start all
```

If arguments contain spaces or characters that the shell expands, quote them
for the shell. `-e` joins its arguments with the quoting that minimega
expects. Put loops and branching that a command file cannot express in a shell
script that calls `-e` repeatedly:

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

Each `-e` opens its own connection, so the overhead for each command is a few
milliseconds. For anything heavier, use the Python bindings.

## Host processes from minimega

`shell` runs a program on the host, waits for it, and returns its stdout as
the response and its stderr as the error:

```minimega
minimega$ shell ls /tmp/minimega/files
minimega$ shell ip link show mega_bridge
```

`background` starts the program and returns at once with an ID. minimega logs
the output of the program at `info` level as it happens and stores it for
later:

```minimega
minimega$ background tcpdump -i mega_tap0 -w /tmp/lan.pcap
Started background process with id 1
minimega$ background-status
host | ID | PID   | RUNNING | ERROR | TIME_START          | TIME_END | COMMAND
mm0  | 1  | 41927 | true    |       | Sep 17 10:04:12 UTC |          | /usr/bin/tcpdump -i mega_tap0 -w /tmp/lan.pcap
minimega$ background-output 1
minimega$ background-error 1
minimega$ clear background-status
```

`background-output` and `background-error` return the captured streams after
the process exits. `clear background-status` forgets finished processes. When
minimega records fifty entries, it also drops the oldest finished entries on
its own.

Both commands look up the program in `PATH` and pass the remaining words
directly as its arguments. There is no shell in between, so pipes,
redirections, and globs do not work. If you need them, wrap the command in
`bash -c "..."`. The program runs with the daemon's privileges, normally root.

`echo` prints its arguments after it strips comments and expands variables.
This is useful for progress markers in command files.

## The command socket

Everything above goes through one interface: a UNIX domain socket at
`<base>/minimega`, `/tmp/minimega/minimega` by default. A client writes one
JSON object for each request, with a `Command` string. For completions, the
client uses `Suggest`. To attach to a plumbing pipe, the client uses
`PlumbPipe`. The client reads back a stream of JSON response objects.

Each response carries the raw responses (`Resp`), the pre-rendered text
(`Rendered`), and `More`. `Resp` has one entry for each host. An entry has
`Header` and `Tabular` for tables and `Response` for plain text. It also has
`Error`. `Data` holds structured data from commands such as `vm info`, `host`,
and `cc`. `More` is `true` until the last response for the command arrives.
Long-running commands interleave progress messages, and minimega flags them
with `Status`.

Do not write that protocol yourself. `pkg/miniclient` in the repository is
the Go client that `minimega -e` and miniweb use. The generated Python module
wraps every command as a method. [Python bindings](python.md) shows it in use.
`minimega -cli` prints the complete command grammar as JSON for tools that
generate their own bindings.

## See also

- [Running minimega](running.md)
- [VM lifecycle](vm-lifecycle.md)
- [Python bindings](python.md)
- [Plumbing](plumbing.md) for `-pipe` and the `pipe` and `plumb` commands
- [Reference: builtins](../reference/minimega.md#builtins)
