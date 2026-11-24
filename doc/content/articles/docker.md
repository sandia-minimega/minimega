# Running in Docker

This page covers the official container image:

- How to pull the image.
- The privileges that the image needs.
- How the configuration reaches minimega and miniweb.
- How to run commands inside the container from the host.
- Changes for a container that is part of a cluster.

This page is the documentation-site version of the `docker/README.md` in the
repository. You need a Linux host with Docker Engine and KVM. See
[Installing minimega](installing.md) for the host checks that apply whether
minimega runs in a container or not.

## The image

`ghcr.io/sandia-minimega/minimega` is built from `docker/Dockerfile` on every
push to `master` and on every release. The image has three stages:

1. A `golang:1.24` builder that runs `scripts/all.bash`.
2. A `python:3.13-slim` stage that builds this documentation.
3. The runtime image on Ubuntu 22.04 with the runtime packages.

The runtime packages are `qemu-kvm`, `qemu-utils`, `openvswitch-switch`,
`dnsmasq`, `iproute2`, `isc-dhcp-client`, `ntfs-3g`, `openssh-client`,
`iptables`, `ffmpeg`, `bash-completion`, `cpio`, `less`, `libpcap-dev`, and
`libxkbfile1`.
When you mount an Android SDK into the container, the Android emulator needs
`libxkbfile1`. The image contains these paths:

| Path | Contents |
|---|---|
| `/opt/minimega/bin/` | every tool, on `PATH` |
| `/opt/minimega/lib/minimega.py` | the Python bindings, with `pyproject.toml`, `README.md`, and `VERSION` beside it for `pip install /opt/minimega/lib` |
| `/opt/minimega/web/` | the miniweb root |
| `/opt/minimega/web/docs/` | this documentation, also linked as `/opt/minimega/docs` |
| `/usr/local/bin/mm` | `minimega -e "$@"`, a shortcut to run one command |
| `/start-minimega.sh` | the container's entry point |
| `/usr/share/bash-completion/completions/minimega` | bash completion for shells inside the container |
| `/usr/share/doc/minimega/` | `LICENSE` and the third-party licenses in `LICENSES/` |

The image does not contain `misc/vmbetter_configs` or the tools that vmbetter
needs, for example `debootstrap`. Build guest images on the host or in a
container that you extend yourself.

The `latest` tag and the version tags (`3.4.0`, `3.4`, `3`) follow releases.
The `master` tag follows the development branch. Each commit also has a tag
with its short SHA.

```bash
docker pull ghcr.io/sandia-minimega/minimega:latest
```

## Starting the container

