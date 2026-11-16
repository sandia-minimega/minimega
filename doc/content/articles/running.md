# Running minimega

This page covers the minimega daemon itself and explains how to start it as a
systemd service or by hand. It lists every startup flag and the `MM_*`
variables that set the defaults of the flags. It describes the base directory
where minimega keeps its state and how to talk to a running instance. It also
explains what `-recover` restores, how logging works, and how to stop minimega
cleanly. This page assumes that minimega is [installed](installing.md). If you
run the container image instead, [Running in Docker](docker.md) explains how the
same settings reach it there.

## Two ways to run it

minimega is a single process. It is the server. If it has a terminal, it is also
the command prompt. If you start minimega with no flags, it prints the copyright
banner, checks its external dependencies, and gives you a prompt.

When that prompt reaches end of file, minimega exits, so you cannot put it in
the background. If you start minimega with `-nostdin`, it becomes a daemon that
you reach through its command socket. The packaged systemd unit starts the
daemon in this mode for you.

## As a service

The packages install `misc/daemon/minimega.service` and `minimega.conf` under
`/opt/minimega/misc/daemon/` and link the configuration to
`/etc/minimega/minimega.conf`. The RPM also links the unit into
`/usr/lib/systemd/system/`, and the `.deb` copies the unit there (see
[Installing minimega](installing.md)).

```bash
sudo systemctl enable --now minimega     # start now and at boot
sudo systemctl status minimega
sudo systemctl restart minimega          # after editing minimega.conf
sudo systemctl stop minimega
```

The unit reads `/etc/minimega/minimega.conf` with `EnvironmentFile=`. It passes
each variable to an explicit flag on the `ExecStart` line. The file is therefore
the only place where you change how the service starts. The file is a symlink
into `/opt/minimega`. `readlink -f /etc/minimega/minimega.conf` shows which file
you edit. Use only plain `KEY=value` lines in the file.

| Variable | Flag | Value in the shipped file | Meaning |
|---|---|---|---|
| `MM_BASE` | `-base` | `/tmp/minimega` | base directory for state and the command socket |
| `MM_FILEPATH` | `-filepath` | `/tmp/minimega/files` | directory served by the file API |
| `MM_DAEMON` | `-nostdin` | `true` | run without a terminal |
| `MM_FORCE` | `-force` | `true` | if a stale socket exists, start anyway |
| `MM_RECOVER` | `-recover` | `false` | adopt VMs from a previous instance |
| `MM_DEGREE` | `-degree` | `0` | number of mesh peers to look for |
| `MM_MSA` | `-msa` | `10` | mesh state announcement period, in seconds |
| `MM_CONTEXT` | `-context` | `minimega` | mesh discovery context |
| `MM_PORT` | `-port` | `9000` | mesh TCP and UDP port |
| `MM_BROADCAST` | `-broadcast` | `255.255.255.255` | mesh discovery broadcast address |
| `MM_VLANRANGE` | `-vlanrange` | `101-4096` | VLANs handed out to namespaces |
| `MM_LOGLEVEL` | `-level` | `error` | log level |
| `MM_LOGFILE` | `-logfile` | `/var/log/minimega.log` | log file |
| `MM_CGROUP` | `-cgroup` | `/sys/fs/cgroup` | cgroup mount for containers |
| `MM_PANIC` | `-panic` | `false` | panic on quit to dump stack traces |
| `MM_ABSSNAPSHOT` | `-abssnapshot` | `false` | absolute backing-file paths in snapshots |

Only the legacy init script uses `MINIMEGA_DIR` from the same file.

The daemon also reads the same `MM_*` names from its own environment. If you
export them before `minimega` on the command line, the effect is the same as the
file. The precedence, from highest to lowest, is: a flag given on the command
line, an `MM_*` variable in the process environment, and the compiled-in
default. If neither `-base` nor `MM_BASE` is set, client invocations (`-e`,
`-attach`, `-pipe`) also read `MM_BASE` from `/etc/minimega/minimega.conf`. This
way, they find a daemon that runs from a non-default base directory. In that
case, they read no other key from the file.

