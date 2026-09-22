# minimega Docker

## Install Docker

Follow the official installation instructions:
[Install Docker Engine](https://docs.docker.com/engine/install/)

> [!TIP]
> For development purposes, it maybe helpful to add your user to the `docker`
> group: `sudo usermod -aG docker $USER`

## Build the Docker images

minimega and miniweb run in separate containers. `docker/Dockerfile` has one
final build stage for each, selected with `--target`. Building without
`--target` produces the `minimega` image.

> [!IMPORTANT]
> The docker images need to be built from the base directory of the minimega
> repository.

```bash
docker build --target minimega -t minimega -f docker/Dockerfile .
docker build --target miniweb  -t miniweb  -f docker/Dockerfile .

# Ensure the builds were successful
docker run --rm minimega /opt/minimega/bin/minimega --version
docker run --rm miniweb ls /opt/minimega/web
```

## Start the minimega Docker container

> [!NOTE]
> The additional privileges and system mounts (e.g. /dev) are required for the
> openvswitch process to run inside the container and to allow minimega to
> perform file injections.

> [!WARNING]
> If the `deploy launch` minimega command is used to initialize a multi-node
> minimega cluster, then a directory containing SSH keys will likely need to be
> mounted as a volume as well (and can be read-only). An example would be
> `-v /root/.ssh:/root/.ssh:ro`.

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
  -v /var/log/minimega:/var/log/minimega \
  -v /tmp/minimega:/tmp/minimega \
  --health-cmd "mm version" \
  minimega
```

The container runs the `start-minimega.sh` script, which starts openvswitch and
then execs minimega, so minimega itself is PID 1 and `docker stop` signals it
directly. minimega tears down namespaces and bridges on SIGTERM and removes its
own socket and PID file; anything left behind by a crash is cleaned up the next
time the script runs. This means the minimega logs will be available in the
container logs via Docker (`docker logs minimega`).

Port 9001 is published here even though minimega itself does not listen on it,
because the miniweb container below shares this container's network namespace.
Omit `-p 9001:9001` if you do not intend to run miniweb.

## Start the miniweb Docker container

miniweb is optional. Skip it when another interface, such as
[phēnix](https://github.com/sandialabs/sceptre-phenix), already provides a UI.

miniweb reaches minimega in two ways:

1. It issues commands over the minimega Unix socket, so it needs the same
   `/tmp/minimega` mount.
2. It proxies VNC and container consoles directly to the ephemeral per-VM shim
   ports that minimega opens, so it must be inside minimega's network
   namespace.

`--network container:minimega` satisfies the second requirement and is why the
miniweb container publishes no ports of its own.

```bash
docker run -d \
  --name miniweb \
  --network container:minimega \
  -v /etc/localtime:/etc/localtime:ro \
  -v /tmp/minimega:/tmp/minimega \
  -v /var/log/minimega:/var/log/minimega \
  miniweb
```

The container runs the `start-miniweb.sh` script, which execs miniweb as PID 1
so Docker signals reach it directly. miniweb dials the minimega socket lazily
and redials when it goes away, so the two containers can start in any order and
minimega can be restarted underneath a running miniweb.

The miniweb image includes the documentation in `/opt/minimega/docs`. With the
default `MINIWEB_ROOT`, miniweb also serves it at
[http://localhost:9001/docs/](http://localhost:9001/docs/).

# Using Docker Compose

If you followed the
[Docker installation instructions](https://docs.docker.com/engine/install/),
then `docker compose` should already be installed. Verify this by running
`docker compose version`. If it's not, then install it using the command:
`sudo apt install docker-compose-plugin`

Start both containers with Docker Compose:

```bash
cd docker/
docker compose up -d
docker compose logs -f  # CTRL+C to stop following logs
```

To run minimega without miniweb, name the service explicitly:

```bash
docker compose up -d minimega
```

To add miniweb later, or to restart it on its own, name it the same way.
`--no-deps` leaves the running minimega container alone:

```bash
docker compose up -d --no-deps miniweb
docker compose logs -f miniweb
docker compose restart miniweb
```

# Extras

## Host wrapper script and tab completion

The `docker/minimega-wrapper.sh` script lets you run `minimega` commands from
the host as if the binary were installed locally, by transparently forwarding
them to `docker exec` inside the running `minimega` container. It also takes
care of only allocating a pseudo-TTY (`-it`) when stdout is actually a
terminal, so its output can be safely piped or redirected (e.g. for bash tab
completion, see below) without picking up spurious `\r` characters.

Install it as `/usr/local/bin/minimega` so it's available on your `$PATH`:

```bash
sudo install -m 0755 docker/minimega-wrapper.sh /usr/local/bin/minimega
```

With that in place, you can run minimega commands directly:

```bash
minimega -e vm info
minimega -attach
```

### Installing tab completion

minimega can generate its own tab-completion script for `bash`, `zsh`, or
`fish` via the `-completion` flag. Since the wrapper script forwards
`-completion` through to the minimega binary inside the container, you can
use it the same way you would the native binary.

To enable it for the current shell session:

```bash
source <(minimega -completion bash)   # bash
source <(minimega -completion zsh)    # zsh
minimega -completion fish | source    # fish
```

To enable it for every session, write the generated script somewhere your
shell's completion system will find it, e.g.:

```bash
# bash
minimega -completion bash | sudo tee /usr/share/bash-completion/completions/minimega >/dev/null

# zsh (write to a directory in your fpath)
minimega -completion zsh | sudo tee /usr/share/zsh/site-functions/_minimega >/dev/null

# fish
minimega -completion fish | sudo tee /usr/share/fish/vendor_completions.d/minimega.fish >/dev/null
```

With completion installed, typing `minimega -e vm sta<TAB>` will complete to
`minimega -e vm start`, the same way tab completion works at the minimega
command prompt itself.

> [!TIP]
> The `minimega` Docker image ships with the `bash-completion` package and a
> pre-generated completion script at
> `/usr/share/bash-completion/completions/minimega`, so bash tab completion
> works out of the box for interactive shells inside the container, e.g.
> `docker exec -it minimega bash`.

## Convenience aliases

```bash
cat <<EOF >> ~/.bash_aliases
alias mm='docker exec -it minimega minimega -e'
alias mminfo='mm .columns name,state,ip,snapshot,cc_active vm info'
alias mmsum='mm .columns name,state,cc_active,uuid vm info summary'
alias minimega='docker exec -it minimega minimega'
alias ovs-vsctl='docker exec -it minimega ovs-vsctl'
EOF

source ~/.bash_aliases
```

> [!TIP]
> On Ubuntu, `~/.bash_aliases` should be auto-sourced by `~/.profile` or
> `~/.bashrc` on login, so the source command is only needed to load them into
> the current session.

## minimega and miniweb configuration

Both containers read configuration the same way. The order of precedence is:
1. Existing environment variables
2. Variables in `/etc/default/minimega`
3. A set of defaults in the `start-minimega.sh` or `start-miniweb.sh` script

Steps 1 and 2 are implemented by `docker/load-defaults.sh`, which both startup
scripts source. `/etc/default/minimega` is a single shared file: bind it into
whichever containers should read it.

The defaults set for minimega are:

```shell
MM_BASE=/tmp/minimega
MM_FILEPATH=/tmp/minimega/files
MM_BROADCAST=255.255.255.255
MM_VLANRANGE=101-4096
MM_PORT=9000
MM_DEGREE=1
MM_CONTEXT=minimega
MM_LOGLEVEL=info
MM_LOGFILE=/var/log/minimega.log
MM_FORCE=true
MM_RECOVER=false
MM_CGROUP=/sys/fs/cgroup
MM_ABSSNAPSHOT=false
```

The defaults set for miniweb are:

```shell
MINIWEB_ROOT=/opt/minimega/web
MINIWEB_HOST=0.0.0.0
MINIWEB_PORT=9001
MINIWEB_BASE=/tmp/minimega
MINIWEB_LOGLEVEL=info
```

`MINIWEB_BASE` is the path to the minimega base directory holding the command
socket, and must match the minimega container's `MM_BASE`.

These miniweb options have no default; the corresponding flag is only passed
when the variable is set to a non-empty value:

| Variable | miniweb flag | Purpose |
| --- | --- | --- |
| `MINIWEB_LOGFILE` | `-logfile` | Also write logs to a file |
| `MINIWEB_NAMESPACE` | `-namespace` | Limit miniweb to a single namespace |
| `MINIWEB_PASSWORDS` | `-passwords` | Password file for authentication |
| `MINIWEB_CERT` | `-cert` | TLS certificate in PEM format |
| `MINIWEB_KEY` | `-key` | TLS key in PEM format |
| `MINIWEB_CONSOLE` | `-console` | Path to the binary the web console attaches with |

By default miniweb logs to stderr only, so `docker logs miniweb` shows
everything.

Additional values can be appended to the miniweb command with `MINIWEB_APPEND`,
the same way `MM_APPEND` works for minimega.

The default miniweb root contains the bundled documentation under `docs/`.
When `MINIWEB_ROOT` points elsewhere, the documentation remains available
inside the miniweb container at `/opt/minimega/docs` but is not served
automatically.

These values can be overwritten either by passing environment variables to
Docker when starting the container or by binding a file to
`/etc/default/minimega` in the container that contains updated values.

> [!NOTE]
> If a value is specified both as an environment variable to Docker and in
> the file bound to `/etc/default/minimega`, the existing non-empty environment
> variable will be used.

> [!WARNING]
> If the port is changed for minimega or miniweb and standard container
> networking is used (not host networking), then the `ports` section in your
> `docker-compose.yml` or `-p` arguments to `docker run` will need to be updated
> to the new value(s) specified. Because miniweb shares the minimega container's
> network namespace, `MINIWEB_PORT` is published by the **minimega** service,
> not the miniweb one. Changing it also means updating the miniweb health check
> in `docker-compose.yml`, which connects to the default port.

Additional values can be appended to the minimega command by using the
`MM_APPEND` environment variable, for example:

```shell
MM_APPEND="-hashfiles -headnode=foo1"
```

## The web console

miniweb's Console tab runs `minimega -attach` on a pty and pipes it to the
browser. The attached process is only a client: it talks to the daemon over the
command socket in `MINIWEB_BASE`, so it works across containers as long as
`/tmp/minimega` is shared. The miniweb image ships the `minimega` binary and
`/console-attach.sh`, a wrapper that passes `MINIWEB_BASE` through, because
miniweb runs the console binary with `-attach` alone.

Set `MINIWEB_CONSOLE` to enable it:

```bash
docker run -d \
  --name miniweb \
  --network container:minimega \
  -e MINIWEB_CONSOLE=/console-attach.sh \
  -v /etc/localtime:/etc/localtime:ro \
  -v /tmp/minimega:/tmp/minimega \
  -v /var/log/minimega:/var/log/minimega \
  miniweb
```

> [!WARNING]
> The console is a full minimega command line, so anyone who can reach miniweb
> can run any minimega command, including `vm kill` and `clear all`. Leave
> `MINIWEB_CONSOLE` unset unless miniweb is on a trusted network, and pair it
> with `MINIWEB_PASSWORDS`. The console is off by default, which matches the
> single-container image: it never passed `-console` either.

## miniweb container hardening

The Compose `miniweb` service runs with no capabilities, no new privileges, a
read-only root filesystem, and a tmpfs on `/tmp`:

```yaml
    cap_drop:
    - ALL
    security_opt:
    - no-new-privileges:true
    read_only: true
    tmpfs:
    - /tmp
```

miniweb serves static files, talks to the minimega command socket, and binds an
unprivileged port, so it needs none of what it gives up. The `docker run`
equivalents are `--cap-drop ALL`, `--security-opt no-new-privileges:true`,
`--read-only`, and `--tmpfs /tmp`. The `/tmp/minimega` volume still mounts over
the tmpfs, so the command socket is reachable as usual.

One consequence: with `read_only` set, anything miniweb writes has to land in a
mounted volume. `MINIWEB_LOGFILE` is the case to watch. A path on the image,
such as `/var/log/miniweb.log`, makes miniweb exit immediately:

```
open /var/log/miniweb.log: read-only file system
```

Use a path under a mount instead, such as `/var/log/minimega/miniweb.log`. The
same applies to the password file written by `miniweb -bootstrap`.

> [!NOTE]
> This hardening is for the reference deployment, not a requirement. Remove
> `read_only` and `tmpfs` (or override them in a `docker-compose.override.yml`)
> when developing or debugging inside the container, where installing packages
> and writing scratch files is the point. miniweb behaves the same either way.

## Open vSwitch configuration

The minimega container `start-minimega.sh` script takes care of starting
Open vSwitch server using the
[`ovs-ctl`](https://docs.openvswitch.org/en/latest/ref/ovs-ctl.8/) program.

Additional values can be appended to the `ovs-ctl start` command by using the
`OVS_APPEND` environment variable, for example if you are runnng the
ovsdb-server externally and only need the Open vSwitch client:

```shell
OVS_APPEND: --no-ovsdb-server --no-ovs-vswitchd
```

The script has the ability to optionally add host Ethernet interface(s) to a
Open vSwitch bridge using the `OVS_HOST_IFACE` environment variable. The format
of the variable is `<bridge>:<port>[,<port>,...]`.

> [!CAUTION]
> The OVS bridge to add the interface(s) to __*must be specified*__

For example, the following will add the `eth0` host interface to the `phenix`
OVS bridge, creating the bridge if it doesn't exist already:

```shell
OVS_HOST_IFACE: phenix:eth0
```

Multiple interfaces can also be specified:

```shell
OVS_HOST_IFACE=phenix:eth0,eth1,eth2
```
