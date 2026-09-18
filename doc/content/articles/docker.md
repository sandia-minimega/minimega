# Running in Docker

This page covers the official container image: pulling it, the privileges it
needs, how its settings reach minimega and miniweb, how to run commands inside
it from the host, and what changes when the container is part of a cluster.
It is the documentation-site version of the `docker/README.md` in the
repository. You need a Linux host with Docker Engine and KVM; see
[Installing minimega](installing.md) for the host checks that apply whether
minimega runs in a container or not.

## The image

`ghcr.io/sandia-minimega/minimega` is built from `docker/Dockerfile` on every
push to `master` and on every release. It has three stages: a `golang:1.24`
builder that runs `scripts/all.bash`, a `python:3.13-slim` stage that builds
this documentation, and the runtime image on Ubuntu 22.04 with the runtime
packages (`qemu-kvm`, `qemu-utils`, `openvswitch-switch`, `dnsmasq`,
`iproute2`, `isc-dhcp-client`, `ntfs-3g`, `openssh-client`, `iptables`,
`ffmpeg`, `bash-completion`). Inside it:

| Path | Contents |
|---|---|
| `/opt/minimega/bin/` | every tool, on `PATH` |
| `/opt/minimega/lib/minimega.py` | the Python bindings, with `README.md` and `VERSION` beside it for `pip install /opt/minimega/lib` |
| `/opt/minimega/web/` | the miniweb root |
| `/opt/minimega/web/docs/` | this documentation, also linked as `/opt/minimega/docs` |
| `/usr/local/bin/mm` | `minimega -e "$@"`, a shortcut for one-off commands |
| `/start-minimega.sh` | the container's entry point |
| `/usr/share/bash-completion/completions/minimega` | bash completion for shells inside the container |

The image does not contain `misc/vmbetter_configs` or the tools vmbetter
needs (`debootstrap` and friends); build guest images on the host or in a
container you extend yourself.

Tags: `latest` and the version tags (`3.2.0`, `3.2`, `3`) follow releases,
`master` follows the development branch, and each commit is also tagged with
its short SHA.

```bash
docker pull ghcr.io/sandia-minimega/minimega:latest
```

## Starting the container

```bash
docker run -d \
  --name minimega \
  --hostname minimega \
  --privileged \
  --cap-add ALL \
  -p 9000:9000/udp \
  -p 9001:9001 \
  -v /dev:/dev \
  -v /lib/modules:/lib/modules:ro \
  -v /tmp/minimega:/tmp/minimega \
  -v /var/log/minimega:/var/log/minimega \
  -e MM_LOGFILE=/var/log/minimega/minimega.log \
  --health-cmd "mm version" \
  ghcr.io/sandia-minimega/minimega:latest
```

Why each part is there:

- `--privileged` and `--cap-add ALL`: Open vSwitch, QEMU with `/dev/kvm`, tap
  devices, mounts for file injection and cgroups for containers all happen
  inside the container.
- `-v /dev:/dev` and `-v /lib/modules:/lib/modules:ro`: the host's devices
  and kernel modules, so `kvm`, `openvswitch` and `nbd` can be used and loaded.
- `-v /tmp/minimega:/tmp/minimega`: the base directory. Sharing it with the
  host lets `minimega -e` on the host reach the socket, keeps VM state across
  container restarts, and gives you a place to drop images that minimega sees
  under `/tmp/minimega/files`.
- `-p 9000:9000/udp` and `-p 9001:9001`: mesh discovery and miniweb. With the
  default bridge networking nothing else is reachable from outside.
- `MM_LOGFILE` and the `/var/log/minimega` mount: the script's default log
  file is `/var/log/minimega.log`, a file directly under `/var/log`, so
  mounting the `/var/log/minimega` directory alone captures nothing. Point
  `MM_LOGFILE` into the mounted directory as above, or leave the mount out and
  read the log with `docker logs minimega`; the entry point writes to both.
- `--health-cmd "mm version"`: healthy once the daemon answers on its socket.

`docker/docker-compose.yml` in the repository is the same configuration for
`docker compose up -d`; it builds the image locally and uses the same
`/var/log/minimega` mount, so add `MM_LOGFILE` to its `environment` for the
same reason.

The entry point, `/start-minimega.sh`, runs as PID 1. It removes a stale
socket and PID file left by a previous run, starts Open vSwitch with `ovs-ctl`
and waits up to thirty seconds for `ovs-vsctl show` to succeed, optionally
adds host interfaces to a bridge, starts miniweb, and finally starts minimega
with `-nostdin` and the flags built from the variables below. Stopping the
container sends the daemon SIGTERM, which tears down VMs, taps and dnsmasq
instances. Passing a command after the image name replaces the entry point,
which is only useful for one-off checks such as
`docker run --rm ghcr.io/sandia-minimega/minimega minimega -version`.

## Configuration

Settings are environment variables. The entry point resolves each one in this
order, first match wins:

1. a variable already set in the container's environment (`-e` on `docker
   run`, `environment` or `env_file` in Compose);
2. a `KEY=value` line in a file mounted at `/etc/default/minimega` (surrounding
   double quotes are stripped; the file is not a shell script);
3. the default built into the script.

A variable that is set but empty counts as unset, so the file value or the
default applies. `/etc/default/minimega` exists only inside the container; it
is unrelated to the host service's `/etc/minimega/minimega.conf`.

| Variable | Default in the image | Passed as |
|---|---|---|
| `MM_BASE` | `/tmp/minimega` | `-base` |
| `MM_FILEPATH` | `/tmp/minimega/files` | `-filepath` |
| `MM_LOGLEVEL` | `info` | `-level` |
| `MM_LOGFILE` | `/var/log/minimega.log` | `-logfile` |
| `MM_FORCE` | `true` | `-force` |
| `MM_RECOVER` | `false` | `-recover` |
| `MM_DEGREE` | `1` | `-degree` |
| `MM_CONTEXT` | `minimega` | `-context` |
| `MM_PORT` | `9000` | `-port` |
| `MM_BROADCAST` | `255.255.255.255` | `-broadcast` |
| `MM_VLANRANGE` | `101-4096` | `-vlanrange` |
| `MM_CGROUP` | `/sys/fs/cgroup` | `-cgroup` |
| `MM_ABSSNAPSHOT` | `false` | `-abssnapshot` |
| `MM_APPEND` | empty | appended verbatim to the command line |
| `MINIWEB_ROOT` | `/opt/minimega/web` | miniweb `-root` |
| `MINIWEB_HOST` | `0.0.0.0` | miniweb `-addr` (with `MINIWEB_PORT`) |
| `MINIWEB_PORT` | `9001` | miniweb `-addr` |
| `OVS_APPEND` | empty | extra arguments to `ovs-ctl start` |
| `OVS_HOST_IFACE` | empty | host interfaces to add to a bridge |

Three defaults differ from a native install on purpose: the log level is
`info` rather than `error`, `MM_DEGREE` is `1` so a container joins a mesh as
soon as it sees a peer, and `MM_FORCE` is `true` so a restarted container
never refuses to start over its own leftover socket.

`MM_APPEND` carries any flag the table does not cover, for example
`MM_APPEND="-hashfiles -headnode=head1"` or `-msa=20`. It is split on
whitespace and placed last, so a flag repeated there overrides the generated
one. Values with spaces cannot be passed this way.

Changing `MM_PORT` or `MINIWEB_PORT` also means changing the `-p` mappings.
Changing `MM_BASE` breaks the `mm` shortcut and the health check, which both
assume `/tmp/minimega`; use `minimega -base <path> -e ...` instead.

### Open vSwitch inside the container

The entry point starts `ovsdb-server` and `ovs-vswitchd` with `ovs-ctl`.
`OVS_APPEND` adds arguments to that call; `--no-ovsdb-server
--no-ovs-vswitchd` is the combination for a host that already runs Open
vSwitch and only wants the client tools inside the container.

`OVS_HOST_IFACE=<bridge>:<port>[,<port>...]` creates the bridge if needed,
brings it up, and adds each host interface to it, for example
`OVS_HOST_IFACE=mega_bridge:eth1`. This is how VLAN traffic reaches other
hosts or physical equipment: the interface becomes a trunk port on the bridge
that carries every VLAN. The interface loses its host IP configuration once it
belongs to the bridge, so do not use the one that carries your SSH session.

## Running commands

```bash
docker exec -it minimega minimega -attach          # a prompt on the daemon
docker exec minimega mm vm info                     # one command, no TTY
docker exec minimega mm .columns name,state,ip vm info
docker exec -it minimega bash                       # a shell with tab completion
```

Omit `-it` when the output is consumed by a script; a pseudo-terminal adds
carriage returns to every line.

`docker/minimega-wrapper.sh` in the repository makes the host's `minimega`
command transparently run inside the container. It allocates a TTY only when
stdout is a terminal, and never for `-completion`, so it works for pipes and
for shell completion:

```bash
sudo install -m 0755 docker/minimega-wrapper.sh /usr/local/bin/minimega
minimega -e vm info
minimega -attach
source <(minimega -completion bash)
```

Because the base directory is shared with the host, a native `minimega`
binary on the host, the Python bindings and miniweb can also connect to
`/tmp/minimega/minimega` directly.

## miniweb and the bundled documentation

miniweb listens on `MINIWEB_HOST:MINIWEB_PORT`, published as port 9001. Open
`http://localhost:9001/` for the VM list, screenshots and VNC consoles
([miniweb](miniweb.md)), and `http://localhost:9001/docs/` for this
documentation, served from `/opt/minimega/web/docs`. If you point
`MINIWEB_ROOT` somewhere else the documentation stays at
`/opt/minimega/docs` inside the container but is no longer served.

## Python bindings

The container carries the bindings that match its daemon at
`/opt/minimega/lib/`. Install them on the host from the running container, or
use the same version from PyPI:

```bash
docker cp minimega:/opt/minimega/lib /tmp/minimega-lib
python3 -m pip install /tmp/minimega-lib
```

Scripts on the host connect through the shared socket,
`minimega.connect('/tmp/minimega/minimega')`. See
[Python bindings](python.md).

## Clusters

A containerized minimega can join a mesh, with caveats:

- Discovery uses UDP broadcast and connections use TCP on `MM_PORT`. With
  bridge networking only the published UDP port is reachable, and the
  container's own address is a private Docker address that other nodes cannot
  connect back to. Run cluster members with `--network host` so the daemon
  binds the host's interfaces directly, and drop the `-p` options.
- `deploy launch` copies the binary to other hosts over SSH from inside the
  container, so mount the keys it should use, read-only:
  `-v /root/.ssh:/root/.ssh:ro`.
- VM traffic between hosts needs a trunk interface on the bridge; use
  `OVS_HOST_IFACE` as described above.

[Cluster setup](cluster.md) covers the mesh itself, `-context`, `-degree` and
namespaces across hosts.

## Building the image yourself

From the repository root:

```bash
docker build -t minimega -f docker/Dockerfile .
docker run --rm minimega minimega -version
```

`--build-arg BASE_IMAGE=...`, `GO_IMAGE=...` and `PYTHON_IMAGE=...` override
the three base images.

## See also

- [Running minimega](running.md) for what every `MM_*` value and flag means
- [Command line and scripting](cli.md)
- [Installing minimega](installing.md)
- [miniweb](miniweb.md)
