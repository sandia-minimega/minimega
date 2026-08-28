---
name: minimega
description: 'Guide for configuring, operating, and integrating minimega to manage KVM virtual machines, Android emulators, Linux containers, networks, namespaces, clusters, files, and miniccc command-and-control. Use for startup flags, environment variables, command files, Docker or systemd configuration, CLI commands, command socket or Python API usage, miniweb, phēnix or FIREWHEEL integration, VM lifecycle, VLANs, captures, or distributed emulation troubleshooting.'
license: GPL-3.0-only (see LICENSE)
---

# minimega operations and integration

minimega manages KVM virtual machines, Android emulators, and Linux containers
on one host or across a mesh-connected cluster. Read
[AGENTS.md](../../AGENTS.md) before changing this repository or writing tests;
this skill focuses on using minimega and preserving its operational contracts.

## When to use this skill

- Run, attach to, automate, or troubleshoot a minimega instance.
- Configure, launch, start, inspect, stop, or remove supported VM types.
- Work with namespaces, scheduling, VLANs, taps, bridges, captures, or clusters.
- Use miniccc command-and-control, file transfer, or miniweb.
- Integrate through the command socket, Go client, generated Python module, or
  higher-level orchestrators such as phēnix and FIREWHEEL.
- Change code that defines minimega commands or externally visible behavior.

## Execution modes

Full operation requires Linux, root or equivalent device/network permissions,
KVM, libpcap, and the external tools needed by the selected feature.

```bash
minimega                         # interactive instance
minimega -nostdin &              # daemonized instance
minimega -attach                 # attach to existing instance
minimega -e vm info              # execute one command
minimega -base /path -e vm info  # target a non-default base
```

`-attach` and `-e` connect to `<base>/minimega`, under `/tmp/minimega` by
default. For scripts and agents, prefer one-shot `-e` commands over an
interactive prompt.

The repository container provides an `mm` wrapper:

```bash
docker exec minimega mm vm info
```

Omit `-it` for non-interactive automation. The normal container deployment is
privileged and mounts host devices; follow `../../docker/README.md`.

## Configuration and CLI help

Read [AGENTS.md](../../AGENTS.md) before modifying configuration, startup flags,
deployment defaults, the Docker wrapper, service units, or configuration
methods. Keep every launcher aligned with the flags registered in
[`cmd/minimega/main.go`](../../cmd/minimega/main.go) and
[`pkg/minilog/minilog.go`](../../pkg/minilog/minilog.go).

### Native process

The native binary does not load YAML, JSON, or TOML configuration files, and
does not treat `.env` as a dotenv file. Local client modes have one exception:
they read `MM_BASE` from `/etc/minimega/minimega.conf` when needed to find the
daemon socket.
Startup values come from compiled defaults, supported `MM_*` process environment
variables, and command-line flags:

| Purpose | Flags |
|---|---|
| Runtime state | `-base`, `-filepath`, `-cgroup` |
| Mesh and VLANs | `-context`, `-degree`, `-msa`, `-broadcast`, `-port`, `-vlanrange`, `-headnode` |
| Lifecycle | `-nostdin`, `-force`, `-recover`, `-panic`, `-hashfiles`, `-abssnapshot` |
| Logging | `-level`, `-logfile`, `-v`, `-verbose` |
| Alternate and client modes | `-e`, `-attach`, `-namespace`, `-pipe`, `-version`, `-cli`, `-completion`, `-suggest` |

Use Go flag syntax: `-name=value` works for every flag, `-name value` works for
non-boolean flags, and booleans use `-flag` or `-flag=false`. Parsing stops at
the first non-flag argument or `--`; when a flag is repeated, the last parsed
value wins. Put `-base` and `-namespace` before the command following `-e`.
`-namespace` affects only `-e` and `-attach`. When `-recover=true`, minimega
attempts recovery after mesh initialization even if `-force=true`; `-force`
only takes precedence when removing an existing command socket. When `-base`
changes while `-filepath` retains its default value, minimega rebases the file
path to `<base>/files`.

There is no automatic startup command file. Despite the `[file]...` text in the
startup usage line, normal server startup ignores positional arguments. Apply
runtime configuration explicitly after startup:

```bash
minimega -e read /path/to/experiment.mm
minimega -e read /path/to/experiment.mm check
```

`read <file>` executes commands in order, stops at invalid syntax, does not stop
when a valid command returns an error, and rejects nested `read` commands. The
`check` form validates syntax without executing commands.

The process recognizes these environment-backed flag defaults:
`MM_BASE`, `MM_DEGREE`, `MM_MSA`, `MM_BROADCAST`, `MM_VLANRANGE`, `MM_PORT`,
`MM_FORCE`, `MM_RECOVER`, `MM_DAEMON`, `MM_CONTEXT`, `MM_FILEPATH`,
`MM_LOGLEVEL`, `MM_LOGFILE`, `MM_CGROUP`, `MM_PANIC`, and `MM_ABSSNAPSHOT`.
Explicit command-line flags take precedence. For `-e`, `-attach`, and `-pipe`,
when neither `MM_BASE` nor `-base` is set, minimega also reads `MM_BASE` from
`/etc/minimega/minimega.conf`; other configuration-file values are not read in
these client modes.

The process explicitly honors `GOMAXPROCS`; inherited environment variables are
also expanded from `$NAME` or `${NAME}` in runtime command string and list
arguments. The `.env` mentioned in command help is a runtime minicli command,
not a file; minimega does not discover or read dotenv files. Use `.env` to
inspect or change the daemon's environment. An empty value unsets a variable,
and changes last only for that process lifetime. `PATH` and tool-specific
variables still affect discovery and execution of external helpers.

### Effective precedence

- **Native:** explicit CLI flags, then supported process environment variables,
  then the compiled default. Repeated CLI flags use the last value.
- **Runtime scripts:** commands execute sequentially, so later commands can
  replace earlier runtime state.
- **Docker:** for generated flags, a non-empty container environment value wins
  over `/etc/default/minimega`, which wins over the wrapper default. A duplicate
  flag in `MM_APPEND` wins because it is appended last; explicit flags then win
  over native defaults.
- **systemd:** systemd resolves `EnvironmentFile=` and substitutes those values
  into explicit `ExecStart` flags; those flags win over native defaults.

### Docker container

The image runs [`docker/start-minimega.sh`](../../docker/start-minimega.sh) as
its default command. The script starts Open vSwitch, waits for it, starts
miniweb, then runs minimega with `-nostdin` and generated flags.

- `MM_BASE`, `MM_FILEPATH`, `MM_BROADCAST`, `MM_VLANRANGE`, `MM_PORT`,
  `MM_DEGREE`, `MM_CONTEXT`, `MM_LOGLEVEL`, `MM_LOGFILE`, `MM_FORCE`,
  `MM_RECOVER`, `MM_CGROUP`, and `MM_ABSSNAPSHOT` map to same-purpose minimega
  flags.
- `MINIWEB_ROOT`, `MINIWEB_HOST`, and `MINIWEB_PORT` configure miniweb.
- `OVS_APPEND` adds raw `ovs-ctl start` arguments. `OVS_HOST_IFACE` uses
  `<bridge>:<port>[,<port>...]`.
- `MM_APPEND` adds otherwise unsupported minimega flags, such as
  `-msa=20 -hashfiles`.

Set values with Docker `-e`, Compose `environment` or `env_file`, or a file
mounted at `/etc/default/minimega`. Existing **non-empty** container environment
values take precedence; an unset or empty value permits the file value, then the
script default. The file parser accepts simple `KEY=value` lines, strips only
surrounding double quotes, and does not source shell expressions.
This container-only file is distinct from the host service's
`/etc/minimega/minimega.conf`.

The wrapper expands scalar configuration values without shell quoting, so paths
and other single values cannot safely contain whitespace, glob characters, or
shell syntax. `MM_APPEND` and `OVS_APPEND` are intentionally split on whitespace:
spaces separate shell-safe argument tokens but cannot be preserved inside one
argument. In `MM_APPEND`, a positional token stops parsing later tokens. Because
it appears last, avoid accidentally duplicating generated flags unless an
override is intentional.