Other things the unit does:

- `Restart=on-success`: systemd restarts the daemon after a clean exit, and
  `quit` produces a clean exit. If you want the daemon to stay down, use
  `systemctl stop minimega`. systemd does not restart the daemon after a crash
  (non-zero exit).
- `KillMode=control-group` and `TimeoutStopSec=5`: `systemctl stop` sends
  SIGTERM. minimega tears down its VMs, taps and dnsmasq instances. Five seconds
  later, systemd kills anything that is still alive in the cgroup.
- `LimitNOFILE=1024000` and `LimitNPROC=4096000`: these are raised limits. A
  host that runs hundreds of VMs then does not run out of file descriptors or
  processes. If you start minimega by hand, set the same limits with
  `ulimit -n`/`ulimit -u`.
- `Wants=openvswitch-switch.service`: pulls in Open vSwitch on Debian and
  Ubuntu. The RHEL unit is named `openvswitch.service`. Enable that unit
  yourself.
- After start, the unit gives the base directory and the command socket to the
  `minimega` group (`chgrp -R minimega`, `chmod g=u`, `g+s`). Any user in that
  group can then run `minimega -attach` or `minimega -e ...` without root. To
  join the group, run `sudo usermod -aG minimega "$USER"` and log in again.
- On stop, the unit removes the stale socket, `${MM_BASE}/minimega`.

`misc/daemon/minimega.init` is a SysV-style script for hosts without systemd.
`sudo /opt/minimega/misc/daemon/minimega.init install` links the script into
`/etc/init.d/minimega`. The script reads the same `minimega.conf`. It accepts
`start`, `stop`, `restart`, `status` and `uninstall`.

## By hand

```bash
sudo minimega                     # interactive, prompt in this terminal
sudo minimega -nostdin &          # daemon, reach it with -attach or -e
sudo minimega -base /srv/minimega -level info -logfile /var/log/minimega.log -nostdin &
```

Flags use Go syntax. `-name=value` works everywhere, and `-name value` works for
non-boolean flags. Booleans are `-flag` or `-flag=false`. Parsing stops at the
first argument that is not a flag, so put flags before the command that follows
`-e`. On a normal start, minimega ignores anything after the flags, and there is
no startup command file. To run a script at startup, start the daemon and then
run `minimega -e read /path/to/script.mm` (see
[Command line and scripting](cli.md)).

### Every flag

| Flag | Default | Meaning |
|---|---|---|
| `-base` | `/tmp/minimega` | base directory for state and the command socket |
| `-filepath` | `/tmp/minimega/files` | directory to serve files from. If you change only `-base`, the default becomes `<base>/files` |
| `-nostdin` | `false` | do not read stdin, so the process can run in the background |
| `-force` | `false` | if `<base>/minimega` exists, run anyway and remove the stale socket |
| `-recover` | `false` | if `-force` is not set, adopt VMs and taps that a previous instance left |
| `-degree` | `0` | number of mesh connections to maintain. `0` disables discovery |
| `-context` | `minimega` | discovery context. Only nodes with the same context connect |
| `-port` | `9000` | mesh port (TCP for connections, UDP for discovery) |
| `-broadcast` | `255.255.255.255` | broadcast address for discovery |
| `-msa` | `10` | mesh state announcement period in seconds |
| `-headnode` | | mesh node to send all logs to and fetch all files from |
| `-vlanrange` | `101-4096` | VLAN range allocated to namespaces |
| `-cgroup` | `/sys/fs/cgroup` | cgroup mount used for containers |
| `-hashfiles` | `false` | hash files served by the file API |
| `-abssnapshot` | `false` | write absolute paths to backing images into snapshots |
| `-panic` | `false` | panic instead of exiting on quit, for stack traces |
| `-level` | `error` | log level: `debug`, `info`, `warn`, `error`, `fatal` |
| `-logfile` | | file to log to. minimega creates the parent directories |
| `-v`, `-verbose` | `true` | log to stderr |
| `-e` | | run the rest of the command line on the running daemon and exit |
| `-attach` | | attach this terminal's prompt to the running daemon |
| `-namespace` | | prepend `namespace <name>` to every `-e` or `-attach` command |
| `-pipe` | | connect stdin and stdout to a named plumbing pipe |
| `-version` | | print the version and exit |
| `-cli` | | validate the command set, print it as JSON and exit |
| `-completion` | | print a completion script for `bash`, `zsh` or `fish` and exit |
| `-suggest` | | print completions for a partial command and exit (used by the completion scripts) |

