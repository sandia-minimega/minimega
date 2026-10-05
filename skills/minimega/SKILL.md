---
name: minimega
description: 'This skill should be used when the user asks how to configure, run, automate, integrate, or troubleshoot minimega (VMs, namespaces, VLANs, clusters, miniccc, miniweb, command socket or Python API, Docker or systemd deployment, phēnix or FIREWHEEL integration), or when changing code that defines minimega startup flags, CLI commands, or other externally visible behavior. Example triggers: "run minimega in Docker", "why does vm launch leave the VM in BUILDING", "phenix mm fails", "add a minimega startup flag".'
license: GPL-3.0-only
---

# minimega operations and integration

minimega manages KVM virtual machines, Android emulators, and Linux containers
on one host or across a mesh-connected cluster. This skill focuses on using
minimega and preserving its operational contracts.

File paths in this skill are relative to the minimega repository root. The skill
lives at `skills/minimega/` inside that repository and may be installed through
a symlink, so do not resolve repository paths relative to this file. Read
`AGENTS.md` at the repository root before changing code, configuration, startup
flags, deployment defaults, the Docker wrapper, service units, or tests.

Supporting files in this skill directory:

- `references/configuration.md`: full detail on native flags and environment
  variables, the Docker wrapper, the systemd unit, precedence, and help output.
- `examples/minimal-kvm.mm`: a complete, runnable command file.

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

The repository container provides an `mm` wrapper that runs `minimega -e`:

```bash
docker exec minimega mm vm info
```

Omit `-it` for non-interactive automation. The normal container deployment is
privileged and mounts host devices; follow `docker/README.md`.

## Configuration and CLI help

Keep every launcher aligned with the flags registered in `cmd/minimega/main.go`
and `pkg/minilog/minilog.go`. The compiled `minimega -h` output is authoritative
for flag names and defaults. `references/configuration.md` covers each mode in
depth; the essentials are:

- **Sources.** The native binary reads compiled defaults, `MM_*` environment
  variables (mapped in `cmd/minimega/environment.go`), and command-line flags,
  in that order of increasing precedence. It does not load YAML, JSON, TOML, or
  dotenv files. `-e`, `-attach`, and `-pipe` also read `MM_BASE` from
  `/etc/minimega/minimega.conf` when neither `MM_BASE` nor `-base` is set.
- **Flag syntax.** Go flag syntax; parsing stops at the first non-flag argument
  or `--`, and the last repeated flag wins. Put `-base` and `-namespace` before
  the command following `-e`.
- **`-force` and `-recover`.** With `-recover=true`, recovery runs after mesh
  initialization even if `-force=true`. The compiled `-h` text for `-recover`
  says "only if -force is not set"; the implementation disagrees, so treat that
  help string as stale.
- **No startup command file.** Server startup ignores positional arguments.
  Apply runtime configuration afterwards with `minimega -e read <file>`; the
  `check` form validates syntax without executing. `read` stops at invalid
  syntax, continues past a command that returns an error, and rejects nested
  `read`. See `examples/minimal-kvm.mm`.
- **Docker.** `docker/start-minimega.sh` generates flags from `MM_*` container
  environment variables, then `/etc/default/minimega`, then script defaults.
  `MM_APPEND` adds raw flags. Docker defaults differ from native defaults
  (`MM_DEGREE=1`, `MM_LOGLEVEL=info`, `MM_FORCE=true`, a log file).
- **systemd.** `misc/daemon/minimega.service` maps `/etc/minimega/minimega.conf`
  to explicit `ExecStart` flags; `MM_APPEND` is not supported there.
- **Help.** `minimega -h` for startup flags, `minimega -e help [command]` for
  runtime commands, `minimega -cli` for the runtime CLI as JSON. Pass the
  correct `-base=<path>` before `-e` when the daemon uses a non-default base.

## Core concepts

### Base directory, file path, and context

- `-base` controls runtime state and the local command socket.
- `-filepath` controls files served by iomeshage; relative VM image paths resolve
  beneath it. Use absolute paths when the file is elsewhere. When `-base`
  changes and `-filepath` keeps its default, the file path becomes
  `<base>/files`.
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
RTOS guests use KVM with `vm config baremetal true`. They require a kernel image
and reject the miniccc backchannel, bidirectional copy and paste, virtio serial
ports, disks, CD-ROMs, migrated VM state, and TPM devices, so clear those from
`vm config` before launching. A bare-metal VM with any `vm config net` entry
must also set `vm config baremetal-network-driver <model>`, because the
board-integrated NIC is not exposed through QEMU device discovery; without the
driver, launch fails. QMP lifecycle control, serial sockets, and tap networking
remain available.

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
- `lib/minimega.py` is not checked in; `lib/` is ignored by git and the file
  only exists after `scripts/build.bash` generates it with `pyapigen` from the
  built binary. Never edit it directly, and do not report it missing from a
  fresh checkout.
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
- **Relocating `MM_BASE` in Docker.** The `mm` wrapper and Compose health check
  honor `MM_BASE` from the container environment, but not a value set only in
  `/etc/default/minimega`; pass `-base=<path>` in that case and update the
  `/tmp/minimega` volume mount if the host needs the socket or files.
- **`vm launch` does not mean running.** Without namespace queueing it creates a
  VM, then `vm start` starts it; with queueing it only enqueues until flushed.
- **`vm config` persists.** Clear or explicitly replace prior values before
  launching a different VM class; bare-metal launches reject leftover disks.
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

Repository paths, relative to the minimega repository root:

- `doc/content/articles/usage.md`: startup, scripts, command socket, mesh
  operation, and logging.
- `doc/content/articles/namespaces.md`: namespace scheduling.
- `doc/content/articles/vmtypes.md`: KVM and container behavior.
- `docker/README.md`: container operation and configuration.

External:

- [minimega API documentation](https://sandia-minimega.github.io/minimega/reference/minimega/)
- [phēnix](https://github.com/sandialabs/sceptre-phenix): higher-level
  experiment orchestration.
- [FIREWHEEL](https://github.com/sandialabs/firewheel): model-component-based
  distributed experiment orchestration.