Docker defaults intentionally differ from native defaults, including
`MM_DEGREE=1`, `MM_LOGLEVEL=info`, `MM_LOGFILE=/var/log/minimega.log`, and
`MM_FORCE=true`. Additional container gotchas:

- Changing `MM_PORT` or `MINIWEB_PORT` also requires matching published ports.
- Changing `MM_BASE` breaks the `mm` wrapper and default Compose health check,
  which connect through `/tmp/minimega`; use `minimega -base=<path> -e ...` and
  update the health check.
- Supplying a command after the image name replaces the Dockerfile `CMD` and
  skips the Open vSwitch, miniweb, and minimega startup wrapper.

### systemd service

The packaged [`minimega.service`](../../misc/daemon/minimega.service) reads
`/etc/minimega/minimega.conf` through `EnvironmentFile=` and maps its `MM_*`
values to explicit startup flags; `MM_APPEND` is not supported. Keep the file to
literal `KEY=value` assignments and check `readlink -f` before editing because a
package may link it into `/opt/minimega`.

Restart the service after changing the environment file. After changing the unit
or a drop-in, run `systemctl daemon-reload` before restarting; replacing
`ExecStart` requires first clearing it with an empty `ExecStart=`. Because the
unit uses `Restart=on-success`, use `systemctl stop minimega` rather than runtime
`quit` when the service must remain stopped.

### Getting help

```bash
minimega -h                  # startup flags and compiled defaults
minimega -e help             # runtime command summary
minimega -e help vm config   # exact runtime command forms
minimega -cli                # machine-readable runtime CLI as JSON, then exit
minimega -completion bash    # generate bash, zsh, or fish completion
```

At an interactive or attached prompt, use `help` and `help <command>`. Pass the
correct `-base=<path>` before `-e` when the daemon uses a non-default base.
`-suggest` is an internal completion-script interface, not normal operator help.

## Core concepts

### Base directory, file path, and context

- `-base` controls runtime state and the local command socket.
- `-filepath` controls files served by iomeshage; relative VM image paths resolve
  beneath it. Use absolute paths when the file is elsewhere.
- `-context` separates mesh discovery groups. `-degree 0` disables automatic
  peer discovery; a positive degree maintains that many mesh connections.
- `-broadcast` and UDP port `9000` control default mesh discovery.

Do not confuse a mesh **context** with a minimega **namespace**. Contexts isolate
clusters at process startup; namespaces isolate and schedule experiment state
inside a running cluster.

### Namespaces

The active namespace scopes VMs, taps, captures, VLAN aliases, and other state.
The default namespace is `minimega`.

```text
namespace                         # list namespaces
namespace experiment-a            # create/select namespace
namespace experiment-a vm info    # run one command in a namespace
ns hosts                          # list active namespace hosts
clear namespace                   # return to default namespace
clear namespace experiment-a      # destroy namespace and its resources
```

A new clustered namespace normally contains mesh nodes except the local head
node. With no mesh peers, it contains the local node. `ns add-hosts` only accepts
hosts already present in the mesh.

### VM configuration and lifecycle

minimega supports `kvm`, `container`, and `android` VM types. `vm config` is
mutable namespace-local state copied into every subsequently launched VM; it
does not retroactively change existing VMs.

```text
vm config                         # inspect current defaults
vm config disk image.qcow2
vm config net LAN
vm launch kvm node1               # create VM in BUILDING state
vm start node1                    # start created VM
.columns name,state vm info
vm stop node1
vm kill node1
clear vm config                   # reset current VM configuration
```

KVM can boot from a disk, CD-ROM, or kernel/initrd pair. Bare-metal firmware and
RTOS guests use KVM with `vm config baremetal true`; they require a kernel image
and disabled backchannel, while QMP lifecycle control, serial sockets, and tap
networking remain available. Set `vm config baremetal-network-driver <model>`
for a board-integrated NIC that QEMU does not expose through device discovery.
Containers require a filesystem containing their init executable. Android VMs
require an AVD name, KVM, and discoverable `emulator` and `adb` tools;
`android-sdk` and `android-avd-dir` are optional overrides. Disk snapshot mode
defaults to `true`, so writes normally do not modify the source image.

