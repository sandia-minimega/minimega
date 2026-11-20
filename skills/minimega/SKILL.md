---
name: minimega
description: 'This skill should be used when the user asks how to configure, run, automate, integrate, or troubleshoot minimega (VMs, namespaces, VLANs, clusters, miniccc, miniweb, command socket or Python API, Docker or systemd deployment, phēnix or FIREWHEEL integration), or when changing code that defines minimega startup flags, CLI commands, or other externally visible behavior. Example triggers: "run minimega in Docker", "why does vm launch leave the VM in BUILDING", "phenix mm fails", "add a minimega startup flag".'
license: GPL-3.0-only
---

# minimega operations and integration

minimega manages KVM virtual machines, Firecracker microVMs, Android emulators,
and Linux containers on one host or across a mesh-connected cluster. This skill focuses on using
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
  to explicit `ExecStart` flags; `MM_APPEND` is not supported there. minimega
  reads the `MM_FIRECRACKER_*` variables of that file from its environment.
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

minimega supports `kvm`, `container`, `android`, and `firecracker` VM types. `vm config` is
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

Firecracker VMs run a Linux guest in a jailed Firecracker microVM. They need an
x86_64 host with `/dev/kvm`, static `firecracker` and `jailer` binaries of the
same version (v1.17.0 or later), and `vm config kernel`. `vm launch` configures
the VMM, and `vm start` boots the guest. A guest reboot stops the VMM, and the
VM goes to `QUIT`. `vm start` then boots it again. cc uses vsock: run
`miniccc -family vsock -port 9002` in the guest. Read
`doc/content/articles/firecracker.md` before you change Firecracker behavior.

When `ns queueing true` is active, `vm launch <type> <name>` queues VMs.
Run `vm launch` with no additional arguments to flush the queue and invoke the
scheduler.

### VNC

Each KVM VM has a VNC shim at the `vnc_port` column of `vm info`. Viewers,
recordings, playbacks, and pastes each open a connection through it. QEMU
allows 32 shared clients for each VM and closes each new connection after that.
See `doc/content/articles/vnc.md`.