```bash
docker run -d \
  --name minimega \
  --hostname minimega \
  --privileged \
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

- `--privileged`: Open vSwitch, QEMU with `/dev/kvm`, tap
  devices, mounts for file injection and cgroups for containers all happen
  inside the container.
- `-v /dev:/dev` and `-v /lib/modules:/lib/modules:ro`: the host's devices
  and kernel modules, so the container can use and load `kvm`, `openvswitch`
  and `nbd`.
- `-v /tmp/minimega:/tmp/minimega`: the base directory. When you share it with
  the host, `minimega -e` on the host reaches the socket. You also have a place
  to put images, which minimega sees under `/tmp/minimega/files`.
- `-p 9000:9000/udp` and `-p 9001:9001`: mesh discovery and miniweb. With the
  default bridge networking, nothing else is reachable from outside.
- `MM_LOGFILE` and the `/var/log/minimega` mount: the default log file of
  the script is `/var/log/minimega.log`. This file is directly under
  `/var/log`, so mounting only the `/var/log/minimega` directory captures
  nothing. Point `MM_LOGFILE` into the mounted directory as above.
  Alternatively, leave the mount out and read the log with
  `docker logs minimega`. The entry point writes to both.
- `--health-cmd "mm version"`: when the daemon
  answers on its socket, the container is healthy.

`docker/docker-compose.yml` in the repository is this configuration for
`docker compose up -d`, with two differences. It uses the network of the host
(`network_mode: host`) in place of published ports. It sets no host name, so
the container has the host name of the machine. It uses the image of the
latest release and sets `MM_LOGFILE`. It also runs an init process as PID 1
(`init: true`), which starts the entry point. To use a different image, set the `MINIMEGA_IMAGE`
environment variable, for example
`MINIMEGA_IMAGE=ghcr.io/sandia-minimega/minimega:master`.

With `docker run`, the entry point, `/start-minimega.sh`, runs as PID 1. It
removes a stale
socket and PID file that a previous run left. Then it starts Open vSwitch with
`ovs-ctl` and waits up to thirty seconds for `ovs-vsctl show` to succeed. Next,
it optionally adds host interfaces to a bridge. Then it starts miniweb.
Finally, it starts minimega with `-nostdin` and the flags that it builds from
the variables below.

When you stop the container, the daemon receives SIGTERM and tears down VMs,
taps and dnsmasq instances. If you pass a command after the image name, the
command replaces the entry point. This is useful only for single checks, for
example
`docker run --rm ghcr.io/sandia-minimega/minimega minimega -version`.

## Configuration

You set the configuration with environment variables. The entry point resolves
each variable in this order. The first match wins:

1. A variable that is already set in the container's environment (`-e` on `docker
   run`, `environment` or `env_file` in Compose).
2. A `KEY=value` line in a file mounted at `/etc/default/minimega`. The entry
   point strips surrounding double quotes. The file is not a shell script.
3. The default that is built into the script.

A variable that is set but empty counts as unset, so the file value or the
default applies. `/etc/default/minimega` exists only inside the container. It
is unrelated to `/etc/minimega/minimega.conf` of the host service.

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
| `MINIWEB_APPEND` | empty | appended verbatim to the miniweb command line |
| `OVS_APPEND` | empty | extra arguments to `ovs-ctl start` |
| `OVS_HOST_IFACE` | empty | host interfaces to add to a bridge |

Three defaults differ from the flag defaults of the minimega binary on
purpose. The log level is `info` rather than `error`. `MM_DEGREE` is `1`, so a
container joins a mesh as soon as it sees a peer. `MM_FORCE` is `true`, as in
the packaged `minimega.conf`, so a restarted container never refuses to start
because of its own leftover socket.

`MM_APPEND` carries any flag that the table does not cover, for example
`MM_APPEND="-hashfiles -headnode=head1"` or `-msa=20`. The entry point splits
it on whitespace and places it last, so a flag repeated there overrides the
generated one. You cannot pass values with spaces this way.

If you change `MM_PORT` or `MINIWEB_PORT`, also change the `-p` mappings. If
you change `MM_BASE` in the environment of the container, miniweb, the `mm`
shortcut, and the health check use the new directory. A value that is only in
`/etc/default/minimega` does not reach `docker exec`, so use
`minimega -base <path> -e ...` there. Also change the `/tmp/minimega` mount.

### Open vSwitch inside the container

The entry point starts `ovsdb-server` and `ovs-vswitchd` with `ovs-ctl`.
`OVS_APPEND` adds arguments to that call. For a host that already runs Open
vSwitch and needs only the client tools inside the container, use `--no-ovsdb-server
--no-ovs-vswitchd`. The client tools then need the database socket of the host.
Mount the Open vSwitch run directory of the host, usually
`/var/run/openvswitch`, at the same path in the container.

`OVS_HOST_IFACE=<bridge>:<port>[,<port>...]` brings up the bridge and adds each
host interface to it, for example `OVS_HOST_IFACE=mega_bridge:eth1`. If the
bridge does not exist, it creates the bridge first. This is how VLAN traffic
reaches other hosts or physical equipment. The interface becomes a trunk port
on the bridge that carries every VLAN. When the interface belongs to the
bridge, it loses its host IP configuration. Do not use the interface that
carries your SSH session.

## Running commands

```bash
docker exec -it minimega minimega -attach          # a prompt on the daemon
docker exec minimega mm vm info                     # one command, no TTY
docker exec minimega mm .columns name,state,ip vm info
docker exec -it minimega bash                       # a shell with tab completion
```

If a script consumes the output, omit `-it`. A pseudo-terminal adds carriage
returns to every line.

`docker/minimega-wrapper.sh` in the repository makes the `minimega` command on
the host run inside the container. When stdout is not a terminal, it does not
allocate a TTY. It also never allocates a TTY for `-completion`. For this
reason, it works for pipes and for shell completion:

```bash
sudo install -m 0755 docker/minimega-wrapper.sh /usr/local/bin/minimega
minimega -e vm info
minimega -attach
source <(minimega -completion bash)
```

The base directory is shared with the host. Therefore a native `minimega`
binary on the host, the Python bindings and miniweb can also connect to
`/tmp/minimega/minimega` directly.

## miniweb and the bundled documentation

miniweb listens on `MINIWEB_HOST:MINIWEB_PORT`. `docker run` publishes it as
port 9001, and the Compose file uses port 9001 of the host directly. Open
`http://localhost:9001/` for the VM list, screenshots and VNC consoles
([miniweb](miniweb.md)), and `http://localhost:9001/docs/` for this
documentation, served from `/opt/minimega/web/docs`. If you point
`MINIWEB_ROOT` somewhere else, the documentation stays at
`/opt/minimega/docs` inside the container but is no longer served.

`MINIWEB_APPEND` carries the miniweb flags that have no variable, for example
`MINIWEB_APPEND="-passwords=/etc/miniweb/passwords"` to require a password.
Mount the files that the flags name into the container. Images of release 3.4.0
and earlier ignore `MINIWEB_APPEND`. See
[Authentication](miniweb.md#authentication).

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

A containerized minimega can join a mesh, but these restrictions apply:

- Discovery uses UDP broadcast and connections use TCP on `MM_PORT`. With
  bridge networking, only the published UDP port is reachable. The
  container's own address is a private Docker address, and other nodes cannot
  connect back to it. Run cluster members with `--network host`, so the daemon
  binds the interfaces of the host directly. Then drop the `-p` options.
- `deploy launch` copies the binary to other hosts over SSH from inside the
  container. Mount the keys that you want it to use, read-only:
  `-v /root/.ssh:/root/.ssh:ro`.
- VM traffic between hosts needs a trunk interface on the bridge. Use
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