When `ns queueing true` is active, `vm launch <type> <name>` queues VMs.
Run `vm launch` with no additional arguments to flush the queue and invoke the
scheduler.

## Command usage and integration

- Run `help` for command groups and `help <command>` for exact syntax. Treat
  generated help and the implementation as authoritative.
- Commands entered at the prompt, through `-e`, over the command socket, or over
  meshage use the same command set.
- Output modifiers such as `.columns`, `.filter`, and `.annotate false` precede
  the command and produce stable, narrow output for automation.
- Use `pkg/miniclient` for Go integrations instead of implementing the socket
  framing again.
- `lib/minimega.py` is generated by `pyapigen`; regenerate it through
  `scripts/build.bash`, never edit it directly.
- Use `phenix mm <minimega command...>` when debugging through a phēnix
  deployment.
- Reproduce FIREWHEEL-generated commands directly in minimega when debugging
  model component or experiment-control failures.

Common command areas include:

```text
check                              # report external dependency availability
host name                          # inspect local host
mesh status                        # inspect cluster connectivity
vm info                            # inspect VMs
vlans                              # inspect allocated VLANs
file list                          # inspect iomeshage files
cc clients                         # inspect miniccc clients
capture                            # inspect active captures
```

Consult command help before mutating state; subcommand requirements evolve.

## Gotchas

- **Do not use `-force` casually.** It repopulates a base directory that appears
  occupied; first confirm no live instance owns it.
- **A daemon needs `-nostdin`.** Closing stdin otherwise causes minimega to exit.
- **Use `disconnect`, not `quit`, from `-attach`.** `quit` stops the daemon.
- **Match `-base` for `-e` and `-attach`.** The wrong base targets a different
  socket or reports no running instance.
- **`vm launch` does not mean running.** Without namespace queueing it creates a
  VM, then `vm start` starts it; with queueing it only enqueues until flushed.
- **`vm config` persists.** Clear or explicitly replace prior values before
  launching a different VM class.
- **Relative images use `-filepath`.** A path valid in the shell may not resolve
  as expected inside minimega or its container.
- **Android capacity is finite.** Emulator console/ADB port allocation currently
  limits each host to 64 active Android VMs, or fewer when ports are occupied.
- **Namespace deletion is destructive.** It kills VMs and removes namespace
  captures, VLAN aliases, and taps.
- **Most functional operations alter the host.** Use an isolated Linux system
  for privileged tests involving KVM, Open vSwitch, taps, mounts, or cgroups.
- **Do not parse default tables by column position.** Select columns explicitly
  because command output is also consumed by phēnix, FIREWHEEL, and scripts.

## Troubleshooting

| Symptom | Likely cause or fix |
|---|---|
| Existing instance not found | Pass the same `-base` used to start it and verify `<base>/minimega` exists. |
| Startup warns about missing tools | Run `check`; install only dependencies required by the intended VM/network operation. |
| VM remains `BUILDING` or enters `ERROR` | Inspect `vm info`, minimega logs, QEMU/KVM availability, image paths, and Open vSwitch state. |
| Cluster peers are absent | Confirm matching `-context`, broadcast domain, port, and nonzero `-degree`; use mesh commands for manual links when broadcast is unavailable. |
| VM image is not found | Check `-filepath`, container mounts, iomeshage availability, and whether the command used a relative path. |
| phēnix operation fails in minimega | Run the equivalent raw command with `phenix mm`, then inspect namespace, VM, file, and miniccc state. |
| FIREWHEEL operation fails in minimega | Capture the generated minimega command, reproduce it directly, then inspect host, namespace, VM, network, and miniccc state. |

## References

- `../../doc/content/articles/usage.md`: startup, scripts, command socket,
  mesh operation, and logging.
- `../../doc/content/articles/namespaces.md`: namespace scheduling.
- `../../doc/content/articles/vmtypes.md`: KVM and container behavior.
- `../../docker/README.md`: container operation and configuration.
- [minimega API documentation](https://sandia-minimega.github.io/)
- [phēnix](https://github.com/sandialabs/sceptre-phenix): higher-level
  experiment orchestration.
- [FIREWHEEL](https://github.com/sandialabs/firewheel): model-component-based
  distributed experiment orchestration.