- **Copy and paste.** `vm config bidirectional-copy-paste` selects the mode.
  With `false` (the default), the shim types client clipboard text as key
  events: printable ASCII, Tab, and Enter only, paced 10 ms for each key event,
  in order for each VM. `vnc type` uses the same path. That is approximately
  50 characters each second (25 with Shift), a maximum of 65,536 characters
  for each text, and a maximum of 8 texts that wait for each VM ("paste queue
  full"). Keys typed during a paste mix with it. With `true`, the text
  goes to QEMU `qemu-vdagent` (UTF-8, both directions). This mode needs QEMU
  6.1 or later and `spice-vdagent` in the guest.
- **Recording.** `vnc record kb|fb <vm> <file>` writes on the VM host; a
  relative path is under `-filepath`. Recordings stop on `vnc stop`, VM kill,
  flush, or guest shutdown. kb files keep keysyms only: QEMU extended key events become
  `KeyEvent` lines, and pointer masks have 9 bits. Playback sends keysyms
  through QEMU's `en-us` keymap and drops the Back and Forward bits (128, 256).
  A text paste or `vnc type` and a `vnc play` cannot run on one VM at the same
  time ("already playing").
- **Keyboard layout.** minimega does not pass `-k` to QEMU. For QEMU extended
  key events (noVNC sends them), the guest layout decides the character from
  the key position. `vm config qemu-append -k en-us` gives keysym translation
  instead.
- **Resize.** A client can change the guest screen size only with
  `vm config vga virtio`. The `vga` column of `vm info` shows the card. fb
  recordings follow a guest resize.

## Command usage and integration

- Run `help` for command groups and `help <command>` for exact syntax. Treat
  generated help and the implementation as authoritative.
- Commands entered at the prompt, through `-e`, over the command socket, or over
  meshage use the same command set.
- Output modifiers such as `.columns`, `.filter`, and `.annotate false` precede
  the command and produce stable, narrow output for automation.
- Terminal output truncates table columns to `.width` characters (default
  50); `.width 0`, piped output, `.csv true`, and `.json true` show all data.
- Use `pkg/miniclient` for Go integrations instead of implementing the socket
  framing again.
- `lib/minimega.py` is not checked in; git ignores the file, and the file
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
vlans blacklist                    # inspect blacklisted VLANs
file list                          # inspect iomeshage files
cc clients                         # inspect miniccc clients and capabilities
cc auto-update [true,false]        # show or set miniccc auto-update per namespace
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
- **Firecracker VMs have at most 17 devices** (31 with
  `vm config firecracker-pci true`). Each disk, the scratch disk, and each NIC
  use one device. The vsock device (`backchannel true`) and the entropy device
  (`firecracker-entropy true`) also use one each, and both are on by default.
- **Firecracker VMs are not recovered** after a minimega restart, and they
  have no VNC, screenshots, CD-ROM, hotplug, `vm net add`, or bonds.
- **Startup removes old Firecracker VMs.** minimega kills the VMMs that an
  earlier process with the same `-base` and `-firecracker-dir` left, and
  removes their host objects. It keeps the image cache.
- **`-firecracker-dir` must be one filesystem** that is not `nodev` or
  `noexec`. Only the owner can write to it and to its parent directories.
  Run `check` and read the log for the Firecracker results.
- **Namespace deletion is destructive.** It kills VMs and removes namespace
  captures, VLAN aliases, and taps.
- **Remove one blacklisted VLAN with `clear vlans blacklist <vlan>`.** Do not
  use `clear vlans all` for this; it clears every alias in every namespace.
  The command fails if the VLAN is not blacklisted or in use. It checks VMs on
  this node and all namespace hosts, but VM configs, queued or launching VMs,
  and host taps on this node only. It also fails if a namespace host does not
  return its VMs; remove hosts that left the mesh with `ns del-hosts`. It
  cannot stop commands on other hosts. Without a VLAN ID, `clear vlans
  blacklist` clears aliases that start with `blacklist`. The blacklist is local
  to each node.
- **Most functional operations alter the host.** Use an isolated Linux system
  for privileged tests involving KVM, Open vSwitch, taps, mounts, or cgroups.
- **`cc auto-update` updates only capable clients.** A client gets an update
  only when `cc clients` shows `auto-update` in its `capabilities` column and
  its binary hash differs from the staged binary. Older miniccc binaries need a
  manual update. minimega looks for `miniccc-<os>-<arch>` (`.exe` on Windows),
  then for `miniccc` or `miniccc.exe` only for the host architecture. Each host
  gets these files over the mesh when you run `cc auto-update true`. minimega
  holds commands for an updating client for up to two minutes. The update
  kills the `cc background` processes of the client. A read-only folder for the
  miniccc binary keeps a fixed version.
- **Packet captures have IDs, states, and counters.** `capture` lists the ID,
  state (`running`, `failed`, or `full`), packets, kernel drops, and file size
  of each pcap capture, with `path` as the last column. A failed or full capture
  stays in the list until `capture pcap delete id <id>`. `clear capture pcap`
  stops only pcap captures. Give `snaplen=`, `filter=`, `maxsize=`,
  `format=pcap|pcapng`, and (VM only) `direction=` after the file name instead of changing the namespace
  settings around one capture.
- **Namespace captures run on the head.** `capture pcap namespace <file>`
  merges the edge traffic of every namespace host into one file on the node
  that runs it and returns an ID. List with `capture pcap namespace` and stop
  with `capture pcap delete namespace <id|file|all>` on that same node; plain
  `capture` does not list them. Every mesh node, relays included, must support
  it, or the start fails.
- **A bridge has at most 32 working mirrors.** Open vSwitch silently ignores
  more. Tap mirrors and captures share them; minimega refuses a new mirror at
  32. `bridge mirrors` lists every Mirror row with its owner namespace and
  kind, also rows that minimega did not make (`external`).
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
- `doc/content/articles/firecracker.md`: Firecracker host requirements,
  lifecycle, limits, cc over vsock, save and restore, and scale notes.
- `doc/content/articles/vnc.md`: VNC recording, playback, typing, and copy and
  paste.
- `docker/README.md`: container operation and configuration.

External:

- [minimega API documentation](https://sandia-minimega.github.io/minimega/reference/minimega/)
- [phēnix](https://github.com/sandialabs/sceptre-phenix): higher-level
  experiment orchestration.
- [FIREWHEEL](https://github.com/sandialabs/firewheel): model-component-based
  distributed experiment orchestration.