`minimega -h` prints the same list with the compiled defaults, and
[`args`](../reference/minimega.md#args) at the prompt prints the values that the
running daemon uses.

When several hosts run minimega together, the mesh flags (`-degree`, `-context`,
`-port`, `-broadcast`, `-msa`, `-headnode`) matter. Otherwise, they do not
matter. [Cluster setup](cluster.md) explains the mesh flags. It also explains
the `deploy` command. That command copies the binary to other nodes and starts
it there with the same flags plus `-nostdin` and `-headnode=<this host>`.

## The base directory

minimega keeps everything that it knows about the running experiment under
`-base`:

| Entry | Contents |
|---|---|
| `minimega` | the command socket, a UNIX domain socket used by `-attach`, `-e`, miniweb and the Python bindings |
| `minimega.pid` | the daemon's PID |
| `files/` | the directory of the file API (`-filepath`). Relative image paths in `vm config` resolve here |
| `<id>/` | one directory per VM. It holds the configuration, name, state, taps, snapshot disks and QEMU sockets of the VM |
| `namespaces/<name>/<uuid>` | symlinks from each namespace to its VMs' directories |
| `bridges`, `taps`, `vlans` | bookkeeping for bridges, host taps and VLAN allocations |

Two daemons cannot share a base directory. If the socket exists, a second start
with the same `-base` exits with `minimega appears to already be running,
override with -force`. Make sure that the old instance is
gone. Then pass `-force`, which removes the stale socket. Do not
delete the base directory to remove the socket, because `files/` is in it by
default. If `/tmp` is a tmpfs, snapshot disks
for VMs in snapshot mode are under `<base>/<id>/`. For large experiments, move
`-base` to a real filesystem.

## Talking to a running daemon

```bash
sudo minimega -attach                 # prompt on the running daemon
sudo minimega -e vm info              # run one command, print, exit
sudo minimega -e .columns name,state vm info
sudo minimega -namespace lab -e vm info
sudo minimega -base /srv/minimega -e host
```

Both modes connect to `<base>/minimega`. Pass the same `-base` that the daemon
uses, or rely on `MM_BASE` in `/etc/minimega/minimega.conf`. An attached prompt
looks like `minimega:/tmp/minimega/minimega$` and warns you that `quit` stops
the daemon. To leave the prompt and keep the daemon running, type `disconnect`
or press Ctrl-D. `-e` prints the rendered response on stdout and any error on
stderr. Shell scripts need this behavior.

`-pipe <name>` connects stdin and stdout to a named pipe in the plumbing system
of the daemon. A host process can then take part in a pipeline (see
[Plumbing](plumbing.md)).

`-cli` prints the complete command set as JSON and exits without starting a
daemon. `pyapigen` uses it to generate the Python bindings. It is also the
easiest way for other tools to discover the command grammar.

## Recovering from a previous instance

If the base directory is left over from a crash and you want a clean slate,
start with `-force`. minimega then removes the stale socket and ignores the old
state.
If the previous daemon died but its QEMU processes still run and you want them
back, start with `-recover` instead. Recovery happens after the mesh comes up
and it:

- reloads VLAN allocations and host taps recorded under the base directory.
- walks `<base>/namespaces/<namespace>/<uuid>` and, for each VM, reads the saved
  configuration, name and taps.
- looks for a `qemu-system-x86` process with matching `-name` and `-uuid`
  arguments. It adopts that process and restores the state of the VM. A VM whose
  process is gone comes back as `QUIT`.
- reattaches the taps and bonds of the VM to their bridges.

Recovery handles only KVM VMs. It skips Android VMs with a warning and does not
adopt containers.

The flag help says that `-recover` applies only without `-force`. But the
daemon attempts recovery whenever `-recover` is set. The two flags differ only
in how they handle a stale socket. `-force -recover` removes the socket and then
adopts what it finds.

Do not enable `MM_RECOVER` permanently in `minimega.conf`. If the saved state is
inconsistent, recovery fails the start.

## Shell completion

```bash
minimega -completion bash | sudo tee /usr/share/bash-completion/completions/minimega >/dev/null
minimega -completion zsh  | sudo tee /usr/share/zsh/site-functions/_minimega >/dev/null
minimega -completion fish | sudo tee /usr/share/fish/vendor_completions.d/minimega.fish >/dev/null
```

The packages and the Docker image install the bash script already. The scripts
complete flags and, after `-e`, the minimega commands themselves. To do this,
they call `minimega -suggest` against the running daemon. For example,
`minimega -e vm sta` plus Tab gives `minimega -e vm start`. For the current
shell only, run `source <(minimega -completion bash)`.

## Logging

At startup `-level`, `-logfile` and `-v` set the level, an optional log file
and whether messages also go to stderr. At the prompt the
[`log` commands](../reference/minimega.md#log-level) change the same things
without a restart:

```minimega
minimega$ log level debug
minimega$ log file /var/log/minimega-debug.log
minimega$ log stderr false
minimega$ log filter meshage
minimega$ log ring 500
minimega$ log syslog remote udp loghost:514
minimega$ clear log filter
```

`log level` alone prints the current level. `log file` alone prints the file
that you set at the prompt. It prints nothing for a file that `-logfile` set.
`log filter <text>` drops every message that contains the text. `log ring
<size>` keeps the last messages in memory. `log ring` with no size prints them,
which is useful on a daemon that has no log file.

`log mesh
<node>` forwards logs to another node in the cluster (the `-headnode` flag sets
this at startup). `clear log` turns off the file, stderr, mesh, and syslog
outputs, including the outputs that the startup flags set. It also resets the
level to the startup value. `clear log file`, `clear log stderr`, and the other
forms act on one setting. Of these, only `clear log level` returns to the
startup value.

Under systemd, the log file is `/var/log/minimega.log`, and stderr output goes
to the journal (`journalctl -u minimega`). If something does not work as you
expect, we recommend that you set `MM_LOGLEVEL="info"` first.

## Stopping

To stop the daemon, use `quit` at the prompt or `minimega -e quit`. When you are
attached, enter `quit` twice. `quit 30` stops the daemon after thirty seconds,
which is useful to tell a whole mesh to exit. Ctrl-C on an interactive instance
and SIGTERM do the same.

Shutdown destroys every namespace. This kills all VMs, stops every dnsmasq
instance, and removes host taps and the bridges that minimega created. Shutdown
also removes the socket and PID file.

Under systemd, `quit` is a clean exit, and `Restart=on-success` starts a new
daemon. If you want the daemon to stay down, use `systemctl stop minimega`.

If a daemon crashes and leaves VMs, taps or bridges behind, start a new daemon
with `-force`. Then run [`nuke`](../reference/minimega.md#nuke). It kills the
QEMU and dnsmasq processes recorded under the base directory. It also kills any
containers and deletes every `mega_tap*` interface on the host. It removes the
bridges that minimega created (not ones that existed before it started). Then it
deletes the whole base directory and exits. At its default location, `files/`
is in the base directory, so `nuke` also deletes your images.

`clear all` is the gentler option for a daemon that is still healthy. It runs a
fixed list of commands: `clear deploy flags`, `dnsmasq kill all`,
`clear namespace all`, `clear vlans all`, `clear plumb`, `clear pipe` and
`clear history`. It leaves the process running.

## See also

- [Command line and scripting](cli.md)
- [Running in Docker](docker.md)
- [Cluster setup](cluster.md)
- [Namespaces](namespaces.md)
- [Security considerations](security.md)
- [Reference: host and other commands](../reference/minimega.md#host-and-other-commands)
