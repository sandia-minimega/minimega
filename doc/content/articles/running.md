# Running minimega

This page covers the minimega daemon itself: starting it as a systemd service
or by hand, every startup flag and the `MM_*` variables that set their
defaults, the base directory it keeps its state in, how to talk to a running
instance, what `-recover` restores, logging, and how to stop it cleanly. It
assumes minimega is [installed](installing.md). If you run the container image
instead, [Running in Docker](docker.md) explains how the same settings reach
it there.

## Two ways to run it

minimega is a single process that is both the server and, when it has a
terminal, the command prompt. Started with no flags it prints the copyright
banner, checks its external dependencies, and gives you a prompt; it exits when
that prompt reaches end of file, so it cannot simply be backgrounded. Started
with `-nostdin` it becomes a daemon that you reach through its command socket.
The packaged systemd unit does the second for you.

## As a service

The packages install `misc/daemon/minimega.service` and `minimega.conf` under
`/opt/minimega/misc/daemon/` and link the configuration to
`/etc/minimega/minimega.conf`. The RPM also links the unit into
`/usr/lib/systemd/system/`; on Debian and Ubuntu you link it yourself once
(see [Installing minimega](installing.md)).

```bash
sudo systemctl enable --now minimega     # start now and at boot
sudo systemctl status minimega
sudo systemctl restart minimega          # after editing minimega.conf
sudo systemctl stop minimega
```

The unit reads `/etc/minimega/minimega.conf` with `EnvironmentFile=` and
passes each variable to an explicit flag on the `ExecStart` line, so the file
is the only place to change how the service starts. It is a symlink into
`/opt/minimega`, so `readlink -f /etc/minimega/minimega.conf` tells you which
file you are really editing. Keep it to plain `KEY=value` lines.

| Variable | Flag | Value in the shipped file | Meaning |
|---|---|---|---|
| `MM_BASE` | `-base` | `/tmp/minimega` | base directory for state and the command socket |
| `MM_FILEPATH` | `-filepath` | `/tmp/minimega/files` | directory served by the file API |
| `MM_DAEMON` | `-nostdin` | `true` | run without a terminal |
| `MM_FORCE` | `-force` | `true` | start even if a stale socket exists |
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

`MINIMEGA_DIR` in the same file is used only by the legacy init script.

The daemon reads the same `MM_*` names from its own environment too, so
exporting them before `minimega` on the command line has the same effect as
the file. Precedence, highest first: a flag given on the command line, then an
`MM_*` variable in the process environment, then the compiled-in default.
Client invocations (`-e`, `-attach`, `-pipe`) additionally read `MM_BASE` from
`/etc/minimega/minimega.conf` when neither `-base` nor `MM_BASE` is set, so
they find a daemon that runs from a non-default base directory; no other key
is read from the file in that case.

Other things the unit does:

- `Restart=on-success`: systemd restarts the daemon after a clean exit, which
  is what `quit` produces. Use `systemctl stop minimega` when you want it to
  stay down. A crash (non-zero exit) is not restarted.
- `KillMode=control-group` and `TimeoutStopSec=5`: `systemctl stop` sends
  SIGTERM, minimega tears down its VMs, taps and dnsmasq instances, and
  anything still alive in the cgroup is killed five seconds later.
- `LimitNOFILE=1024000` and `LimitNPROC=4096000`: raised limits so a host
  running hundreds of VMs does not run out of file descriptors or processes.
  Set the same with `ulimit -n`/`ulimit -u` when you start minimega by hand.
- `Wants=openvswitch-switch.service`: pulls in Open vSwitch on Debian and
  Ubuntu. The RHEL unit is named `openvswitch.service`; enable it yourself.
- After start, the base directory and the command socket are handed to the
  `minimega` group (`chgrp -R minimega`, `chmod g=u`, `g+s`), so any user in
  that group can run `minimega -attach` or `minimega -e ...` without root:
  `sudo usermod -aG minimega "$USER"`, then log in again.
- On stop it removes the stale socket, `${MM_BASE}/minimega`.

`misc/daemon/minimega.init` is a SysV-style script for hosts without systemd.
`sudo /opt/minimega/misc/daemon/minimega.init install` links it into
`/etc/init.d/minimega` and reads the same `minimega.conf`; it accepts `start`,
`stop`, `restart`, `status` and `uninstall`.

## By hand

```bash
sudo minimega                     # interactive, prompt in this terminal
sudo minimega -nostdin &          # daemon, reach it with -attach or -e
sudo minimega -base /srv/minimega -level info -logfile /var/log/minimega.log -nostdin &
```

Flags use Go syntax: `-name=value` works everywhere, `-name value` for
non-boolean flags, and booleans are `-flag` or `-flag=false`. Parsing stops at
the first argument that is not a flag, so put flags before the command that
follows `-e`. Anything after the flags is ignored on a normal start; there is
no startup command file. To run a script at startup, start the daemon and then
`minimega -e read /path/to/script.mm` (see
[Command line and scripting](cli.md)).

### Every flag

| Flag | Default | Meaning |
|---|---|---|
| `-base` | `/tmp/minimega` | base directory for state and the command socket |
| `-filepath` | `/tmp/minimega/files` | directory to serve files from; follows `-base` to `<base>/files` when you change only `-base` |
| `-nostdin` | `false` | do not read stdin, so the process can run in the background |
| `-force` | `false` | run even if `<base>/minimega` exists, removing the stale socket |
| `-recover` | `false` | adopt VMs and taps left by a previous instance (only when `-force` is not set) |
| `-degree` | `0` | number of mesh connections to maintain; `0` disables discovery |
| `-context` | `minimega` | discovery context; only nodes with the same context connect |
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
| `-logfile` | | file to log to; parent directories are created |
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
[`args`](../reference/minimega.md#args) at the prompt prints the values the
running daemon is using.

The mesh flags (`-degree`, `-context`, `-port`, `-broadcast`, `-msa`,
`-headnode`) matter only when several hosts run minimega together;
[Cluster setup](cluster.md) explains them and the `deploy` command that copies
the binary to other nodes and starts it there with the same flags plus
`-nostdin` and `-headnode=<this host>`.

## The base directory

Everything minimega knows about the running experiment lives under `-base`:

| Entry | Contents |
|---|---|
| `minimega` | the command socket, a UNIX domain socket used by `-attach`, `-e`, miniweb and the Python bindings |
| `minimega.pid` | the daemon's PID |
| `files/` | the file API's directory (`-filepath`); relative image paths in `vm config` resolve here |
| `<id>/` | one directory per VM, holding its config, name, state, taps, snapshot disks and QEMU sockets |
| `namespaces/<name>/<uuid>` | symlinks from each namespace to its VMs' directories |
| `bridges`, `taps`, `vlans` | bookkeeping for bridges, host taps and VLAN allocations |

Two daemons cannot share a base directory. A second start with the same
`-base` exits with `minimega appears to already be running, override with
-force` if the socket exists. Delete the directory, or pass `-force`, when you
know the old instance is gone. If `/tmp` is a tmpfs, remember that snapshot
disks for VMs in snapshot mode live under `<base>/<id>/`; move `-base` to a
real filesystem for large experiments.

## Talking to a running daemon

```bash
sudo minimega -attach                 # prompt on the running daemon
sudo minimega -e vm info              # run one command, print, exit
sudo minimega -e .columns name,state vm info
sudo minimega -namespace lab -e vm info
sudo minimega -base /srv/minimega -e host
```

Both modes connect to `<base>/minimega`, so pass the same `-base` the daemon
uses (or rely on `MM_BASE` in `/etc/minimega/minimega.conf`). An attached
prompt looks like `minimega:/tmp/minimega/minimega$` and warns you that `quit`
stops the daemon; type `disconnect` or press Ctrl-D to leave the prompt and
keep the daemon running. `-e` prints the rendered response on stdout and any
error on stderr, which is what shell scripts want.

`-pipe <name>` connects stdin and stdout to a named pipe in the daemon's
plumbing system, so a host process can take part in a pipeline; see
[Plumbing](plumbing.md).

`-cli` prints the complete command set as JSON and exits without starting a
daemon. `pyapigen` uses it to generate the Python bindings and it is the
easiest way for other tools to discover the command grammar.

## Recovering from a previous instance

Start with `-force` when the base directory is left over from a crash and you
want a clean slate; the stale socket is removed and the old state is ignored.
Start with `-recover` instead when the previous daemon died but its QEMU
processes are still running and you want them back. Recovery happens after the
mesh comes up and it:

- reloads VLAN allocations and host taps recorded under the base directory;
- walks `<base>/namespaces/<namespace>/<uuid>` and, for each VM, reads its
  saved config, name and taps;
- looks for a `qemu-system-x86` process with matching `-name` and `-uuid`
  arguments and adopts it, restoring the VM's state; a VM whose process is gone
  comes back as `QUIT`;
- reattaches the VM's taps and bonds to their bridges.

Only KVM VMs are recovered. Android VMs are skipped with a warning and
containers are not adopted. The flag help describes `-recover` as applying
only when `-force` is not set, but the daemon attempts recovery whenever
`-recover` is set; the two flags differ only in how a stale socket is
handled, so `-force -recover` removes the socket and then adopts what it
finds. Recovery fails the start if the saved state is inconsistent, so do not
enable `MM_RECOVER` permanently in `minimega.conf`.

## Shell completion

```bash
minimega -completion bash | sudo tee /usr/share/bash-completion/completions/minimega >/dev/null
minimega -completion zsh  | sudo tee /usr/share/zsh/site-functions/_minimega >/dev/null
minimega -completion fish | sudo tee /usr/share/fish/vendor_completions.d/minimega.fish >/dev/null
```

The packages and the Docker image install the bash script already. The
scripts complete flags and, after `-e`, the minimega commands themselves by
calling `minimega -suggest` against the running daemon: `minimega -e vm sta`
plus Tab gives `minimega -e vm start`. For the current shell only,
`source <(minimega -completion bash)`.

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

`log level` alone prints the current level; `log file` alone prints the file
in use. `log filter <text>` drops every message containing the text.
`log ring <size>` keeps the last messages in memory, and `log ring` with no
size prints them, which is handy on a daemon that has no log file. `log mesh
<node>` forwards logs to another node in the cluster (the `-headnode` flag
sets this at startup). `clear log` resets all of it to the startup flags;
`clear log file`, `clear log level` and the other forms reset one setting.

Under systemd the log file is `/var/log/minimega.log` and stderr output goes
to the journal (`journalctl -u minimega`), so `MM_LOGLEVEL="info"` is a useful
first step when something misbehaves.

## Stopping

`quit` at the prompt (twice, when attached) or `minimega -e quit` stops the
daemon; `quit 30` stops it after thirty seconds, which is useful for telling a
whole mesh to exit. Ctrl-C on an interactive instance and SIGTERM do the same.
Shutdown destroys every namespace, which kills all VMs, stops every dnsmasq
instance, removes host taps and the bridges minimega created, and removes the
socket and PID file. Under systemd, `quit` is a clean exit and
`Restart=on-success` starts a new daemon; use `systemctl stop minimega` when
you mean it.

When a daemon has crashed and left VMs, taps or bridges behind, start a new
one with `-force` and run [`nuke`](../reference/minimega.md#nuke). It kills the
QEMU and dnsmasq processes recorded under the base directory, kills any
containers, deletes every `mega_tap*` interface on the host, removes the
bridges minimega created (not ones that existed before it started), deletes
the whole base directory, and then exits. `clear all` is the gentler option
for a daemon that is still healthy: it runs a fixed list of commands
(`clear deploy flags`, `dnsmasq kill all`, `clear namespace all`,
`clear vlans all`, `clear plumb`, `clear pipe`, `clear history`) and leaves
the process running.

## See also

- [Command line and scripting](cli.md)
- [Running in Docker](docker.md)
- [Cluster setup](cluster.md)
- [Namespaces](namespaces.md)
- [Security considerations](security.md)
- [Reference: host and other commands](../reference/minimega.md#host-and-other-commands)
